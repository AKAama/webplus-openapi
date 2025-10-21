package recover

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"webplus-openapi/pkg/models"

	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type BadgerStore interface {
	Store(key string, value interface{}) error
	Get(key string, value interface{}) error
	Delete(key string) error
	Exists(key string) bool
	View(fn func(txn *badger.Txn) error) error
	Upsert(key string, value interface{}) error
	DeleteMatching(value interface{}, query badgerhold.Query) error
}

var once sync.Once
var manager *Manager

func GetInstance() *Manager {
	return manager
}

func Init(cfg *Config) error {
	once.Do(func() {
		inst := &Manager{
			cfg: cfg,
		}
		manager = inst
	})
	return nil
}

func NewRecoverService(db *gorm.DB, manager *Manager, badgerStore BadgerStore) *Service {
	return &Service{
		db:          db,
		manager:     manager,
		badgerStore: badgerStore,
	}
}

// GetArticleById 根据ID查询文章详情
func (r *ArticleRepository) GetArticleById(article *models.ArticleInfo) (*models.ArticleInfo, error) {
	// 构建SQL查询语句
	var query strings.Builder
	query.WriteString("SELECT ")
	query.WriteString("a.id AS articleId,a.title AS title,a.quoteTitle as quoteTitle,a.shortTitle as shortTitle,a.auxiliaryTitle as auxiliaryTitle, ")
	query.WriteString("a.folderId as folderId,a.typeId,a.creatorName as creatorName,a.lastModifyTime,a.createTime,")
	query.WriteString("a.author as author,a.source as source,a.keywords,a.linkUrl as linkUrl,a.summary AS summary,")
	query.WriteString("a.imageDir AS imageDir,a.filepath AS filePath,a.firstImgPath,a.createOrgName as createOrgName,a.createSiteId as siteId,")
	query.WriteString("a.urlPath,")
	query.WriteString("s.name as siteName,")
	query.WriteString("sa.id AS siteArticleId,sa.publishTime AS publishTime, sa.publisherName AS publisherName,sa.publishOrgName AS publishOrgName,sa.visitCount AS visitCount, ")
	query.WriteString("sa.opened AS opened,sa.published AS published,")
	query.WriteString("f.path AS folderPath ")
	query.WriteString("FROM T_ARTICLE a ")
	query.WriteString("JOIN T_FOLDER f on f.id=a.folderId ")
	query.WriteString("JOIN T_SITEARTICLE sa ON a.id = sa.publishArticleId ")
	query.WriteString("JOIN T_SITE s ON sa.siteId = s.id ")
	query.WriteString("WHERE sa.selfCreate = 1 AND a.deleted = 0 AND a.archived = 0 AND a.id = ?")

	// 执行查询，直接使用ArticleInfo结构体
	var result models.ArticleInfo
	err := r.db.Raw(query.String(), article.ArticleId).Scan(&result).Error
	if err != nil {
		return nil, fmt.Errorf("查询文章详情失败: %w", err)
	}

	// 检查是否找到文章
	if result.ArticleId == "" {
		return nil, fmt.Errorf("文章不存在: articleId=%s", article.ArticleId)
	}

	// 获取文章内容
	content, err := r.getArticleContent(article.ArticleId)
	if err != nil {
		zap.S().Warnf("获取文章内容失败: articleId=%s, err=%v", article.ArticleId, err)
		content = "" // 内容获取失败不影响文章恢复
	}

	// 设置文章内容
	result.Content = content

	// 计算并填充所有展示栏目（去重）
	// 1) 唯一来源栏目的栏目
	columnIdMap := make(map[int]string)
	columns := make([]models.Column, 0)

	// 解析 articleId 与 folderId 为整型
	articleIdInt, _ := strconv.Atoi(result.ArticleId)
	folderIdInt, _ := strconv.Atoi(result.FolderId)

	// 1. 唯一来源 - 根据文件夹ID查找栏目
	var singleFolderColumn models.Column
	q1 := r.db.Table("T_COLUMN c").
		Select("c.id,c.name").
		Where("c.singleFolderId = ?", folderIdInt)
	if err := q1.Find(&singleFolderColumn).Error; err != nil {
		zap.S().Errorw("GetArticleById.singleFolderColumn", "err", err)
	}
	if singleFolderColumn.Id > 0 {
		columnIdMap[singleFolderColumn.Id] = singleFolderColumn.Name
		columns = append(columns, singleFolderColumn)
	}

	// 2. 获取将文件夹设置为信息源的栏目 (mappingTypeId = 0)
	var dataSourceColumns []models.Column
	if err := r.db.Table("T_COLUMN_DATASOURCE cds").
		Select("cds.SrcColumnId as id,c.name").
		Joins("JOIN T_COLUMN c on cds.SrcColumnId = c.id").
		Where("cds.mappingObjectId = ?", folderIdInt).
		Where("cds.mappingTypeId = ?", 0).
		Find(&dataSourceColumns).Error; err != nil {
		zap.S().Errorw("GetArticleById.dataSourceColumns", "err", err)
	}
	for _, ds := range dataSourceColumns {
		if _, exists := columnIdMap[ds.Id]; exists {
			continue
		}
		columnIdMap[ds.Id] = ds.Name
		columns = append(columns, ds)
	}

	// 3. 根据文章获取直接跨站和多栏发布的栏目（T_COLUMNARTICLE）
	var colArtColumns []models.Column
	if err := r.db.Table("T_COLUMN c").
		Select("c.id,c.name").
		Joins("JOIN T_COLUMNARTICLE ca ON c.id=ca.columnId").
		Where("ca.articleId = ?", articleIdInt).
		Find(&colArtColumns).Error; err != nil {
		zap.S().Errorw("GetArticleById.colArtColumns", "err", err)
	}
	for _, ca := range colArtColumns {
		if _, exists := columnIdMap[ca.Id]; exists {
			continue
		}
		columnIdMap[ca.Id] = ca.Name
		columns = append(columns, ca)
	}

	// 4. 递归：获取将上面所有栏目作为信息源的栏目 (mappingTypeId = 1)
	r.getDataSourceColumn(columnIdMap, &columns)

	// 写回到 result 的数组字段
	for id, name := range columnIdMap {
		result.ColumnId = append(result.ColumnId, strconv.Itoa(id))
		result.ColumnName = append(result.ColumnName, name)
	}

	// 查询文章附件信息
	result = *r.queryMediaFileByObjId(&result, "")

	zap.S().Debugf("成功查询文章详情: articleId=%s, title=%s", result.ArticleId, result.Title)
	return &result, nil
}

// getArticleContent 获取文章内容
func (r *ArticleRepository) getArticleContent(articleId string) (string, error) {
	var content string
	err := r.db.Table("T_ARTICLE").
		Select("content").
		Where("id = ?", articleId).
		Scan(&content).Error

	if err != nil {
		return "", fmt.Errorf("查询文章内容失败: %w", err)
	}

	return content, nil
}

// RestoreArticleInfos 修复文章访问地址
func (r *ArticleRepository) RestoreArticleInfos(articleInfo *models.ArticleInfo) (*models.ArticleInfo, error) {
	// 获取站点域名
	visitDomain, err := r.getVisitDomain(articleInfo.SiteId)
	if err != nil {
		zap.S().Warnf("获取站点域名失败: siteId=%s, err=%v", articleInfo.SiteId, err)
		// 域名获取失败不影响文章恢复，使用默认值
		visitDomain = ""
	}

	// 处理访问地址
	if articleInfo.VisitUrl == "" {
		visitUrl, err := r.generateVisitUrl(articleInfo, visitDomain)
		if err != nil {
			zap.S().Warnf("生成访问地址失败: articleId=%s, err=%v", articleInfo.ArticleId, err)
		} else {
			articleInfo.VisitUrl = visitUrl
		}
	}

	// 处理封面图地址
	if articleInfo.FirstImgPath != "" {
		articleInfo.FirstImgPath = r.processImagePath(articleInfo.FirstImgPath, articleInfo.FilePath, visitDomain)
	}

	zap.S().Debugf("成功修复文章访问地址: articleId=%s, visitUrl=%s", articleInfo.ArticleId, articleInfo.VisitUrl)
	return articleInfo, nil
}

// getVisitDomain 获取站点访问域名
func (r *ArticleRepository) getVisitDomain(siteId string) (string, error) {
	var domain string
	err := r.db.Table("T_SITE").
		Select("domain").
		Where("id = ?", siteId).
		Scan(&domain).Error

	if err != nil {
		return "", fmt.Errorf("查询站点域名失败: %w", err)
	}

	return domain, nil
}

// generateVisitUrl 生成文章访问地址
func (r *ArticleRepository) generateVisitUrl(articleInfo *models.ArticleInfo, visitDomain string) (string, error) {
	if visitDomain != "" {
		return fmt.Sprintf("%s/article/%s", visitDomain, articleInfo.ArticleId), nil
	}

	// 如果没有域名，返回空字符串
	return "", nil
}

// processImagePath 处理图片路径
func (r *ArticleRepository) processImagePath(firstImgPath, filePath, visitDomain string) string {
	if firstImgPath == "" {
		return ""
	}

	// 如果图片路径已经包含完整路径，直接返回
	if strings.Contains(firstImgPath, "/_upload/article/images/") {
		if visitDomain != "" {
			return visitDomain + firstImgPath
		}
		return firstImgPath
	}

	// 处理相对路径
	if !strings.HasPrefix(firstImgPath, "/") {
		firstImgPath = "/" + firstImgPath
	}

	if visitDomain != "" {
		return visitDomain + "/_upload/article/images/" + filePath + firstImgPath
	}

	return firstImgPath
}

// queryMediaFileByObjId 根据文章ID查询附件信息
func (r *ArticleRepository) queryMediaFileByObjId(articleInfo *models.ArticleInfo, serverName string) *models.ArticleInfo {
	// 构建查询语句
	var query strings.Builder
	query.WriteString("FROM `T_MEDIAFILE_USED` tmu JOIN `T_MEDIAFILE` tm ON tmu.`mediaFileId` = tm.`id` WHERE tmu.`objId` = ?")

	var params []any
	params = append(params, articleInfo.ArticleId)

	// 利用切片数组来接收查询结果
	var attachments []models.Attachment
	sql := fmt.Sprintf("SELECT tm.`name` AS name, tm.filePath AS path %s", query.String())

	err := r.db.Raw(sql, params...).Scan(&attachments).Error
	if err != nil {
		zap.S().Warnf("查询文章附件失败: articleId=%s, err=%v", articleInfo.ArticleId, err)
		return articleInfo
	}

	if len(attachments) > 0 {
		// 处理附件路径
		for i := range attachments {
			if attachments[i].Path != "" {
				// 处理服务器名称，去掉/main.部分
				serArr := strings.Split(serverName, "/main.")
				if len(serArr) > 0 {
					attachments[i].Path = serArr[0] + attachments[i].Path
				}
			}
		}
		articleInfo.Attachment = attachments
		zap.S().Debugf("成功查询文章附件: articleId=%s, 附件数量=%d", articleInfo.ArticleId, len(attachments))
	}

	return articleInfo
}

// getDataSourceColumn 递归获取展示栏目 (mappingTypeId = 1)
func (r *ArticleRepository) getDataSourceColumn(columnIdMap map[int]string, columns *[]models.Column) {
	if len(*columns) == 0 {
		return
	}
	for _, column := range *columns {
		cs := make([]models.Column, 0)
		err := r.db.Table("T_COLUMN_DATASOURCE cds").
			Select("cds.SrcColumnId as id,c.name").
			Joins("JOIN T_COLUMN c on cds.SrcColumnId = c.id").
			Where("cds.mappingObjectId = ?", column.Id).
			Where("cds.mappingTypeId = ?", 1).
			Find(&cs).Error
		if err != nil {
			zap.S().Errorw("GetArticleById.getDataSourceColumn", "err", err)
			continue
		}
		if len(cs) > 0 {
			newColumns := make([]models.Column, 0)
			for _, c := range cs {
				if _, exists := columnIdMap[c.Id]; exists {
					continue
				}
				nc := models.Column{Id: c.Id, Name: c.Name}
				newColumns = append(newColumns, nc)
				columnIdMap[c.Id] = c.Name
				*columns = append(*columns, c)
			}
			r.getDataSourceColumn(columnIdMap, &newColumns)
		}
	}
}
