package server

import (
	"fmt"
	"strconv"
	"time"
	"webplus-openapi/pkg/store"
	"webplus-openapi/pkg/util"

	"github.com/gin-gonic/gin"
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

	// TODO: 实现从数据库获取文章列表的逻辑
	// 这里暂时返回模拟数据
	list := []gin.H{
		{
			"id":       "1",
			"title":    "示例文章1",
			"siteId":   siteId,
			"columnId": columnId,
		},
		{
			"id":       "2",
			"title":    "示例文章2",
			"siteId":   siteId,
			"columnId": columnId,
			"keyWord":  keyWord,
			"start":    startTime,
			"end":      endTime,
		},
	}
	total := len(list)

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
