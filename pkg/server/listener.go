package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/models"
	"webplus-openapi/pkg/nsc"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// Article 站群发的nats消息结构
type Article struct {
	Operate         string `json:"operate,omitempty"`         // 操作
	ArticleId       string `json:"articleId,omitempty"`       // id
	VisitUrl        string `json:"visitUrl,omitempty"`        // 访问地址
	PublishColumnId string `json:"publishColumnId,omitempty"` // 发布栏目id
	SiteId          string `json:"siteId,omitempty"`          // 站点id
	ServerName      string `json:"serverName,omitempty"`      // 域名
}

type Articles struct {
	List []*Article `json:"list"`
}

const (
	OperateArtUpdate    = "1" //文章新增或修改
	OperateArtDelete    = "2" //文章删除
	OperateColArtDelete = "7"
	OperateColArtCreate = "8"
	OperateArtVisit     = "article.visitCount.change"
)

var once sync.Once

var webplusManager *Manager

type Manager struct {
	cfg *Config
}

func GetInstance() *Manager {
	return webplusManager
}

func Init(cfg *Config) error {
	once.Do(func() {
		inst := &Manager{
			cfg: cfg,
		}
		webplusManager = inst
	})
	return nil
}

func (w *Manager) Serve(cfg *Config, ctx context.Context) error {
	//开启nats客户端链接
	nc := nsc.GetNatsClient()
	js, err := jetstream.New(nc.GetNatsConn())
	if err != nil {
		return err
	}
	err = w.natsStreamMustReady()
	if err != nil {
		return fmt.Errorf("nats stream not ready [%s]", err.Error())
	}
	var consumerName = cfg.Nats.ConsumerName
	//从系统环境变量中取出DEBUG的值来判断我当前是不是调试环境
	if debug, _ := strconv.ParseBool(os.Getenv("DEBUG")); debug {
		//如果是调试环境，就用临时的consumer，设置空值的时候会默认生成随机的consumerName
		consumerName = "temp_consumer"
	}
	//如果不是调试环境，就用指定的consumer
	consumer, err := js.CreateOrUpdateConsumer(context.Background(), cfg.Nats.WebplusStreamName, jetstream.ConsumerConfig{
		Name:          consumerName,
		Durable:       consumerName,
		FilterSubject: cfg.Nats.SubjectName,
	})
	if consumer == nil {
		zap.S().Error("consumer is nil")
		return fmt.Errorf("consumer create error")
	}
	if err != nil {
		return err
	}
	group, c := errgroup.WithContext(ctx)
	group.Go(func() error {
		for {
			select {
			//上下文被取消时，退出循环，返回空
			case <-c.Done():
				zap.S().Info("NATS listener 接收到关闭信号，正在退出...")
				return nil
			default:
				// 使用带超时的Fetch，避免无限循环
				messages, err := consumer.Fetch(1, jetstream.FetchMaxWait(1*time.Second))
				if err != nil {
					zap.S().Debugf("NATS Fetch 错误: %v", err)
					// 如果上下文被取消，直接退出
					select {
					case <-c.Done():
						return nil
					default:
						// 继续循环，但添加短暂延迟避免CPU占用过高
						time.Sleep(100 * time.Millisecond)
						continue
					}
				}
				for msg := range messages.Messages() {
					zap.S().Debugf("msg: %s", string(msg.Data()))
					_ = w.handleOneMsg(msg)
					err := msg.Ack()
					if err != nil {
						zap.S().Errorf("消息确认失败: %v", err)
						return err
					}
				}
			}
		}
	})
	return group.Wait()
}

// 确认stream存在，如果存在，绑定的主题需要追加
func (w *Manager) natsStreamMustReady() error {
	natsClient := nsc.GetNatsClient()
	conn := natsClient.GetNatsConn()
	js, err := jetstream.New(conn)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	//确保stream存在
	stream, err := js.Stream(ctx, w.cfg.Nats.WebplusStreamName)
	zap.S().Infof("*** check stream %s. ***", w.cfg.Nats.WebplusStreamName)
	if err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
		return err
	}
	var subjects = []string{w.cfg.Nats.SubjectName}
	if err == nil {
		zap.S().Infof("*** stream %s exist check subjects. ***", w.cfg.Nats.WebplusStreamName)
		si, err := stream.Info(ctx)
		if err != nil {
			return err
		}
		_subjects := si.Config.Subjects
		subjects = lo.Uniq(append(subjects[:], _subjects[:]...))
	}
	zap.S().Infof("*** make sure stream %s and subject ready. ***", w.cfg.Nats.WebplusStreamName)
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     w.cfg.Nats.WebplusStreamName,
		Subjects: subjects,
	})
	if err != nil {
		return err
	}
	return nil
}

/**
 * 提取消息，持久化到文件流（可以理解为go的数据库）
 */
func (w *Manager) handleOneMsg(msg jetstream.Msg) error {
	var article Article
	err := json.Unmarshal(msg.Data(), &article)
	if err != nil {
		zap.S().Error("JSON解析失败", zap.Error(err))
		return err
	}
	zap.S().Debugf("收到订阅消息事件--> %s", getOpearteName(article.Operate))
	//根据收到的article的operate来进行业务处理
	switch article.Operate {
	case OperateArtUpdate:
		err = w.handleArticleUpdate(&article)
	case OperateArtDelete:
		err = delArticleById(&article)
	case OperateColArtDelete:
		err = deleteColumnArtsByArtId(&article)
	case OperateColArtCreate:
		err = updateColumnArtsByArtId(&article)
	case OperateArtVisit:
		err = updateVisitCountByArtId(&article)

	}
	return err
}

// 处理文章更新
func (w *Manager) handleArticleUpdate(article *Article) error {
	zap.S().Debugf("收到文章更新事件--> 文章id是%s", article.ArticleId)
	artInfo := w.QueryArticleById(article)
	if artInfo == nil {
		zap.S().Debugf("站群不存在这条数据，文章id是=%s", article.ArticleId)
		return nil
	}
	artInfo.VisitUrl = article.VisitUrl
	//封装附件
	artInfo = w.queryMediaFileByObjId(artInfo, article)
	if artInfo == nil {
		zap.S().Errorf("处理文章附件失败，文章id是=%s", article.ArticleId)
		return fmt.Errorf("处理文章附件失败")
	}
	return handleArticleUpsert(artInfo)
}

// QueryArticleById 根据文章id来查询mysql文章信息
func (w *Manager) QueryArticleById(result *Article) *models.ArticleInfo {
	sourceDB := db.GetSourceDB()

	// 使用临时结构体避免切片字段问题
	type ArticleQueryResult struct {
		ArticleId      string `gorm:"column:articleId"`
		Title          string `gorm:"column:title"`
		FolderId       string `gorm:"column:folderId"`
		ShortTitle     string `gorm:"column:shortTitle"`
		AuxiliaryTitle string `gorm:"column:auxiliaryTitle"`
		CreatorName    string `gorm:"column:creatorName"`
		Summary        string `gorm:"column:summary"`
		PublishTime    string `gorm:"column:publishTime"`
		PublisherName  string `gorm:"column:publisherName"`
		PublishOrgName string `gorm:"column:publishOrgName"`
		FirstImgPath   string `gorm:"column:firstImgPath"`
		ImageDir       string `gorm:"column:imageDir"`
		FilePath       string `gorm:"column:filePath"`
		SiteId         string `gorm:"column:siteId"`
		SiteName       string `gorm:"column:siteName"`
		VisitUrl       string `gorm:"column:visitUrl"`
		ColumnId       string `gorm:"column:columnId"`
		ColumnName     string `gorm:"column:columnName"`
		models.ArticleFields
	}

	// 第一步：查询文章基本信息（不包含content）
	var query strings.Builder
	query.WriteString("FROM T_ARTICLE ta ")
	query.WriteString("JOIN T_SITEARTICLE tsa ON ta.id = tsa.publishArticleId ")
	query.WriteString("JOIN T_COLUMN tc ON tc.SyncFolderId = ta.folderId ")
	query.WriteString("JOIN T_SITE ts ON tsa.siteId = ts.id ")
	query.WriteString("WHERE tsa.selfCreate = 1 AND ta.id = ?")

	params := []any{result.ArticleId}

	var queryResult ArticleQueryResult
	fieldSelect := buildArticleFieldSelect("ta")
	baseSelect := "SELECT ts.name as siteName, tsa.siteId AS siteId, ta.id AS articleId, ta.folderId as folderId, " +
		"ta.linkUrl AS visitUrl, " +
		"tc.id AS columnId, tc.name AS columnName, ta.title AS title, " +
		"ta.shortTitle as shortTitle, ta.auxiliaryTitle as auxiliaryTitle, " +
		"ta.creatorName as creatorName, ta.summary, " +
		"tsa.publishTime AS publishTime, tsa.publisherName AS publisherName, " +
		"tsa.publishOrgName AS publishOrgName, ta.firstImgPath, " +
		"ta.imagedir AS imageDir, ta.filepath AS filePath"
	sql := fmt.Sprintf("%s, %s %s", baseSelect, fieldSelect, query.String())

	err := sourceDB.Raw(sql, params...).Scan(&queryResult)
	if err.Error != nil {
		return nil
	}

	if queryResult.ArticleId == "" {
		return nil
	}

	// 手动构建ArticleInfo结构体
	articleInfo := &models.ArticleInfo{
		ArticleId:      queryResult.ArticleId,
		FolderId:       queryResult.FolderId,
		Title:          queryResult.Title,
		ShortTitle:     queryResult.ShortTitle,
		AuxiliaryTitle: queryResult.AuxiliaryTitle,
		CreatorName:    queryResult.CreatorName,
		Summary:        queryResult.Summary,
		PublishTime:    ParsePgTime(queryResult.PublishTime),
		PublisherName:  queryResult.PublisherName,
		PublishOrgName: queryResult.PublishOrgName,
		FirstImgPath:   queryResult.FirstImgPath,
		ImageDir:       queryResult.ImageDir,
		FilePath:       queryResult.FilePath,
		SiteId:         queryResult.SiteId,
		SiteName:       queryResult.SiteName,
		VisitUrl:       queryResult.VisitUrl,
		// 初始化切片字段
		ColumnId:   []string{queryResult.ColumnId},
		ColumnName: []string{queryResult.ColumnName},
	}
	articleInfo.ArticleFields = queryResult.ArticleFields

	// 第二步：查询文章内容并直接拼接
	var contentList []struct {
		Content string `json:"content"`
	}

	contentSQL := "SELECT content FROM T_ARTICLECONTENT WHERE articleId = ?"
	err = sourceDB.Raw(contentSQL, result.ArticleId).Scan(&contentList)
	if err.Error != nil {
		articleInfo.Content = ""
	} else {
		var fullContent strings.Builder
		for _, contentItem := range contentList {
			fullContent.WriteString(contentItem.Content)
		}
		articleInfo.Content = fullContent.String()
	}
	return articleInfo
}

func (w *Manager) queryMediaFileByObjId(artInfo *models.ArticleInfo, article *Article) *models.ArticleInfo {
	if artInfo == nil {
		zap.S().Error("文章信息为空，无法查询附件")
		return nil
	}
	//根据文章id来查询文件路径mediaFile
	webplusDB := db.GetSourceDB()
	//构建查询语句
	var query strings.Builder
	query.WriteString("FROM `T_MEDIAFILE_USED`  tmu   JOIN `T_MEDIAFILE`  tm  ON  tmu.`mediaFileId` = tm.`id`  WHERE tmu.`objId` = ?")
	var params []any
	params = append(params, artInfo.ArticleId)
	//利用切片数组来接收查询结果
	var attachments []models.Attachment
	webplusDB.Raw(fmt.Sprintf("select  tm.`name` AS name,tm.filePath  AS path  %s", query.String()), params...).Scan(&attachments)
	if len(attachments) > 0 {
		//处理path
		for _, attachment := range attachments {
			if attachment.Path != "" {
				serArr := strings.Split(article.ServerName, "/main.")
				attachment.Path = serArr[0] + attachment.Path
			}
		}
		artInfo.Attachment = attachments
	}
	return artInfo
}

// queryColumnInfo 根据栏目ID查询栏目名称
func queryColumnInfo(columnIdStr string) string {
	if columnIdStr == "" {
		return ""
	}
	webplusDB := db.GetSourceDB()
	var columnName string
	sql := "SELECT name FROM T_COLUMN WHERE id = ?"
	err := webplusDB.Raw(sql, columnIdStr).Scan(&columnName)
	if err.Error != nil {
		zap.S().Errorf("查询栏目信息失败: columnId=%s, err=%v", columnIdStr, err.Error)
		return ""
	}
	return columnName
}

// querySiteInfo 根据栏目ID查询栏目名称
func querySiteInfo(siteIdStr string) string {
	if siteIdStr == "" {
		return ""
	}
	webplusDB := db.GetSourceDB()
	var siteName string
	sql := "SELECT name FROM T_SITE WHERE id = ?"
	err := webplusDB.Raw(sql, siteIdStr).Scan(&siteName)
	if err.Error != nil {
		zap.S().Errorf("查询栏目信息失败: columnId=%s, err=%v", siteIdStr, err.Error)
		return ""
	}
	return siteName
}

func ParsePgTime(raw string) *time.Time {
	if raw == "" {
		return nil
	}
	// 尝试多种常见的 Postgres 字符串时间格式
	layouts := []string{
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, raw)
		if err == nil {
			return &t
		}
	}
	return nil
}

// 处理文章新增还是修改：写入 targetDB 的 article_static / article_dynamic
func handleArticleUpsert(artInfo *models.ArticleInfo) error {
	if artInfo == nil {
		return fmt.Errorf("文章信息为空，无法存储")
	}
	if artInfo.LastModifyTime != nil && artInfo.LastModifyTime.IsZero() {
		artInfo.LastModifyTime = nil
	}

	targetDB := db.GetTargetDB()
	if targetDB == nil {
		return fmt.Errorf("targetDB 未初始化")
	}

	articleIDInt, err := strconv.ParseInt(artInfo.ArticleId, 10, 64)
	if err != nil {
		return fmt.Errorf("articleId 转换失败: %v", err)
	}

	tx := targetDB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %v", tx.Error)
	}

	// 清理旧数据
	if err := tx.Table(models.TableNameArticleStatic).Where("articleId = ?", articleIDInt).Delete(nil).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("清理 article_static 失败: %v", err)
	}
	if err := tx.Table(models.TableNameArticleDynamic).Where("articleId = ?", articleIDInt).Delete(nil).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("清理 article_dynamic 失败: %v", err)
	}

	// 写入 article_static（创建站点 URL）
	articleRow := map[string]interface{}{
		"articleId":      articleIDInt,
		"createSiteId":   artInfo.SiteId,
		"folderId":       artInfo.FolderId,
		"title":          artInfo.Title,
		"shortTitle":     artInfo.ShortTitle,
		"auxiliaryTitle": artInfo.AuxiliaryTitle,
		"creatorName":    artInfo.CreatorName,
		"summary":        artInfo.Summary,
		"publishTime":    artInfo.PublishTime,
		"lastModifyTime": artInfo.LastModifyTime,
		"visitUrl":       artInfo.VisitUrl,
		"createTime":     artInfo.CreateTime,
		"firstImgPath":   artInfo.FirstImgPath,
		"imageDir":       artInfo.ImageDir,
		"filePath":       artInfo.FilePath,
		"publisherName":  artInfo.PublisherName,
		"publishOrgName": artInfo.PublishOrgName,
		"content":        artInfo.Content,
	}

	// 扩展字段 field1-50
	fieldsValue := reflect.ValueOf(artInfo.ArticleFields)
	fieldsType := fieldsValue.Type()
	for i := 0; i < fieldsType.NumField(); i++ {
		field := fieldsType.Field(i)
		fieldValue := fieldsValue.Field(i)
		fieldName := strings.ToLower(field.Name) // Field1 -> field1
		articleRow[fieldName] = fieldValue.String()
	}

	if err := tx.Table(models.TableNameArticleStatic).Create(&articleRow).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("写入 article_static 失败: %v", err)
	}

	// 写入 article_dynamic（按当前 Id/Name 列表）
	for i := range artInfo.ColumnId {
		colIDStr := artInfo.ColumnId[i]
		colName := ""
		if i < len(artInfo.ColumnName) {
			colName = artInfo.ColumnName[i]
		}
		colIDInt, err := strconv.ParseInt(colIDStr, 10, 64)
		if err != nil {
			zap.S().Warnf("解析栏目ID失败: articleId=%s, columnId=%s, err=%v", artInfo.ArticleId, colIDStr, err)
			continue
		}
		colRow := map[string]interface{}{
			"articleId":  articleIDInt,
			"columnId":   colIDInt,
			"columnName": colName,
			"siteId":     artInfo.SiteId,
			"siteName":   artInfo.SiteName,
			"url":        artInfo.VisitUrl,
		}
		if err := tx.Table(models.TableNameArticleDynamic).Create(&colRow).Error; err != nil {
			tx.Rollback()
			return fmt.Errorf("写入 article_dynamic 失败: %v", err)
		}
	}

	// 写入 article_attachment
	if len(artInfo.Attachment) > 0 {
		for _, att := range artInfo.Attachment {
			attRow := map[string]interface{}{
				"articleId": articleIDInt,
				"name":      att.Name,
				"path":      att.Path,
			}
			if err := tx.Table(models.TableNameArticleAttachment).Create(&attRow).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("写入 article_attachment 失败: %v", err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}

	return nil
}

// delArticleById 文章删除：删除 targetDB 中的 article_static / article_dynamic 记录
func delArticleById(msg *Article) error {
	targetDB := db.GetTargetDB()
	if targetDB == nil {
		return fmt.Errorf("targetDB 未初始化")
	}
	articleIDInt, err := strconv.ParseInt(msg.ArticleId, 10, 64)
	if err != nil {
		return fmt.Errorf("articleId 转换失败: %v", err)
	}
	tx := targetDB.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %v", tx.Error)
	}
	if err := tx.Table(models.TableNameArticleStatic).Where("articleId = ?", articleIDInt).Delete(nil).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("删除 article_static 失败: %v", err)
	}
	if err := tx.Table(models.TableNameArticleDynamic).Where("articleId = ?", articleIDInt).Delete(nil).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("删除 article_dynamic 失败: %v", err)
	}
	if err := tx.Table(models.TableNameArticleAttachment).Where("articleId = ?", articleIDInt).Delete(nil).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("删除 article_attachment 失败: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("提交事务失败: %v", err)
	}
	return nil
}

// updateColumnArtsByArtId 栏目文章新增：在 article_dynamic 中为文章增加一个栏目记录
func updateColumnArtsByArtId(msg *Article) error {
	targetDB := db.GetTargetDB()
	if targetDB == nil {
		return fmt.Errorf("targetDB 未初始化")
	}

	articleIDInt, err := strconv.ParseInt(msg.ArticleId, 10, 64)
	if err != nil {
		return fmt.Errorf("articleId 转换失败: %v", err)
	}
	colIDInt, err := strconv.ParseInt(msg.PublishColumnId, 10, 64)
	if err != nil {
		return fmt.Errorf("columnId 转换失败: %v", err)
	}

	columnNameStr := queryColumnInfo(msg.PublishColumnId)

	// 检查是否已存在
	var count int64
	if err := targetDB.Table(models.TableNameArticleDynamic).
		Where("articleId = ? AND columnId = ?", articleIDInt, colIDInt).
		Count(&count).Error; err != nil {
		return fmt.Errorf("检查栏目记录失败: %v", err)
	}
	if count > 0 {
		zap.S().Debugf("文章 %s 的栏目ID %s 已存在，跳过添加", msg.ArticleId, msg.PublishColumnId)
		return nil
	}

	colRow := map[string]interface{}{
		"articleId":  articleIDInt,
		"columnId":   colIDInt,
		"columnName": columnNameStr,
		"siteId":     msg.SiteId,
		"siteName":   querySiteInfo(msg.SiteId),
		"url":        msg.VisitUrl,
	}
	if err := targetDB.Table(models.TableNameArticleDynamic).Create(&colRow).Error; err != nil {
		return fmt.Errorf("写入 article_dynamic 失败: %v", err)
	}

	zap.S().Infof("成功为文章 %s 添加栏目ID: %s", msg.ArticleId, msg.PublishColumnId)
	return nil
}

// deleteColumnArtsByArtId 栏目文章删除：从 article_dynamic 中删除指定栏目记录
func deleteColumnArtsByArtId(msg *Article) error {
	targetDB := db.GetTargetDB()
	if targetDB == nil {
		return fmt.Errorf("targetDB 未初始化")
	}

	articleIDInt, err := strconv.ParseInt(msg.ArticleId, 10, 64)
	if err != nil {
		return fmt.Errorf("articleId 转换失败: %v", err)
	}
	colIDInt, err := strconv.ParseInt(msg.PublishColumnId, 10, 64)
	if err != nil {
		return fmt.Errorf("columnId 转换失败: %v", err)
	}

	if err := targetDB.Table(models.TableNameArticleDynamic).
		Where("articleId = ? AND columnId = ?", articleIDInt, colIDInt).
		Delete(nil).Error; err != nil {
		return fmt.Errorf("删除 article_dynamic 失败: %v", err)
	}

	zap.S().Infof("成功从文章 %s 中移除栏目ID: %s", msg.ArticleId, msg.PublishColumnId)
	return nil
}

func getOpearteName(operate string) string {
	operateName := ""
	switch operate {
	case OperateArtUpdate:
		operateName = "文章新增或修改"
	case OperateArtDelete:
		operateName = "文章删除"
	case OperateColArtDelete:
		operateName = "栏目文章删除"
	case OperateColArtCreate:
		operateName = "栏目文章新增"
	case OperateArtVisit:
		operateName = "访问量变化"
	default:
		operateName = "未知操作"
	}
	return operateName
}

func buildArticleFieldSelect(alias string) string {
	var builder strings.Builder
	for i := 1; i <= 50; i++ {
		builder.WriteString(fmt.Sprintf("%s.field%d AS field%d", alias, i, i))
		if i != 50 {
			builder.WriteString(", ")
		}
	}
	return builder.String()
}

func updateVisitCountByArtId(msg *Article) error {
	targetDB := db.GetTargetDB()
	if targetDB == nil {
		return fmt.Errorf("targetDB 未初始化")
	}
	sourceDB := db.GetSourceDB()
	if sourceDB == nil {
		return fmt.Errorf("sourceDB 未初始化")
	}
	articleIdsStr := msg.ArticleId

	// 分割文章ID，并过滤掉空字符串（处理末尾逗号的情况）
	articleIdsRaw := strings.Split(articleIdsStr, ",")
	articleIds := make([]string, 0, len(articleIdsRaw))
	for _, id := range articleIdsRaw {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			articleIds = append(articleIds, trimmed)
		}
	}

	// 如果没有有效的文章ID，直接返回
	if len(articleIds) == 0 {
		zap.S().Debugf("没有有效的文章ID，跳过访问量更新")
		return nil
	}

	// 检查 targetDB 中文章是否存在
	var count int64
	if err := targetDB.Table(models.TableNameArticleStatic).
		Where("articleId IN ?", articleIds).
		Count(&count).Error; err != nil {
		return fmt.Errorf("检查文章是否存在失败: %v", err)
	}

	// 如果文章不存在，跳过更新
	if count == 0 {
		zap.S().Debugf("文章 %v 在 targetDB 中不存在，跳过访问量更新", articleIds)
		return nil
	}

	// 从 sourceDB 批量查询访问量
	type VisitCountResult struct {
		PublishArticleId string `gorm:"column:publishArticleId"`
		VisitCount       int    `gorm:"column:visitCount"`
	}
	var visitResults []VisitCountResult
	sql := "SELECT sa.publishArticleId, sa.visitCount FROM T_SITEARTICLE sa WHERE sa.publishArticleId IN ? AND sa.selfCreate = 1"
	if err := sourceDB.Raw(sql, articleIds).Scan(&visitResults).Error; err != nil {
		return fmt.Errorf("从 sourceDB 查询访问量失败: %v", err)
	}

	// 如果没有查询到结果，跳过更新
	if len(visitResults) == 0 {
		zap.S().Debugf("文章 %v 在 sourceDB 中不存在或未发布，跳过访问量更新", articleIds)
		return nil
	}

	// 构建访问量映射，方便批量更新
	visitCountMap := make(map[string]int)
	for _, result := range visitResults {
		visitCountMap[result.PublishArticleId] = result.VisitCount
	}

	// 批量更新 targetDB 中的访问量
	updatedCount := 0
	for _, articleId := range articleIds {
		if visitCount, exists := visitCountMap[articleId]; exists {
			// 使用 Update 方法（单数形式）只更新 visitCount 字段，确保不影响其他字段
			if err := targetDB.Table(models.TableNameArticleStatic).
				Where("articleId = ?", articleId).
				Update("visitCount", visitCount).Error; err != nil {
				zap.S().Warnf("更新文章 %s 的访问量失败: %v", articleId, err)
				continue
			}
			updatedCount++
		}
	}

	zap.S().Infof("成功更新 %d 篇文章的访问量，文章ID: %v", updatedCount, articleIds)
	return nil
}
