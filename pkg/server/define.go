package server

import "gorm.io/gorm"

// Handler v1版本API处理器
type Handler struct {
	cfg Config
	db  *gorm.DB // 来自 targetDB 的只读 MySQL
}

// ColumnInfo 栏目信息响应结构体
type ColumnInfo struct {
	ColumnId       int    `json:"columnId"`       // 栏目ID
	ColumnName     string `json:"columnName"`     // 栏目名称
	ParentColumnId int    `json:"parentColumnId"` // 父栏目ID
	ColumnUrl      string `json:"columnUrl"`      // 栏目链接
	Path           string `json:"path"`           // 栏目路径
	Sort           int    `json:"sort"`           // 栏目排序
	Status         int    `json:"status"`
}

type SiteInfo struct {
	SiteId    int    `json:"siteId"`
	SiteName  string `json:"siteName"`
	Status    int    `json:"status"`
	SiteUrl   string `json:"siteUrl"`
	ShortName string `json:"shortName"`
	Logo      string `json:"logo"`
}

// GetColumnsResponse GetColumns API 响应结构体
type GetColumnsResponse struct {
	Found      bool                 `json:"found"`      // 是否找到数据
	Items      []ColumnInfo         `json:"items"`      // 栏目列表
	Pagination GetColumnsPagination `json:"pagination"` // 分页信息
}

// GetColumnsPagination 分页信息
type GetColumnsPagination struct {
	Page     int   `json:"page"`     // 当前页码
	PageSize int   `json:"pageSize"` // 每页大小
	HasNext  bool  `json:"hasNext"`  // 是否有下一页
	Total    int64 `json:"total"`    // 总记录数
}

// GetSitesResponse GetSites API 响应结构体
type GetSitesResponse struct {
	Found      bool                 `json:"found"`      // 是否找到数据
	Items      []SiteInfo           `json:"items"`      // 站点列表
	Pagination GetColumnsPagination `json:"pagination"` // 分页信息（复用栏目分页结构）
}
