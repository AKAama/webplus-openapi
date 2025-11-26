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
	articleIdStr := c.Query("articleId")

	// 转换 siteId
	var siteId = 0
	if siteIdStr != "" {
		if parsed, err := strconv.Atoi(siteIdStr); err == nil {
			siteId = parsed
		}
	}

	keyWord := c.Query("keyWord")
	startTimeStr := c.Query("startTime")
	endTimeStr := c.Query("endTime")

	loc, _ := time.LoadLocation("Asia/Shanghai") //统一为北京时间
	timeFormats := []string{
		time.RFC3339,
		"2006-01-02T15:04:05 -07:00",
		"2006-01-02T15:04:05-07:00",
		"2006-01-02 15:04:05",
		time.RFC3339Nano,
		"2006-01-02",
	}

	var startTime, endTime *time.Time
	if startTimeStr != "" {
		var parsed time.Time
		var err error
		for _, f := range timeFormats {
			if parsed, err = time.Parse(f, startTimeStr); err == nil {
				parsed = parsed.In(loc)
				if f == "2006-01-02" {
					parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, loc)
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
				parsed = parsed.In(loc)
				if f == "2006-01-02" {
					parsed = time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 0, loc)
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

	cursor := c.Query("cursor")
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := badgerhold.Where("ArticleId").Ne("")

	// articleId 精确过滤
	if articleIdStr != "" {
		query = query.And("ArticleId").Eq(articleIdStr)
	}

	if siteId > 0 {
		query = query.And("SiteId").Eq(strconv.Itoa(siteId))
	}

	// 关键字模糊匹配
	if keyWord != "" {
		pattern := ".*" + regexp.QuoteMeta(keyWord) + ".*"
		regExp, err := regexp.Compile(pattern)
		if err != nil {
			util.Err(c, fmt.Errorf("正则表达式编译错误: %v", err))
			return
		}
		query = query.And("Title").RegExp(regExp)
	}

	// 时间范围过滤
	if startTime != nil {
		query = query.And("PublishTime").Ge(*startTime)
	}
	if endTime != nil {
		query = query.And("PublishTime").Le(*endTime)
	}

	// 游标解析
	var cursorTime time.Time
	if cursor != "" {
		parts := strings.Split(cursor, ",")
		if len(parts) == 2 {
			cursorTimeStr := parts[0]
			for _, f := range timeFormats {
				if t, err := time.Parse(f, cursorTimeStr); err == nil {
					cursorTime = t.In(loc)
					break
				}
			}
			query = query.And("PublishTime").Lt(cursorTime)
		}
	}

	// 查询（不含栏目过滤）
	var allArticles []models.ArticleInfo
	err := h.articleManager.Find(&allArticles, query)
	if err != nil {
		util.Err(c, fmt.Errorf("查询文章列表失败: %v", err))
		return
	}

	// Go 层栏目过滤
	if columnIdStr != "" {
		columnIds := strings.Split(columnIdStr, ",")
		var validColumnIds []string
		for _, id := range columnIds {
			if s := strings.TrimSpace(id); s != "" {
				validColumnIds = append(validColumnIds, s)
			}
		}
		if len(validColumnIds) > 0 {
			var filtered []models.ArticleInfo
			for _, art := range allArticles {
				for _, cid := range art.ColumnId {
					for _, id := range validColumnIds {
						if cid == id {
							filtered = append(filtered, art)
							goto Next
						}
					}
				}
			Next:
			}
			allArticles = filtered
		}
	}

	// 排序（时间倒序）
	sort.Slice(allArticles, func(i, j int) bool {
		if allArticles[i].PublishTime == nil && allArticles[j].PublishTime == nil {
			return allArticles[i].ArticleId > allArticles[j].ArticleId
		}
		if allArticles[i].PublishTime == nil {
			return false
		}
		if allArticles[j].PublishTime == nil {
			return true
		}
		if allArticles[i].PublishTime.Equal(*allArticles[j].PublishTime) {
			return allArticles[i].ArticleId > allArticles[j].ArticleId
		}
		return allArticles[i].PublishTime.After(*allArticles[j].PublishTime)
	})

	hasNext := len(allArticles) > pageSize
	if hasNext {
		allArticles = allArticles[:pageSize]
	}

	var nextCursor string
	if hasNext && len(allArticles) > 0 {
		last := allArticles[len(allArticles)-1]
		if last.PublishTime != nil {
			nextCursor = fmt.Sprintf("%s,%s", last.PublishTime.UTC().Format(time.RFC3339), last.ArticleId)
		}
	}

	list := make([]gin.H, len(allArticles))
	for i, a := range allArticles {
		list[i] = gin.H{
			"articleId":      a.ArticleId,
			"title":          a.Title,
			"siteId":         a.SiteId,
			"siteName":       a.SiteName,
			"columnId":       a.ColumnId,
			"columnName":     a.ColumnName,
			"creatorName":    a.CreatorName,
			"firstImgPath":   a.FirstImgPath,
			"summary":        a.Summary,
			"publishTime":    a.PublishTime,
			"lastModifyTime": a.LastModifyTime,
			"visitUrl":       a.VisitUrl,
			"content":        a.Content,
			"attachment":     a.Attachment,
		}
		injectArticleFields(list[i], a.ArticleFields)
	}

	util.Ok(c, gin.H{
		"found": len(allArticles) > 0,
		"items": list,
		"pagination": gin.H{
			"pageSize":   pageSize,
			"hasNext":    hasNext,
			"nextCursor": nextCursor,
			"cursor":     cursor,
		},
	})
}

func injectArticleFields(target gin.H, fields models.ArticleFields) {
	for key, value := range fields.ToMap() {
		target[key] = value
	}
}
