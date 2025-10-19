package recover

import (
	"fmt"
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

	// 设置文章内容和其他必要字段
	result.Content = content
	result.ColumnId = article.ColumnId // 使用传入的ColumnId

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
func (r *ArticleRepository) RestoreArticleInfos(articleInfo *models.ArticleInfo, article *models.ArticleInfo) (*models.ArticleInfo, error) {
	// 获取站点域名
	visitDomain, err := r.getVisitDomain(articleInfo.SiteId)
	if err != nil {
		zap.S().Warnf("获取站点域名失败: siteId=%s, err=%v", articleInfo.SiteId, err)
		// 域名获取失败不影响文章恢复，使用默认值
		visitDomain = ""
	}

	// 处理访问地址
	if articleInfo.VisitUrl == "" {
		// 如果没有访问地址，尝试生成
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

	// 处理附件访问域名（如果需要的话，可以在这里设置相关字段）
	// articleInfo.AttachVisitDomain = visitDomain

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
	// 这里可以根据实际业务逻辑生成访问地址
	// 暂时返回一个简单的URL格式
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
