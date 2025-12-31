package server

import (
	"fmt"
	"path"
	"regexp"
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
// @Router       /api/v1/webplus/getArticles [post]
func (h *Handler) GetArticles(c *gin.Context) {
	columnIdStr := util.GetParam(c, "columnId")
	var siteIdStr string
	//如果同时传columnId和siteId，则只看columnId
	if columnIdStr == "" {
		siteIdStr = util.GetParam(c, "siteId")
	}
	articleIdStr := util.GetParam(c, "articleId")

	title := util.GetParam(c, "title")
	startTimeStr := util.GetParam(c, "startTime")
	endTimeStr := util.GetParam(c, "endTime")

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

	pageSizeStr := util.GetParam(c, "pageSize")
	if pageSizeStr == "" {
		pageSizeStr = "20"
	}
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	pageStr := util.GetParam(c, "page")
	if pageStr == "" {
		pageStr = "1"
	}
	page, _ := strconv.Atoi(pageStr)
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
		VisitCount     int        `gorm:"column:visitCount"`
		Keywords       string     `gorm:"column:keywords"`
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
		keyword := util.GetParam(c, field) // 自动用字段名作为 query 参数名
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
			VisitCount:     r.VisitCount,
			Keywords:       r.Keywords,
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
		"visitCount":     a.VisitCount,
		"keywords":       a.Keywords,
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

// GetSites 获取站点列表
// @Summary      获取站点列表
// @Description  按站点ID、名称等条件分页获取站点
// @Tags         sites
// @Produce      json
// @Param        siteId   query  string  false  "站点ID，逗号分隔"
// @Param        name     query  string  false  "站点名称模糊搜索"
// @Param        page     query  int     false  "页码，从1开始"
// @Param        pageSize query  int     false  "每页大小"
// @Success      200  {object}  util.Response{data=GetSitesResponse}
// @Router       /api/v1/webplus/getSites [get]
// @Router       /api/v1/webplus/getSites [post]
func (h *Handler) GetSites(c *gin.Context) {
	siteIdStr := util.GetParam(c, "siteId")
	name := util.GetParam(c, "name")

	pageSizeStr := util.GetParam(c, "pageSize")
	if pageSizeStr == "" {
		pageSizeStr = "20"
	}
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	pageStr := util.GetParam(c, "page")
	if pageStr == "" {
		pageStr = "1"
	}
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	targetDB := db.GetTargetDB()
	if targetDB == nil {
		util.Err(c, fmt.Errorf("targetDB 未初始化"))
		return
	}

	// 构建查询
	query := targetDB.Table(models.TableNameTSite)

	// 站点ID过滤
	if siteIdStr != "" {
		validSiteIds, _ := parseIDList(siteIdStr)
		if len(validSiteIds) > 0 {
			query = query.Where("ID IN ?", validSiteIds)
		}
	}

	// 名称模糊搜索
	if name != "" {
		like := "%" + name + "%"
		query = query.Where("NAME LIKE ?", like)
	}

	// 统计总数
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		util.Err(c, fmt.Errorf("统计站点总数失败: %v", err))
		return
	}

	// 分页
	offset := (page - 1) * pageSize
	query = query.Order("ID ASC").Offset(offset).Limit(pageSize)

	var sites []models.TSite
	if err := query.Find(&sites).Error; err != nil {
		util.Err(c, fmt.Errorf("查询站点列表失败: %v", err))
		return
	}

	// 是否还有下一页
	hasNext := int64(page*pageSize) < total

	// 批量查询 T_PUBLISHSITE 表，获取已发布的站点ID（deleted = 0）和完整的发布记录
	publishedSiteIds := make(map[int]bool)
	siteToPublishSiteMap := make(map[int]*models.TPublishSite)     // siteId -> TPublishSite（当前站点的发布记录）
	publishSiteIdToRecordMap := make(map[int]*models.TPublishSite) // publishSiteId -> TPublishSite（所有发布记录，用于查找父记录）
	if len(sites) > 0 {
		// 收集所有站点ID
		siteIds := make([]int, 0, len(sites))
		for _, s := range sites {
			siteIds = append(siteIds, s.Id)
		}

		// 查询 T_PUBLISHSITE 表，找出 siteId 存在且 deleted = 0 的记录，获取完整信息
		var publishSites []models.TPublishSite
		if err := targetDB.Table(models.TableNameTPubSite).
			Where("siteId IN ? AND deleted = ?", siteIds, 0).
			Find(&publishSites).Error; err == nil {
			// 构建映射
			for i := range publishSites {
				ps := &publishSites[i]
				publishedSiteIds[ps.SiteId] = true
				siteToPublishSiteMap[ps.SiteId] = ps
				publishSiteIdToRecordMap[ps.Id] = ps
			}
		}

		// 收集所有需要的 parentId（用于查找父发布记录）
		parentPublishSiteIds := make([]int, 0)
		parentIdSet := make(map[int]bool)
		for _, ps := range publishSites {
			if ps.ParentId > 0 && !parentIdSet[ps.ParentId] {
				parentPublishSiteIds = append(parentPublishSiteIds, ps.ParentId)
				parentIdSet[ps.ParentId] = true
			}
		}

		// 查询父发布记录（通过 parentId 找到父站点的发布记录）
		if len(parentPublishSiteIds) > 0 {
			var parentPublishSites []models.TPublishSite
			if err := targetDB.Table(models.TableNameTPubSite).
				Where("id IN ? AND deleted = ?", parentPublishSiteIds, 0).
				Find(&parentPublishSites).Error; err == nil {
				for i := range parentPublishSites {
					pps := &parentPublishSites[i]
					publishSiteIdToRecordMap[pps.Id] = pps
				}
			}
		}
	}

	// 批量查询父站点信息（用于没有 domainName 的站点）
	parentSiteMap := make(map[int]models.TSite) // siteId -> TSite
	if len(siteToPublishSiteMap) > 0 {
		// 收集所有需要的父站点 siteId
		parentSiteIds := make([]int, 0)
		parentSiteIdSet := make(map[int]bool)
		for _, ps := range siteToPublishSiteMap {
			if ps.ParentId > 0 {
				// 通过 parentId 找到父发布记录
				if parentPublishSite, exists := publishSiteIdToRecordMap[ps.ParentId]; exists {
					parentSiteId := parentPublishSite.SiteId
					if parentSiteId > 0 && !parentSiteIdSet[parentSiteId] {
						parentSiteIds = append(parentSiteIds, parentSiteId)
						parentSiteIdSet[parentSiteId] = true
					}
				}
			}
		}

		// 批量查询父站点
		if len(parentSiteIds) > 0 {
			var parentSites []models.TSite
			if err := targetDB.Table(models.TableNameTSite).
				Where("ID IN ?", parentSiteIds).
				Find(&parentSites).Error; err == nil {
				for _, ps := range parentSites {
					parentSiteMap[ps.Id] = ps
				}
			}
		}
	}

	// 转换为响应格式
	list := make([]SiteInfo, len(sites))
	for i, s := range sites {
		// 根据 T_PUBLISHSITE 表判断是否已发布
		status := 0
		if publishedSiteIds[s.Id] {
			status = 1
		}

		// 处理域名：已发布的站点必须有 URL
		siteUrl := ""
		if status == 1 {
			// 情况1：站点有自己的 domainName
			if s.DomainName != "" {
				re := regexp.MustCompile(`[,，]+`)
				parts := re.Split(s.DomainName, -1)
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						siteUrl = trimmed
						break
					}
				}
			} else {
				// 情况2：通过 T_PUBLISHSITE 的 parentId 找到父站点的 domainName + 自己的 DummyName
				if currentPublishSite, hasPublishSite := siteToPublishSiteMap[s.Id]; hasPublishSite {
					if currentPublishSite.ParentId > 0 {
						// 通过 parentId 找到父发布记录
						if parentPublishSite, exists := publishSiteIdToRecordMap[currentPublishSite.ParentId]; exists {
							// 获取父发布记录的 siteId
							parentSiteId := parentPublishSite.SiteId
							// 查询父站点的 domainName
							if parentSite, exists := parentSiteMap[parentSiteId]; exists {
								// 获取父站点的第一个域名
								if parentSite.DomainName != "" {
									re := regexp.MustCompile(`[,，]+`)
									parts := re.Split(parentSite.DomainName, -1)
									var parentDomain string
									for _, part := range parts {
										trimmed := strings.TrimSpace(part)
										if trimmed != "" {
											parentDomain = trimmed
											break
										}
									}
									// 拼接父站点域名 + 自己的 DummyName
									if parentDomain != "" && s.DummyName != "" {
										siteUrl = parentDomain + "/" + s.DummyName
									} else if parentDomain != "" {
										siteUrl = parentDomain
									}
								}
							}
						}
					}
				}
			}
		}

		logoURL := ""
		if s.Logo != "" && siteUrl != "" {
			logoPath := path.Join("/_upload", s.FilePath, s.Logo)
			logoURL = "http://" + siteUrl + logoPath
		}

		list[i] = SiteInfo{
			SiteId:    s.Id,
			SiteName:  s.Name,
			Status:    status, // 根据 T_PUBLISHSITE 表判断：存在且 deleted = 0 则为 1，否则为 0
			SiteUrl:   siteUrl,
			ShortName: s.ShortName,
			Logo:      logoURL,
		}
	}

	response := GetSitesResponse{
		Found: len(sites) > 0,
		Items: list,
		Pagination: GetColumnsPagination{
			Page:     page,
			PageSize: pageSize,
			HasNext:  hasNext,
			Total:    total,
		},
	}

	util.Ok(c, response)
}

// GetColumns 获取栏目列表
// @Summary      获取栏目列表
// @Description  按站点、父栏目等条件分页获取栏目
// @Tags         columns
// @Produce      json
// @Param        siteId   query  string  false  "站点ID，逗号分隔"
// @Param        parentId query  string  false  "父栏目ID"
// @Param        page     query  int     false  "页码，从1开始"
// @Param        pageSize query  int     false  "每页大小"
// @Param        name     query  string  false  "栏目名称模糊搜索"
// @Success      200  {object}  util.Response{data=GetColumnsResponse}
// @Router       /api/v1/webplus/getColumns [get]
// @Router       /api/v1/webplus/getColumns [post]
func (h *Handler) GetColumns(c *gin.Context) {
	siteIdStr := util.GetParam(c, "siteId")
	parentIdStr := util.GetParam(c, "parentId")
	name := util.GetParam(c, "name")
	showType := util.GetParam(c, "showType")

	pageSizeStr := util.GetParam(c, "pageSize")
	if pageSizeStr == "" {
		pageSizeStr = "20"
	}
	pageSize, _ := strconv.Atoi(pageSizeStr)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	pageStr := util.GetParam(c, "page")
	if pageStr == "" {
		pageStr = "1"
	}
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}

	sourceDB := db.GetTargetDB()
	if sourceDB == nil {
		util.Err(c, fmt.Errorf("sourceDB 未初始化"))
		return
	}

	// 构建查询
	query := sourceDB.Table("T_COLUMN")

	// 站点过滤
	if siteIdStr != "" {
		validSiteIds, _ := parseIDList(siteIdStr)
		if len(validSiteIds) > 0 {
			// 将 int64 转换为 string 进行查询（因为 TColumn.SiteId 是 string 类型）
			siteIdStrs := make([]string, 0, len(validSiteIds))
			for _, id := range validSiteIds {
				siteIdStrs = append(siteIdStrs, strconv.FormatInt(id, 10))
			}
			query = query.Where("siteId IN ?", siteIdStrs)
		}
	}

	// 父栏目过滤
	if parentIdStr != "" {
		query = query.Where("parentId = ?", parentIdStr)
	}

	// 名称模糊搜索
	if name != "" {
		like := "%" + name + "%"
		query = query.Where("name LIKE ?", like)
	}

	if showType == "tree" {
		query = query.Where("parentId = 0 AND id != 1 ")
	}

	// 统计总数
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		util.Err(c, fmt.Errorf("统计栏目总数失败: %v", err))
		return
	}

	// 分页
	offset := (page - 1) * pageSize
	query = query.Order("id ASC").Offset(offset).Limit(pageSize)

	var columns []models.TColumn
	if err := query.Find(&columns).Error; err != nil {
		util.Err(c, fmt.Errorf("查询栏目列表失败: %v", err))
		return
	}

	// 是否还有下一页
	hasNext := int64(page*pageSize) < total

	// 转换为响应格式
	list := make([]ColumnInfo, len(columns))
	for i := range columns {
		//处理URL
		col := &columns[i] // 获取指针，避免拷贝

		if col.Link == "" {
			col.Link = generateUrl(*col) // 注意：如果 generateUrl 需要值，就传 *col；如果接受指针，就传 col
		}
		//处理全路径
		allPathIds := h.extractIdsFromPath(col.Path)
		columnIdToName := make(map[int]string)
		if len(allPathIds) > 0 {
			var pathColumns []models.TColumn
			if err := h.db.Table(models.TableNameTColumn).
				Where("id IN ?", allPathIds).
				Select("id, name").
				Find(&pathColumns).Error; err == nil {
				for _, pathCol := range pathColumns {
					columnIdToName[pathCol.Id] = pathCol.Name
				}
			}
		}
		// 将数字 path 转换为中文 path
		chinesePath := h.convertPathToChineseWithCache(col.Path, columnIdToName, col, allPathIds)

		list[i] = ColumnInfo{
			ColumnId:       col.Id,
			ColumnName:     col.Name,
			ParentColumnId: col.ParentId,
			ColumnUrl:      col.Link,
			Path:           chinesePath,
			Sort:           col.Sort,
			Status:         col.Navigation, //暂时按哈工大的需求来。如果是导航栏目就是显示
		}
	}

	response := GetColumnsResponse{
		Found: len(columns) > 0,
		Items: list,
		Pagination: GetColumnsPagination{
			Page:     page,
			PageSize: pageSize,
			HasNext:  hasNext,
			Total:    total,
		},
	}

	util.Ok(c, response)
}

// extractIdsFromPath 从 path 中提取所有 ID
func (h *Handler) extractIdsFromPath(path string) []int {
	if path == "" {
		return nil
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if id, err := strconv.Atoi(part); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// convertPathToChineseWithCache 将数字路径转换为中文路径（使用缓存）
func (h *Handler) convertPathToChineseWithCache(path string, columnIdToName map[int]string, col *models.TColumn, ids []int) string {
	if path == "" {
		return ""
	}

	// 处理单独的 / 的情况，返回 /系统站点
	if col.Id == 1 {
		return "/系统站点"
	}

	// 如果提取不到 ID，返回默认格式
	if len(ids) == 0 {
		return "/系统站点" + "/" + col.Name
	}

	var chineseParts []string
	for i := 0; i < len(ids); i++ {
		id := ids[i]
		if name, exists := columnIdToName[id]; exists {
			chineseParts = append(chineseParts, name)
		}
	}

	// 拼接成路径格式: /系统站点/栏目1/栏目2/当前栏目名称
	var result string
	if len(chineseParts) > 0 {
		result = "/系统站点/" + strings.Join(chineseParts, "/") + "/" + col.Name
	} else {
		result = "/系统站点/" + col.Name
	}
	return result
}

// getSiteDomainName 获取站点的域名，支持通过 T_PUBLISHSITE 的 parentId 查找父站点域名
// siteId: 站点ID
// dummyName: 可选的虚拟名称，如果提供了且站点没有自己的域名，会拼接到父站点域名后面
// 返回: 域名（不包含协议），如果找不到则返回空字符串
func getSiteDomainName(siteId int) string {
	targetDB := db.GetTargetDB()
	if targetDB == nil {
		return ""
	}

	// 查询站点信息
	var site models.TSite
	if err := targetDB.Table(models.TableNameTSite).Where("ID = ?", siteId).First(&site).Error; err != nil {
		return ""
	}

	// 情况1：站点有自己的 domainName
	if site.DomainName != "" {
		re := regexp.MustCompile(`[,，]+`)
		parts := re.Split(site.DomainName, -1)
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				return trimmed
			}
		}
	}

	// 情况2：站点没有 domainName，通过 T_PUBLISHSITE 的 parentId 查找父站点域名
	dummyName := site.DummyName
	var publishSite models.TPublishSite
	if err := targetDB.Table(models.TableNameTPubSite).
		Where("siteId = ? AND deleted = ?", siteId, 0).
		First(&publishSite).Error; err != nil {
		return ""
	}

	// 如果当前站点有 parentId
	if publishSite.ParentId > 0 {
		// 通过 parentId 找到父发布记录
		var parentPublishSite models.TPublishSite
		if err := targetDB.Table(models.TableNameTPubSite).
			Where("id = ? AND deleted = ?", publishSite.ParentId, 0).
			First(&parentPublishSite).Error; err != nil {
			return ""
		}

		// 获取父发布记录的 siteId
		parentSiteId := parentPublishSite.SiteId
		if parentSiteId > 0 {
			// 查询父站点信息
			var parentSite models.TSite
			if err := targetDB.Table(models.TableNameTSite).
				Where("ID = ?", parentSiteId).
				First(&parentSite).Error; err != nil {
				return ""
			}

			// 获取父站点的第一个域名
			if parentSite.DomainName != "" {
				re := regexp.MustCompile(`[,，]+`)
				parts := re.Split(parentSite.DomainName, -1)
				var parentDomain string
				for _, part := range parts {
					trimmed := strings.TrimSpace(part)
					if trimmed != "" {
						parentDomain = trimmed
						break
					}
				}

				// 拼接父站点域名 + 自己的 DummyName（如果提供了）
				if parentDomain != "" {
					if dummyName != "" {
						return parentDomain + "/" + dummyName
					}
					return parentDomain
				}
			}
		}
	}

	return ""
}

func generateUrl(column models.TColumn) string {
	targetDB := db.GetTargetDB()
	if targetDB == nil {
		return ""
	}

	// 使用可复用的函数获取站点域名
	domainName := getSiteDomainName(column.SiteId)
	if domainName == "" {
		return ""
	}

	url := domainName + "/" + column.UrlName + "/list.htm"
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	return url
}
