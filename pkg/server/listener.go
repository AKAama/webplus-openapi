package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/models"
	"webplus-openapi/pkg/nsc"
	"webplus-openapi/pkg/store"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"github.com/timshannon/badgerhold/v4"
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
	OperateArtUpdate = "1" //文章新增或修改
	OperateArtDelete = "2" //文章删除
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
				return nil
				//如果上下文没有被取消
			default:
				messages, _ := consumer.Fetch(1, jetstream.FetchMaxWait(1*time.Second))
				for msg := range messages.Messages() {
					zap.S().Debugf("msg: %s", string(msg.Data()))
					_ = w.handleOneMsg(msg)
					err := msg.Ack()
					if err != nil {
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
	}
	return err
}

// 处理文章更新
func (w *Manager) handleArticleUpdate(article *Article) error {
	zap.S().Debugf("收到文章更新事件--> %s", article.ArticleId)
	artInfo := w.QueryArticleById(article)
	if artInfo == nil {
		zap.S().Debugf("站群不存在这条数据，文章id是=%s", article.ArticleId)
		return nil
	}
	artInfo.VisitUrl = article.VisitUrl
	//封装附件
	artInfo = w.queryMediaFileByObjId(artInfo, article)
	return handleArticleUpsert(artInfo)
}

// QueryArticleById 根据文章id来查询mysql文章信息
func (w *Manager) QueryArticleById(result *Article) *models.ArticleInfo {
	webplusDB := db.GetDB()

	// 第一步：查询文章基本信息（不包含content）
	var query strings.Builder
	query.WriteString("FROM T_ARTICLE ta ")
	query.WriteString("JOIN T_SITEARTICLE tsa ON ta.id = tsa.publishArticleId ")
	query.WriteString("JOIN T_COLUMN tc ON tc.SyncFolderId = ta.folderId ")
	query.WriteString("JOIN T_SITE ts ON tsa.siteId = ts.id ")
	query.WriteString("WHERE tsa.selfCreate = 1 AND ta.id = ?")

	params := []any{result.ArticleId}

	var list []*models.ArticleInfo
	sql := fmt.Sprintf("SELECT ts.name as siteName, tsa.siteId AS siteId, ta.id AS articleId, "+
		"ta.linkUrl AS VisitUrl, "+
		"tc.id AS columnId, tc.name AS columnName, ta.title AS title, "+
		"ta.shortTitle as shortTitle, ta.auxiliaryTitle as auxiliaryTitle, "+
		"ta.creatorName as creatorName, ta.summary, "+
		"tsa.publishTime AS publishTime, tsa.publisherName AS publisherName, "+
		"tsa.publishOrgName AS publishOrgName, ta.firstImgPath, "+
		"ta.imagedir AS imageDir, ta.filepath AS filePath, "+
		"tsa.opened AS opened, "+ // 是否公开
		"ta.deleted AS deleted, ta.archived AS archived, ta.frozen AS frozen, "+
		"ta.status AS status, tsa.published AS published "+ // 状态相关字段
		"%s", query.String())

	err := webplusDB.Raw(sql, params...).Scan(&list)
	if err.Error != nil {
		return nil
	}

	if len(list) == 0 {
		return nil
	}

	articleInfo := list[0]

	// 第二步：查询文章内容并直接拼接
	var contentList []struct {
		Content string `json:"content"`
	}

	contentSQL := "SELECT content FROM T_ARTICLECONTENT WHERE articleId = ?"
	err = webplusDB.Raw(contentSQL, result.ArticleId).Scan(&contentList)
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
	//根据文章id来查询文件路径mediaFile
	webplusDB := db.GetDB()
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

// 处理文章新增还是修改
func handleArticleUpsert(artInfo *models.ArticleInfo) error {
	if artInfo.LastModifyTime.IsZero() {
		artInfo.LastModifyTime = nil
	}
	bs := store.GetBadgerStore()
	return bs.Upsert(artInfo.ArticleId, artInfo)
}

func delArticleById(msg *Article) error {
	bs := store.GetBadgerStore()
	query := badgerhold.Where("ArticleId").Eq(msg.ArticleId)
	return bs.DeleteMatching(&models.ArticleInfo{}, *query)
}

func getOpearteName(operate string) string {
	operateName := ""
	switch operate {
	case OperateArtUpdate:
		operateName = "文章新增或修改"
	case OperateArtDelete:
		operateName = "文章删除"
	default:
		operateName = "未知操作"
	}
	return operateName
}
