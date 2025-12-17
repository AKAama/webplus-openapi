package models

const TableNameArticleDynamic = "article_dynamic"

type ArticleDynamic struct {
	Id         int    `json:"id" gorm:"primary_key;AUTO_INCREMENT"`
	ArticleId  string `json:"articleId" gorm:"column:articleId" `
	ColumnId   string `json:"columnId" gorm:"column:columnId" `
	ColumnName string `json:"columnName" gorm:"column:columnName" `
	SiteId     string `json:"siteId" gorm:"column:siteId" `
	SiteName   string `json:"siteName" gorm:"column:siteName" `
	Url        string `json:"url" gorm:"column:url" ` //展示对应栏目自己的url，而不是创建站点的
}

func (*ArticleDynamic) TableName() string {
	return TableNameArticleDynamic
}
