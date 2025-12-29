package models

const TableNameTPubSite = "T_PUBLISHSITE"

type TPublishSite struct {
	Id              int `json:"id" gorm:"column:id;primary_key"`
	PublishServerId int `json:"publishServerId" gorm:"column:publishServerId"`
	SiteId          int `json:"siteId" gorm:"column:siteId"`
	ParentId        int `json:"parentId" gorm:"column:parentId"`
	Deleted         int `json:"deleted" gorm:"column:deleted"`
	EnableRedirect  int `json:"enableRedirect" gorm:"column:enableRedirect"`
}

func (*TPublishSite) TableName() string {
	return TableNameTPubSite
}
