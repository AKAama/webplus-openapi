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

// GetArticles 获取文章列表
// @Summary      获取文章列表
// @Description  按栏目、站点、时间分页获取文章
// @Tags         articles
// @Produce      json
// @Param        columnId  query  string  false  "栏目ID，逗号分隔"
// @Param        siteId    query  string  false  "站点ID，逗号分隔"
// @Param        page      query  int     false  "页码，从1开始"
// @Param        pageSize  query  int     false  "每页大小"
// @Param        title     query  string  false  "标题模糊搜索"
// @Param        startTime query  string  false  "开始时间，格式: 2025-01-01"
// @Param        endTime   query  string  false  "结束时间，格式: 2025-01-01"
// @Param        articleId query  string  false  "文章ID"
// @Param        fuzzyField query  string  false  "模糊搜索字段，逗号分隔"
// @Success      200  {object}  util.Response
// @Router       /api/v1/webplus/getArticles [get]
func (h *Handler) GetArticles(c *gin.Context) {
	columnIdStr := c.Query("columnId")
	var siteIdStr string
	//如果同时传columnId和siteId，则只看columnId
	if columnIdStr == "" {
		siteIdStr = c.Query("siteId")
	}
	articleIdStr := c.Query("articleId")

	title := c.Query("title")
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

	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}

	targetDB := db.GetTargetDB()
	if targetDB == nil {
		util.Err(c, fmt.Errorf("targetDB 未初始化"))
		return
	}
	// 1. 如果有栏目 / 站点过滤，先在 article_dynamic 中过滤出文章ID
	var (
		articleIDsByColumnId []int64
		articleIDsBySiteId   []int64
		filterColumnId       string
	)

	// 1.1 栏目过滤
	if columnIdStr != "" {
		validColumnIds, first := parseIDList(columnIdStr)
		if len(validColumnIds) == 0 {
			util.Ok(c, gin.H{"found": false, "items": []gin.H{}, "pagination": gin.H{"pageSize": pageSize}})
			return
		}
		filterColumnId = first

		if err := targetDB.Table(models.TableNameArticleDynamic).
			Select("DISTINCT articleId").
			Where("columnId IN ?", validColumnIds).
			Scan(&articleIDsByColumnId).Error; err != nil {
			util.Err(c, fmt.Errorf("根据栏目过滤文章失败: %v", err))
			return
		}
		if len(articleIDsByColumnId) == 0 {
			util.Ok(c, gin.H{"found": false, "items": []gin.H{}, "pagination": gin.H{"pageSize": pageSize}})
			return
		}
	}

	// 1.2 站点过滤（当未传 columnId 时生效）
	if columnIdStr == "" && siteIdStr != "" {
		validSiteIds, _ := parseIDList(siteIdStr)
		if len(validSiteIds) == 0 {
			util.Ok(c, gin.H{"found": false, "items": []gin.H{}, "pagination": gin.H{"pageSize": pageSize}})
			return
		}

		if err := targetDB.Table(models.TableNameArticleDynamic).
			Select("DISTINCT articleId").
			Where("siteId IN ?", validSiteIds).
			Scan(&articleIDsBySiteId).Error; err != nil {
			util.Err(c, fmt.Errorf("根据站点过滤文章失败: %v", err))
			return
		}
		if len(articleIDsBySiteId) == 0 {
			util.Ok(c, gin.H{"found": false, "items": []gin.H{}, "pagination": gin.H{"pageSize": pageSize}})
			return
		}
	}

	// 2. 构建 article_static 查询
	type ArticleRow struct {
		ArticleId      int64      `gorm:"column:articleId"`
		Title          string     `gorm:"column:title"`
		Summary        string     `gorm:"column:summary"`
		CreatorName    string     `gorm:"column:creatorName"`
		PublishTime    *time.Time `gorm:"column:publishTime"`
		LastModifyTime *time.Time `gorm:"column:lastModifyTime"`
		FirstImgPath   string     `gorm:"column:firstImgPath"`
		Content        string     `gorm:"column:content"`
		VisitUrl       string     `gorm:"column:visitUrl"`
	}

	query := targetDB.Table(models.TableNameArticleStatic)

	// 按栏目过滤
	if len(articleIDsByColumnId) > 0 {
		query = query.Where("articleId IN ?", articleIDsByColumnId)
	}

	if len(articleIDsBySiteId) > 0 {
		query = query.Where("articleId IN ?", articleIDsBySiteId)
	}

	// articleId 精确过滤
	if articleIdStr != "" {
		if aid, err := strconv.ParseInt(articleIdStr, 10, 64); err == nil {
			query = query.Where("articleId = ?", aid)
		} else {
			util.Err(c, fmt.Errorf("articleId 必须为数字: %s", articleIdStr))
			return
		}
	}

	// 关键字模糊匹配（标题）
	if title != "" {
		like := "%" + title + "%"
		query = query.Where("title LIKE ?", like)
	}

	for _, field := range h.cfg.Search.FuzzyField {
		keyword := c.Query(field) // 自动用字段名作为 query 参数名
		if keyword != "" {
			likeValue := "%" + strings.TrimSpace(keyword) + "%"
			query = query.Where(fmt.Sprintf("%s LIKE ?", field), likeValue)
		}
	}

	// 时间范围过滤
	if startTime != nil {
		query = query.Where("publishTime >= ?", *startTime)
	}
	if endTime != nil {
		query = query.Where("publishTime <= ?", *endTime)
	}

	// 3. 统计总数（不分页）
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		util.Err(c, fmt.Errorf("统计文章总数失败: %v", err))
		return
	}

	// 排序 + 分页（基于 page/pageSize）
	offset := (page - 1) * pageSize
	query = query.Order("publishTime DESC, articleId DESC").Offset(offset).Limit(pageSize)

	var rows []ArticleRow
	if err := query.Scan(&rows).Error; err != nil {
		util.Err(c, fmt.Errorf("查询文章列表失败: %v", err))
		return
	}

	// 是否还有下一页
	hasNext := int64(page*pageSize) < total

	// 3. 批量查询栏目数据并组装 Id/Name，并查询附件
	var articleIDs []int64
	for _, r := range rows {
		articleIDs = append(articleIDs, r.ArticleId)
	}

	columnMap := make(map[int64][]models.Column)
	attachMap := make(map[int64][]models.Attachment)
	if len(articleIDs) > 0 {
		var colRows []models.Column
		if err := targetDB.Table(models.TableNameArticleDynamic).
			Select("articleId, columnId, columnName,siteId, siteName,Url as url").
			Where("articleId IN ?", articleIDs).
			Scan(&colRows).Error; err != nil {
			util.Err(c, fmt.Errorf("查询文章栏目和站点失败: %v", err))
			return
		}
		for _, cr := range colRows {
			columnMap[cr.ArticleId] = append(columnMap[cr.ArticleId], cr)
		}

		var attRows []models.ArticleAttachment
		if err := targetDB.Table(models.TableNameArticleAttachment).
			Select("articleId, name, path").
			Where("articleId IN ?", articleIDs).
			Scan(&attRows).Error; err != nil {
			util.Err(c, fmt.Errorf("查询文章附件失败: %v", err))
			return
		}
		for _, ar := range attRows {
			attachMap[ar.ArticleId] = append(attachMap[ar.ArticleId], models.Attachment{
				Name: ar.Name,
				Path: ar.Path,
			})
		}
	}

	// 4. 组装为 ArticleInfo
	allArticles := make([]models.ArticleInfo, 0, len(rows))
	for _, r := range rows {
		a := models.ArticleInfo{
			ArticleId:      strconv.FormatInt(r.ArticleId, 10),
			Title:          r.Title,
			Summary:        r.Summary,
			CreatorName:    r.CreatorName,
			PublishTime:    r.PublishTime,
			LastModifyTime: r.LastModifyTime,
			VisitUrl:       r.VisitUrl,
			FirstImgPath:   r.FirstImgPath,
			Content:        r.Content,
		}

		if cols, ok := columnMap[r.ArticleId]; ok {
			// 保持按 columnId 升序
			sort.Slice(cols, func(i, j int) bool { return cols[i].ColumnId < cols[j].ColumnId })
			for _, cRow := range cols {
				a.ColumnId = append(a.ColumnId, strconv.FormatInt(int64(cRow.ColumnId), 10))
				a.ColumnName = append(a.ColumnName, cRow.ColumnName)
			}
		}
		if atts, ok := attachMap[r.ArticleId]; ok {
			a.Attachment = atts
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

	list := make([]gin.H, len(allArticles))
	for i, a := range allArticles {
		item := h.buildArticleResponse(a)

		// 组装栏目数组 [{columnId,columnName,url},...]
		articleIDInt, _ := strconv.ParseInt(a.ArticleId, 10, 64)
		if cols, ok := columnMap[articleIDInt]; ok {
			columnsArr := make([]gin.H, 0, len(cols))
			for _, cRow := range cols {
				columnsArr = append(columnsArr, gin.H{
					"columnId":   strconv.FormatInt(int64(cRow.ColumnId), 10),
					"columnName": cRow.ColumnName,
					"siteId":     cRow.SiteId,
					"siteName":   cRow.SiteName,
					"url":        cRow.Url,
				})
			}
			item["columnInfo"] = columnsArr

			// 如果按 columnId 精确过滤，优先使用对应栏目的 URL 覆盖 visitUrl
			if filterColumnId != "" {
				for _, cRow := range cols {
					if strconv.FormatInt(int64(cRow.ColumnId), 10) == filterColumnId && cRow.Url != "" {
						item["visitUrl"] = cRow.Url
						break
					}
				}
			}
		}

		list[i] = item
	}

	util.Ok(c, gin.H{
		"found": len(allArticles) > 0,
		"items": list,
		"pagination": gin.H{
			"page":     page,
			"pageSize": pageSize,
			"hasNext":  hasNext,
			"total":    total,
		},
	})
}

// parseIDList 将逗号分隔的 ID 字符串解析为 int64 列表，并返回第一个合法 ID 的原始字符串
func parseIDList(s string) ([]int64, string) {
	parts := strings.Split(s, ",")
	var (
		result   []int64
		firstRaw string
	)
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			if firstRaw == "" {
				firstRaw = t
			}
			if v, err := strconv.ParseInt(t, 10, 64); err == nil {
				result = append(result, v)
			}
		}
	}
	return result, firstRaw
}

// buildArticleResponse 根据配置构建文章响应数据
func (h *Handler) buildArticleResponse(a models.ArticleInfo) gin.H {
	// 构建完整的响应数据
	fullData := gin.H{
		"articleId":      a.ArticleId,
		"title":          a.Title,
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
