package server

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
	"webplus-openapi/pkg/models"
	"webplus-openapi/pkg/store"
	"webplus-openapi/pkg/util"

	"github.com/gin-gonic/gin"
	"github.com/timshannon/badgerhold/v4"
)

// Handler v1版本API处理器
type Handler struct {
	cfg            Config
	articleManager *store.BadgerStore
}

// Pager 分页参数
type Pager struct {
	Page     int `json:"page" form:"page" binding:"min=1"`
	PageSize int `json:"pageSize" form:"pageSize" binding:"min=1,max=100"`
	Total    int `json:"total"`
}

// GetArticles 获取文章列表
func (h *Handler) GetArticles(c *gin.Context) {
	pager := h.parsePager(c)
	columnIdStr := c.Query("columnId")
	siteIdStr := c.Query("siteId")

	// 转换ID类型
	var columnId, siteId = 0, 0
	if columnIdStr != "" {
		if parsed, err := strconv.Atoi(columnIdStr); err == nil {
			columnId = parsed
		}
	}
	if siteIdStr != "" {
		if parsed, err := strconv.Atoi(siteIdStr); err == nil {
			siteId = parsed
		}
	}

	// 检查是否有关键字
	keyWord := c.Query("keyWord")
	startTimeStr := c.Query("startTime")
	endTimeStr := c.Query("endTime")

	var startTime, endTime *time.Time

	if startTimeStr != "" {
		if t, err := h.parseTimeString(startTimeStr); err != nil {
			util.Err(c, fmt.Errorf("invalid startTime format: %v", err))
			return
		} else {
			startTime = &t
		}
	}

	if endTimeStr != "" {
		if t, err := h.parseTimeString(endTimeStr); err != nil {
			util.Err(c, fmt.Errorf("invalid endTime format: %v", err))
			return
		} else {
			endTime = &t
		}
	}

	// 从Badger数据库获取文章列表
	var articles []models.ArticleInfo

	// 首先测试是否有任何数据
	var testArticles []models.ArticleInfo
	testQuery := badgerhold.Where("ArticleId").Ne("")
	err := h.articleManager.Find(&testArticles, testQuery)
	if err != nil {
		util.Err(c, fmt.Errorf("测试查询失败: %v", err))
		return
	}
	fmt.Printf("数据库中总文章数: %d\n", len(testArticles))

	query := badgerhold.Where("ArticleId").Ne("")

	// 添加站点ID过滤
	if siteId > 0 {
		query = query.And("SiteId").Eq(strconv.Itoa(siteId))
	}

	// 添加栏目ID过滤
	if columnId > 0 {
		// 对于数组字段，使用正则表达式匹配
		columnIdStr := strconv.Itoa(columnId)
		pattern := ".*" + regexp.QuoteMeta(columnIdStr) + ".*"
		regExp, err := regexp.Compile(pattern)
		if err != nil {
			util.Err(c, fmt.Errorf("栏目ID正则表达式编译错误: %v", err))
			return
		}
		query = query.And("ColumnId").RegExp(regExp)
		fmt.Printf("栏目ID过滤: %s\n", columnIdStr)
	}

	// 添加关键字过滤 - 使用正则表达式实现模糊搜索
	if keyWord != "" {
		// 创建正则表达式，匹配包含关键字的字符串
		pattern := ".*" + regexp.QuoteMeta(keyWord) + ".*"
		regExp, err := regexp.Compile(pattern)
		if err != nil {
			util.Err(c, fmt.Errorf("正则表达式编译错误: %v", err))
			return
		}
		query = query.And("Title").RegExp(regExp)
		// 调试信息
		fmt.Printf("搜索关键字: %s\n", keyWord)
		fmt.Printf("正则表达式模式: %s\n", pattern)
	}

	// 添加时间范围过滤
	if startTime != nil {
		query = query.And("PublishTime").Ge(*startTime)
	}
	if endTime != nil {
		query = query.And("PublishTime").Le(*endTime)
	}

	// 执行查询
	err = h.articleManager.Find(&articles, query)
	if err != nil {
		util.Err(c, fmt.Errorf("查询文章列表失败: %v", err))
		return
	}
	fmt.Printf("查询结果数量: %d\n", len(articles))

	// 获取总数
	total, err := h.articleManager.Count(&models.ArticleInfo{}, query)
	if err != nil {
		util.Err(c, fmt.Errorf("获取文章总数失败: %v", err))
		return
	}

	// 分页处理
	offset := (pager.Page - 1) * pager.PageSize
	if offset >= len(articles) {
		articles = []models.ArticleInfo{}
	} else {
		end := offset + pager.PageSize
		if end > len(articles) {
			end = len(articles)
		}
		articles = articles[offset:end]
	}

	// 转换为响应格式
	list := make([]gin.H, len(articles))
	for i, article := range articles {
		list[i] = gin.H{
			"articleId":      article.ArticleId,
			"title":          article.Title,
			"siteId":         article.SiteId,
			"siteName":       article.SiteName,
			"columnId":       article.ColumnId,
			"columnName":     article.ColumnName,
			"creatorName":    article.CreatorName,
			"summary":        article.Summary,
			"publishTime":    article.PublishTime,
			"lastModifyTime": article.LastModifyTime,
			"visitUrl":       article.VisitUrl,
			"content":        article.Content,
			"attachment":     article.Attachment,
		}
	}

	util.Ok(c, gin.H{
		"count": total,
		"items": list,
		"page":  pager.Page,
		"size":  pager.PageSize,
	})
}

// parsePager 解析分页参数
func (h *Handler) parsePager(c *gin.Context) *Pager {
	var pager Pager
	err := c.ShouldBindQuery(&pager)
	if err != nil {
		return &Pager{Page: 1, PageSize: 10}
	}
	if pager.Page < 1 {
		pager.Page = 1
	}
	if pager.PageSize < 1 {
		pager.PageSize = 10
	}
	if pager.PageSize > 100 {
		pager.PageSize = 100
	}
	return &pager
}

// parseTimeString 解析时间字符串
func (h *Handler) parseTimeString(timeStr string) (time.Time, error) {
	// 支持多种时间格式
	formats := []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000Z",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported time format: %s", timeStr)
}
