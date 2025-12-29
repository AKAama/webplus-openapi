package models

const TableNameTSite = "T_SITE"

type TSite struct {
	Id         int    `json:"id" gorm:"column:ID;primary_key"`     // 站点ID
	Name       string `json:"name" gorm:"column:NAME"`             // 站点名称
	DomainName string `json:"domainName" gorm:"column:DOMAINNAME"` // 域名
	FilePath   string `json:"filePath" gorm:"column:filepath"`     //文件路径
	Logo       string `json:"logo" gorm:"column:logo"`             //logo名
	ShortName  string `json:"shortName" gorm:"column:ShortName"`   //简称
}

func (*TSite) TableName() string {
	return TableNameTSite
}
