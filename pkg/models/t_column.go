package models

const TableNameTColumn = "T_COLUMN"

type TColumn struct {
	Id         int    `json:"id" gorm:"column:id;primary_key"` // 栏目ID
	Name       string `json:"name" gorm:"column:name"`         // 栏目名称
	SiteId     int    `json:"siteId" gorm:"column:siteId"`     // 站点ID
	ParentId   int    `json:"parentId" gorm:"column:parentId"` // 父栏目ID
	Link       string `json:"link" gorm:"column:link"`
	Path       string `json:"path" gorm:"column:path"`             // 栏目路径
	Sort       int    `json:"sort" gorm:"column:sort"`             // 栏目排序
	Navigation int    `json:"navigation" gorm:"column:navigation"` // 是否导航
	Readonly   int    `json:"readonly" gorm:"column:readonly"`     // 是否只读
}

func (*TColumn) TableName() string {
	return TableNameTColumn
}
