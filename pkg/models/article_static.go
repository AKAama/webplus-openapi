package models

import "time"

const TableNameArticleStatic = "article_static"

type ArticleStatic struct {
	ArticleId      string       `json:"articleId" gorm:"column:articleId;primary_key" ` // 文章id
	CreateSiteId   string       `json:"createSiteId" gorm:"column:createSiteId" `
	FolderId       string       `json:"folderId" gorm:"column:folderId"  `           //文件夹id
	Title          string       `json:"title" gorm:"column:title" `                  // 文章标题
	ShortTitle     string       `json:"shortTitle" gorm:"column:shortTitle"`         // 文章短标题
	AuxiliaryTitle string       `json:"auxiliaryTitle" gorm:"column:auxiliaryTitle"` // 文章副标题
	CreatorName    string       `json:"creatorName" gorm:"column:creatorName"`       // 作者
	Summary        string       `json:"summary" gorm:"column:summary"`               // 文章简介
	PublishTime    *time.Time   `json:"publishTime" gorm:"column:publishTime"`       // 发布时间
	LastModifyTime *time.Time   `json:"lastModifyTime" gorm:"column:lastModifyTime"` // 最后修改时间
	PublisherName  string       `json:"publisherName" gorm:"column:publisherName"`   // 发布人名称
	PublishOrgName string       `json:"publishOrgName" gorm:"column:publishOrgName"` // 发布单位名称
	FirstImgPath   string       `json:"firstImgPath" gorm:"column:firstImgPath"`     // 封面图地址
	ImageDir       string       `json:"imageDir" gorm:"column:imageDir"`             // 图片目录
	FilePath       string       `json:"filePath" gorm:"column:filePath"`             // 附件散射目录
	CreateTime     string       `json:"createTime"  gorm:"column:createTime"`        //	文章创建时间
	Content        string       `json:"content" gorm:"column:content"`               //	文章内容
	VisitUrl       string       `json:"visitUrl" gorm:"column:visitUrl"`
	Attachment     []Attachment `json:"attachment" gorm:"-"`
	ArticleFields
}

func (*ArticleStatic) TableName() string {
	return TableNameArticleStatic
}
