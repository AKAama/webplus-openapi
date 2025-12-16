package server

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"webplus-openapi/pkg/db"
	"webplus-openapi/pkg/models"
	"webplus-openapi/pkg/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Handler v1版本API处理器
type Handler struct {
	cfg Config
	db  *gorm.DB // 来自 targetDB 的只读 MySQL
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

	// 使用 db_storage 中的 article / article_column 表进行查询
	db := db.GetTargetDB()
	if db == nil {
		util.Err(c, fmt.Errorf("db_storage 未初始化"))
		return
	}

	// 1. 如果有栏目过滤，先根据 article_column 查出符合条件的 articleId 列表
	var articleIDsByColumn []int64
	if columnIdStr != "" {
		columnIds := strings.Split(columnIdStr, ",")
		var validColumnIds []int64
		for _, id := range columnIds {
			if s := strings.TrimSpace(id); s != "" {
				if v, err := strconv.ParseInt(s, 10, 64); err == nil {
					validColumnIds = append(validColumnIds, v)
				}
			}
		}
		if len(validColumnIds) == 0 {
			// 没有合法的栏目ID，直接返回空
			util.Ok(c, gin.H{
				"found":      false,
				"items":      []gin.H{},
				"pagination": gin.H{"pageSize": pageSize, "hasNext": false, "nextCursor": "", "cursor": cursor},
			})
			return
		}

		// 从 article_column 中查出匹配的文章ID
		if err := db.Table("article_column").
			Select("DISTINCT article_id").
			Where("column_id IN ?", validColumnIds).
			Scan(&articleIDsByColumn).Error; err != nil {
			util.Err(c, fmt.Errorf("根据栏目过滤文章失败: %v", err))
			return
		}
		if len(articleIDsByColumn) == 0 {
			util.Ok(c, gin.H{
				"found":      false,
				"items":      []gin.H{},
				"pagination": gin.H{"pageSize": pageSize, "hasNext": false, "nextCursor": "", "cursor": cursor},
			})
			return
		}
	}

	// 2. 构建 article 查询
	type ArticleRow struct {
		ArticleId      int64      `gorm:"column:article_id"`
		SiteId         int64      `gorm:"column:site_id"`
		SiteName       string     `gorm:"column:site_name"`
		Title          string     `gorm:"column:title"`
		Summary        string     `gorm:"column:summary"`
		CreatorName    string     `gorm:"column:creator_name"`
		PublishTime    *time.Time `gorm:"column:publish_time"`
		LastModifyTime *time.Time `gorm:"column:last_modify_time"`
		VisitUrl       string     `gorm:"column:visit_url"`
		FirstImgPath   string     `gorm:"column:first_img_path"`
		Content        string     `gorm:"column:content"`
	}

	query := db.Table("article")

	// 按栏目过滤
	if len(articleIDsByColumn) > 0 {
		query = query.Where("article_id IN ?", articleIDsByColumn)
	}

	// siteId 过滤
	if siteId > 0 {
		query = query.Where("site_id = ?", siteId)
	}

	// articleId 精确过滤
	if articleIdStr != "" {
		if aid, err := strconv.ParseInt(articleIdStr, 10, 64); err == nil {
			query = query.Where("article_id = ?", aid)
		} else {
			util.Err(c, fmt.Errorf("articleId 必须为数字: %s", articleIdStr))
			return
		}
	}

	// 关键字模糊匹配（标题）
	if keyWord != "" {
		like := "%" + keyWord + "%"
		query = query.Where("title LIKE ?", like)
	}

	// 时间范围过滤
	if startTime != nil {
		query = query.Where("publish_time >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("publish_time <= ?", *endTime)
	}

	// 游标解析（基于 publish_time）
	if cursor != "" {
		parts := strings.Split(cursor, ",")
		if len(parts) >= 1 {
			cursorTimeStr := parts[0]
			for _, f := range timeFormats {
				if t, err := time.Parse(f, cursorTimeStr); err == nil {
					t = t.In(loc)
					query = query.Where("publish_time < ?", t)
					break
				}
			}
		}
	}

	// 排序 + 分页，多取一条判断 hasNext
	query = query.Order("publish_time DESC, article_id DESC").Limit(pageSize + 1)

	var rows []ArticleRow
	if err := query.Scan(&rows).Error; err != nil {
		util.Err(c, fmt.Errorf("查询文章列表失败: %v", err))
		return
	}

	hasNext := len(rows) > pageSize
	if hasNext {
		rows = rows[:pageSize]
	}

	// 3. 批量查询栏目数据并组装 ColumnId/ColumnName
	var articleIDs []int64
	for _, r := range rows {
		articleIDs = append(articleIDs, r.ArticleId)
	}

	type ColumnRow struct {
		ArticleId  int64  `gorm:"column:article_id"`
		ColumnId   int64  `gorm:"column:column_id"`
		ColumnName string `gorm:"column:column_name"`
	}
	columnMap := make(map[int64][]ColumnRow)
	if len(articleIDs) > 0 {
		var colRows []ColumnRow
		if err := db.Table("article_column").
			Select("article_id, column_id, column_name").
			Where("article_id IN ?", articleIDs).
			Scan(&colRows).Error; err != nil {
			util.Err(c, fmt.Errorf("查询文章栏目失败: %v", err))
			return
		}
		for _, cr := range colRows {
			columnMap[cr.ArticleId] = append(columnMap[cr.ArticleId], cr)
		}
	}

	// 4. 组装为 ArticleInfo
	allArticles := make([]models.ArticleInfo, 0, len(rows))
	for _, r := range rows {
		a := models.ArticleInfo{
			ArticleId:      strconv.FormatInt(r.ArticleId, 10),
			SiteId:         strconv.FormatInt(r.SiteId, 10),
			SiteName:       r.SiteName,
			Title:          r.Title,
			Summary:        r.Summary,
			CreatorName:    r.CreatorName,
			PublishTime:    r.PublishTime,
			LastModifyTime: r.LastModifyTime,
			VisitUrl:       r.VisitUrl,
			FirstImgPath:   r.FirstImgPath,
			Content:        r.Content,
			ColumnId:       []string{},
			ColumnName:     []string{},
		}

		if cols, ok := columnMap[r.ArticleId]; ok {
			// 保持按 columnId 升序
			sort.Slice(cols, func(i, j int) bool { return cols[i].ColumnId < cols[j].ColumnId })
			for _, cRow := range cols {
				a.ColumnId = append(a.ColumnId, strconv.FormatInt(cRow.ColumnId, 10))
				a.ColumnName = append(a.ColumnName, cRow.ColumnName)
			}
		}
		allArticles = append(allArticles, a)
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

	var nextCursor string
	if hasNext && len(allArticles) > 0 {
		last := allArticles[len(allArticles)-1]
		if last.PublishTime != nil {
			nextCursor = fmt.Sprintf("%s,%s", last.PublishTime.UTC().Format(time.RFC3339), last.ArticleId)
		}
	}

	list := make([]gin.H, len(allArticles))
	for i, a := range allArticles {
		item := h.buildArticleResponse(a)
		list[i] = item
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

// buildArticleResponse 根据配置构建文章响应数据
func (h *Handler) buildArticleResponse(a models.ArticleInfo) gin.H {
	// 构建完整的响应数据
	fullData := gin.H{
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

	// 注入扩展字段
	injectArticleFields(fullData, a.ArticleFields)

	// 如果没有配置字段过滤，返回所有字段
	if h.cfg.ResponseFields == nil || len(h.cfg.ResponseFields.EnabledFields) == 0 {
		return fullData
	}

	// 根据配置过滤字段
	enabledFields := make(map[string]bool)
	for _, field := range h.cfg.ResponseFields.EnabledFields {
		enabledFields[strings.ToLower(field)] = true
	}

	// 构建过滤后的响应数据
	filteredData := make(gin.H)
	for key, value := range fullData {
		// 字段名转换为小写进行匹配
		keyLower := strings.ToLower(key)
		if enabledFields[keyLower] {
			filteredData[key] = value
		}
	}

	return filteredData
}

func injectArticleFields(target gin.H, fields models.ArticleFields) {
	for key, value := range fields.ToMap() {
		target[key] = value
	}
}
