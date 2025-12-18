package recover

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"webplus-openapi/pkg/models"

	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SiteInfo 站点信息结构体
type SiteInfo struct {
	ID         string `gorm:"column:id"`
	Name       string `gorm:"column:name"`
	DomainName string `gorm:"column:domainName"`
	DummyName  string `gorm:"column:dummyName"`
	ParentId   string `gorm:"column:parentId"`
}

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
	query.WriteString("f.path AS folderPath, ")
	query.WriteString(buildArticleFieldSelect("a"))
	query.WriteString(" FROM T_ARTICLE a ")
	query.WriteString("JOIN T_FOLDER f on f.id=a.folderId ")
	query.WriteString("JOIN T_SITEARTICLE sa ON a.id = sa.publishArticleId ")
	query.WriteString("JOIN T_SITE s ON sa.siteId = s.id ")
	query.WriteString("WHERE sa.selfCreate = 1 AND a.deleted = 0 AND a.archived = 0 AND a.id = ?")

	// 执行查询，使用临时结构体避免切片字段问题
	type ArticleQueryResult struct {
		ArticleId      string     `gorm:"column:articleId"`
		Title          string     `gorm:"column:title"`
		QuoteTitle     string     `gorm:"column:quoteTitle"`
		ShortTitle     string     `gorm:"column:shortTitle"`
		AuxiliaryTitle string     `gorm:"column:auxiliaryTitle"`
		FolderId       string     `gorm:"column:folderId"`
		TypeId         string     `gorm:"column:typeId"`
		CreatorName    string     `gorm:"column:creatorName"`
		LastModifyTime *time.Time `gorm:"column:lastModifyTime"`
		CreateTime     string     `gorm:"column:createTime"`
		Author         string     `gorm:"column:author"`
		Source         string     `gorm:"column:source"`
		Keywords       string     `gorm:"column:keywords"`
		LinkUrl        string     `gorm:"column:linkUrl"`
		Summary        string     `gorm:"column:summary"`
		ImageDir       string     `gorm:"column:imageDir"`
		FilePath       string     `gorm:"column:filePath"`
		FirstImgPath   string     `gorm:"column:firstImgPath"`
		CreateOrgName  string     `gorm:"column:createOrgName"`
		SiteId         string     `gorm:"column:siteId"`
		UrlPath        string     `gorm:"column:urlPath"`
		SiteName       string     `gorm:"column:siteName"`
		SiteArticleId  string     `gorm:"column:siteArticleId"`
		PublishTime    *time.Time `gorm:"column:publishTime"`
		PublisherName  string     `gorm:"column:publisherName"`
		PublishOrgName string     `gorm:"column:publishOrgName"`
		VisitCount     int        `gorm:"column:visitCount"`
		Opened         int        `gorm:"column:opened"`
		Published      int        `gorm:"column:published"`
		FolderPath     string     `gorm:"column:folderPath"`
		models.ArticleFields
	}

	var queryResult ArticleQueryResult
	err := r.db.Raw(query.String(), article.ArticleId).Scan(&queryResult).Error
	if err != nil {
		return nil, fmt.Errorf("查询文章详情失败: %w", err)
	}

	// 检查是否找到文章
	if queryResult.ArticleId == "" {
		return nil, fmt.Errorf("文章不存在: articleId=%s", article.ArticleId)
	}

	// 手动构建ArticleInfo结构体
	result := models.ArticleInfo{
		ArticleId:      queryResult.ArticleId,
		Title:          queryResult.Title,
		ShortTitle:     queryResult.ShortTitle,
		AuxiliaryTitle: queryResult.AuxiliaryTitle,
		FolderId:       queryResult.FolderId,
		CreatorName:    queryResult.CreatorName,
		LastModifyTime: queryResult.LastModifyTime,
		CreateTime:     queryResult.CreateTime,
		Summary:        queryResult.Summary,
		ImageDir:       queryResult.ImageDir,
		FilePath:       queryResult.FilePath,
		FirstImgPath:   queryResult.FirstImgPath,
		SiteId:         queryResult.SiteId,
		SiteName:       queryResult.SiteName,
		PublishTime:    queryResult.PublishTime,
		PublisherName:  queryResult.PublisherName,
		PublishOrgName: queryResult.PublishOrgName,
		// 初始化切片字段
		ColumnId:   []string{},
		ColumnName: []string{},
	}
	result.ArticleFields = queryResult.ArticleFields

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
	var firstColumn []int
	q1 := r.db.Table("T_COLUMN c").
		Select("c.id,c.name").
		Where("c.singleFolderId = ?", folderIdInt)
	if err := q1.Find(&singleFolderColumn).Error; err != nil {
		zap.S().Errorw("GetArticleById.singleFolderColumn", "err", err)
	}
	if singleFolderColumn.Id > 0 {
		columnIdMap[singleFolderColumn.Id] = singleFolderColumn.Name
		columns = append(columns, singleFolderColumn)
		firstColumn = append(firstColumn, singleFolderColumn.Id)
	}
	zap.S().Debugf("唯一来源 - 根据文件夹ID查找栏目: %v", firstColumn)

	// 2. 获取将文件夹设置为信息源的栏目 (mappingTypeId = 0)
	var dataSourceColumns []models.Column
	var secondColumn []int
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
		secondColumn = append(secondColumn, ds.Id)
	}
	zap.S().Debugf("获取将文件夹设置为信息源的栏目: %v", secondColumn)

	// 3. 根据文章获取直接跨站和多栏发布的栏目（T_COLUMNARTICLE）
	var colArtColumns []models.Column
	var thirdColumn []int
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
		thirdColumn = append(thirdColumn, ca.Id)
	}
	zap.S().Debugf("根据文章获取直接跨站和多栏发布的栏目: %v", thirdColumn)

	// 4. 递归：获取将上面所有栏目作为信息源的栏目 (mappingTypeId = 1)
	r.getDataSourceColumn(columnIdMap, &columns)

	// 写回到 result 的数组字段（确保顺序稳定）
	if len(columnIdMap) > 0 {
		ids := make([]int, 0, len(columnIdMap))
		for id := range columnIdMap {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			result.ColumnId = append(result.ColumnId, strconv.Itoa(id))
			result.ColumnName = append(result.ColumnName, columnIdMap[id])
		}
	}

	// 查询文章附件信息
	baseDomain := r.getBaseDomain(result.SiteId)
	result = *r.queryMediaFileByObjId(&result, baseDomain)

	zap.S().Debugf("成功查询文章详情: articleId=%s, title=%s", result.ArticleId, result.Title)
	return &result, nil
}

// getArticleContent 获取文章内容
func (r *ArticleRepository) getArticleContent(articleId string) (string, error) {
	var content string
	err := r.db.Table("T_ARTICLECONTENT").
		Select("content").
		Where("articleId = ?", articleId).
		Scan(&content).Error

	if err != nil {
		return "", fmt.Errorf("查询文章内容失败: %w", err)
	}

	return content, nil
}

// RestoreArticleInfos 修复文章访问地址
func (r *ArticleRepository) RestoreArticleInfos(articleInfo *models.ArticleInfo) (*models.ArticleInfo, error) {
	// 处理访问地址
	if articleInfo.VisitUrl == "" {
		// 获取第一个栏目ID用于构建访问地址
		var columnId string
		if len(articleInfo.ColumnId) > 0 {
			columnId = articleInfo.ColumnId[0]
		}
		//判断取的这个columnId是否属于创建站点，不属于就换
		if columnId != "" && articleInfo.SiteId != "" {
			// 优先选择与创建站点匹配的栏目
			for _, cid := range articleInfo.ColumnId {
				if cid == "" {
					continue
				}
				if sid, err := r.querySiteByColumnId(cid); err == nil && sid == articleInfo.SiteId {
					columnId = cid
					break
				}
			}
		}
		visitUrl := r.queryVisitUrlFromDB(articleInfo.ArticleId, articleInfo.SiteId, columnId)
		if visitUrl != "" {
			articleInfo.VisitUrl = visitUrl
		}
	}

	// 处理封面图地址
	if articleInfo.FirstImgPath != "" {
		// 获取基础域名用于图片路径处理
		baseDomain := r.getBaseDomain(articleInfo.SiteId)
		articleInfo.FirstImgPath = r.processImagePath(articleInfo.FirstImgPath, articleInfo.FilePath, baseDomain)
	}

	zap.S().Debugf("成功修复文章访问地址: articleId=%s, visitUrl=%s", articleInfo.ArticleId, articleInfo.VisitUrl)
	return articleInfo, nil
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

// queryVisitUrlFromDB 查询文章访问地址
func (r *ArticleRepository) queryVisitUrlFromDB(articleId string, siteId string, columnId string) string {
	// 查询站点信息
	site, err := r.querySiteInfo(siteId)
	if err != nil {
		zap.S().Errorf("查询站点信息失败: %v", err)
		return ""
	}

	// 计算基础域名
	var baseDomain string
	if site.DomainName != "" {
		// 使用正则分割多种分隔符
		re := regexp.MustCompile(`[,，]+`)
		parts := re.Split(site.DomainName, -1)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				baseDomain = part
				break
			}
		}
	} else if site.ParentId != "" {
		// 查询父站点域名
		var parentDomain string
		var parentSiteId string
		var domainSeparator = regexp.MustCompile(`[,，\s;]+`)
		siteParentSQL := "SELECT SITEID FROM T_PUBLISHSITE WHERE id = ?"
		err2 := r.db.Raw(siteParentSQL, site.ParentId).Scan(&parentSiteId)
		parentSQL := "SELECT DOMAINNAME FROM T_SITE WHERE id = ?"
		if err2.Error == nil {
			err2 = r.db.Raw(parentSQL, parentSiteId).Scan(&parentDomain)
		}
		if err2.Error != nil {
			zap.S().Errorf("查询父站点域名失败: %v", err2.Error)
			return ""
		}
		zap.S().Infof("原始 DOMAINNAME: %q", parentDomain)
		parts := domainSeparator.Split(parentDomain, -1)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				parentDomain = part
				break
			}
		}
		baseDomain = parentDomain + "/_s" + siteId
	}

	// 查询文章urlPath，取决于站群文章页URL模式，0：散射目录模式，1：日期模式
	//1、若是散射目录，则查T_ARTICLE表的urlPath字段
	//2、日期模式可能会出现老版本历史遗留问题，所以不看urlPath，直接看T_ARTICLE的createTime
	urlPath, err := r.queryArticleUrlPath(articleId)
	if err != nil {
		zap.S().Errorf("查询文章urlPath失败: %v", err)
		return ""
	}

	// 缺失必要数据
	if baseDomain == "" || urlPath == "" {
		zap.S().Debugf("缺少必要数据构建访问地址，domainName: %s, urlPath: %s", baseDomain, urlPath)
		return ""
	}

	// 构建访问地址
	visitUrl := r.buildVisitUrl(baseDomain, urlPath, columnId, articleId)
	return visitUrl
}

// querySiteInfo 查询站点信息
func (r *ArticleRepository) querySiteInfo(siteId string) (*SiteInfo, error) {
	var site SiteInfo
	siteSQL := `SELECT s.DOMAINNAME as domainName, s.DUMMYNAME as dummyName, ps.PARENTID as parentId  FROM T_SITE s JOIN T_PUBLISHSITE ps ON s.ID = ps.SITEID WHERE s.ID = ?`
	err := r.db.Raw(siteSQL, siteId).Scan(&site).Error
	if err != nil {
		return nil, fmt.Errorf("查询站点信息失败: %w", err)
	}
	return &site, nil
}

func (r *ArticleRepository) querySiteByColumnId(columnId string) (siteId string, err error) {
	siteSQL := `SELECT siteId  FROM T_COLUMN c WHERE c.ID = ?`
	if err = r.db.Raw(siteSQL, columnId).Scan(&siteId).Error; err != nil {
		return "", fmt.Errorf("查询站点信息失败: %w", err)
	}
	return siteId, nil
}

// queryArticleUrlPath 查询文章URL路径
func (r *ArticleRepository) queryArticleUrlPath(articleId string) (string, error) {
	var createTime string
	articleSQL := "SELECT createTime FROM T_ARTICLE WHERE id = ?"
	err := r.db.Raw(articleSQL, articleId).Scan(&createTime).Error
	if err != nil {
		return "", err
	}

	// 解析时间格式并转换为 "2025/0427"
	if createTime == "" {
		return "", nil
	}

	// 尝试解析多种时间格式
	var t time.Time
	formats := []string{
		"2006-01-02T15:04:05Z07:00", // ISO 8601: 2025-04-27T17:56:53+08:00
		"2006-01-02 15:04:05",       // 常规格式: 2025-04-27 10:45:05
		"2006-01-02",                // 仅日期: 2025-04-27
	}

	parsed := false
	for _, format := range formats {
		t, err = time.Parse(format, createTime)
		if err == nil {
			parsed = true
			break
		}
	}

	if parsed {
		// 转换为 "2025/0427" 格式
		return t.Format("2006/0102"), nil
	}

	// 海大从老版本升上来，urlPath很多不是年月日，这里用返回的createTime实际上可以当做是urlPath，为了后续构建URL部分可以复用
	return createTime, nil
}

// buildVisitUrl 构建访问地址的方法
func (r *ArticleRepository) buildVisitUrl(domainName, urlPath, columnId, articleId string) string {
	// urlPath 为空无法构建
	if urlPath == "" {
		return ""
	}

	// 将形如 "2025-0322" 的路径格式化为 "2025/0322"
	formatted := strings.ReplaceAll(urlPath, "-", "/")

	// 规范化 domain：若无协议，默认 http
	if !strings.HasPrefix(domainName, "http://") && !strings.HasPrefix(domainName, "https://") {
		domainName = "http://" + domainName
	}
	domainName = strings.TrimRight(domainName, "/")

	// 拼接短链：/{formatted}/c{columnId}a{articleId}/page.htm
	shortLink := fmt.Sprintf("/%s/c%sa%s/page.htm", formatted, columnId, articleId)

	visitUrl := domainName + shortLink
	return visitUrl
}

// getBaseDomain 获取基础域名用于图片路径处理
func (r *ArticleRepository) getBaseDomain(siteId string) string {
	site, err := r.querySiteInfo(siteId)
	if err != nil {
		zap.S().Warnf("获取站点信息失败: %v", err)
		return ""
	}
	var baseDomain string

	if site.DomainName != "" {
		re := regexp.MustCompile(`[,，]+`)
		parts := re.Split(site.DomainName, -1)
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" {
				baseDomain = part
				break
			}
		}
		return baseDomain
	}
	return ""
}
