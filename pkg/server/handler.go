package server

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

// GetArticles 获取文章列表 - 使用游标分页
func (h *Handler) GetArticles(c *gin.Context) {
	columnIdStr := c.Query("columnId")
	siteIdStr := c.Query("siteId")

	// 转换ID类型
	var siteId = 0
	if siteIdStr != "" {
		if parsed, err := strconv.Atoi(siteIdStr); err == nil {
			siteId = parsed
		}
	}

	// 检查是否有关键字
	keyWord := c.Query("keyWord")
	startTimeStr := c.Query("startTime")
	endTimeStr := c.Query("endTime")

	// 统一的时间解析格式（供 startTime/endTime 以及 cursor 使用）
	timeFormats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05 -07:00",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		"2006-01-02",
	}

	var startTime, endTime *time.Time

	// 可选的时间范围过滤：startTime <= publishTime <= endTime
	if startTimeStr != "" {
		var parsed time.Time
		var err error
		for _, f := range timeFormats {
			if parsed, err = time.Parse(f, startTimeStr); err == nil {
				// 如果是纯日期格式，补充起始时间 00:00:00
				if f == "2006-01-02" {
					parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, parsed.Location())
				}
				break
			}
		}
		if err != nil {
			util.Err(c, fmt.Errorf("invalid startTime: %s", startTimeStr))
			return
		}
		startTime = &parsed
	}
	if endTimeStr != "" {
		var parsed time.Time
		var err error
		for _, f := range timeFormats {
			if parsed, err = time.Parse(f, endTimeStr); err == nil {
				// 如果是纯日期格式，补充结束时间 23:59:59
				if f == "2006-01-02" {
					parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, parsed.Location())
				}
				break
			}
		}
		if err != nil {
			util.Err(c, fmt.Errorf("invalid endTime: %s", endTimeStr))
			return
		}
		endTime = &parsed
	}

	// 游标分页参数处理
	cursor := c.Query("cursor") // 复合游标: "时间戳,ArticleID"
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))

	// 参数验证
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// 构建查询条件
	query := badgerhold.Where("ArticleId").Ne("")

	// 添加站点ID过滤
	if siteId > 0 {
		query = query.And("SiteId").Eq(strconv.Itoa(siteId))
	}

	// 添加栏目ID过滤 - 支持多个栏目ID（逗号分隔）
	if columnIdStr != "" {
		columnIds := strings.Split(columnIdStr, ",")
		// 过滤空字符串
		var validColumnIds []string
		for _, id := range columnIds {
			if strings.TrimSpace(id) != "" {
				validColumnIds = append(validColumnIds, strings.TrimSpace(id))
			}
		}
		if len(validColumnIds) > 0 {
			// 对于数组字段，使用正则表达式匹配多个栏目ID
			// 构建匹配任意一个栏目ID的正则表达式
			var patterns []string
			for _, columnId := range validColumnIds {
				patterns = append(patterns, ".*"+regexp.QuoteMeta(columnId)+".*")
			}
			// 使用 OR 逻辑：匹配任意一个栏目ID
			pattern := strings.Join(patterns, "|")
			regExp, err := regexp.Compile(pattern)
			if err != nil {
				util.Err(c, fmt.Errorf("栏目ID正则表达式编译错误: %v", err))
				return
			}
			query = query.And("ColumnId").RegExp(regExp)
		}
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
	}

	// 添加时间范围过滤
	if startTime != nil {
		query = query.And("PublishTime").Ge(*startTime)
	}
	if endTime != nil {
		query = query.And("PublishTime").Le(*endTime)
	}

	// 解析复合游标
	var cursorTime time.Time
	if cursor != "" {
		parts := strings.Split(cursor, ",")
		if len(parts) != 2 {
			util.Err(c, fmt.Errorf("invalid cursor: %s", cursor))
			return
		}

		cursorTimeStr, _ := parts[0], parts[1] // 暂时不使用 cursorID，因为 badgerhold 不支持复杂条件

		// 解析时间
		var err error
		for _, format := range timeFormats {
			if cursorTime, err = time.Parse(format, cursorTimeStr); err == nil {
				break
			}
		}

		if err != nil {
			util.Err(c, err)
			return
		}

		// 添加复合条件：时间小于游标时间
		// 注意：badgerhold 不支持复杂的 OR 条件，这里只使用时间条件
		query = query.And("PublishTime").Lt(cursorTime)
	}

	// 执行查询 - 获取比需要多一条的数据来判断是否有下一页
	var articles []models.ArticleInfo
	err := h.articleManager.Find(&articles, query)
	if err != nil {
		util.Err(c, fmt.Errorf("查询文章列表失败: %v", err))
		return
	}

	// 手动排序：先按时间倒序，再按ID倒序（确保唯一性和一致性）
	sort.Slice(articles, func(i, j int) bool {
		if articles[i].PublishTime == nil && articles[j].PublishTime == nil {
			return articles[i].ArticleId > articles[j].ArticleId
		}
		if articles[i].PublishTime == nil {
			return false
		}
		if articles[j].PublishTime == nil {
			return true
		}
		if articles[i].PublishTime.Equal(*articles[j].PublishTime) {
			return articles[i].ArticleId > articles[j].ArticleId
		}
		return articles[i].PublishTime.After(*articles[j].PublishTime)
	})

	// 判断是否有下一页
	hasNext := len(articles) > pageSize
	if hasNext {
		articles = articles[:pageSize] // 移除多查询的那一条
	}

	// 计算下一页游标：时间,ArticleID
	var nextCursor string
	if hasNext && len(articles) > 0 {
		lastItem := articles[len(articles)-1]
		if lastItem.PublishTime != nil {
			timeStr := lastItem.PublishTime.UTC().Format(time.RFC3339)
			nextCursor = fmt.Sprintf("%s,%s", timeStr, lastItem.ArticleId)
		}
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
			"firstImgPath":   article.FirstImgPath,
			"summary":        article.Summary,
			"publishTime":    article.PublishTime,
			"lastModifyTime": article.LastModifyTime,
			"visitUrl":       article.VisitUrl,
			"content":        article.Content,
			"attachment":     article.Attachment,
		}
	}

	// 返回游标分页结果
	util.Ok(c, gin.H{
		"found": len(articles) > 0,
		"items": list,
		"pagination": gin.H{
			"pageSize":   pageSize,
			"hasNext":    hasNext,
			"nextCursor": nextCursor,
			"cursor":     cursor,
		},
	})
}
