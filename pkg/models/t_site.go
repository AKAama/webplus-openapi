package models

const TableNameTSite = "T_SITE"

type TSite struct {
	Id         int    `json:"id" gorm:"column:ID;primary_key"`     // 站点ID
	Name       string `json:"name" gorm:"column:NAME"`             // 站点名称
	DomainName string `json:"domainName" gorm:"column:DOMAINNAME"` // 域名
}

func (*TSite) TableName() string {
	return TableNameTSite
}
