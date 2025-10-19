package models

import (
	"time"
)

type ArticleInfo struct {
	SiteId         string       `json:"-" gorm:"column:siteId" badgerhold:"index"`                // 站点id
	SiteName       string       `json:"siteName,omitempty" gorm:"column:siteName"`                // 站点名称
	ArticleId      string       `json:"articleId" gorm:"column:articleId"  badgerhold:"key"`      // 文章id
	ColumnId       string       `json:"columnId" gorm:"column:columnId" badgerhold:"index"`       // 栏目id
	ColumnName     string       `json:"columnName" gorm:"column:columnName"`                      // 栏目名称
	Title          string       `json:"title" gorm:"column:title" badgerhold:"index"`             // 文章标题
	ShortTitle     string       `json:"-" gorm:"column:shortTitle"`                               // 文章短标题
	AuxiliaryTitle string       `json:"-" gorm:"column:auxiliaryTitle"`                           // 文章副标题
	CreatorName    string       `json:"creatorName" gorm:"column:creatorName"`                    // 作者
	Summary        string       `json:"summary" gorm:"column:summary"`                            // 文章简介
	PublishTime    *time.Time   `json:"publishTime" gorm:"column:publishTime" badgerhold:"index"` // 发布时间
	LastModifyTime *time.Time   `json:"lastModifyTime" gorm:"column:lastModifyTime"`              // 最后修改时间
	PublisherName  string       `json:"-" gorm:"column:publisherName"`                            // 发布人名称
	PublishOrgName string       `json:"-" gorm:"column:publishOrgName"`                           // 发布单位名称
	VisitUrl       string       `json:"visitUrl"`                                                 // 访问地址
	FirstImgPath   string       `json:"-" gorm:"column:firstImgPath"`                             // 封面图地址
	ImageDir       string       `json:"-" gorm:"column:imageDir"`                                 // 图片目录
	FilePath       string       `json:"-" gorm:"column:filePath"`                                 // 附件散射目录
	CreateTime     string       `json:"-"  gorm:"column:createTime"`                              //	文章创建时间
	Content        string       `json:"content" gorm:"column:content"`                            //	文章内容
	Attachment     []Attachment `json:"attachment" gorm:"-"`
}
type Attachment struct {
	Name string `json:"attachmentName,omitempty"  gorm:"column:name"`
	Path string `json:"attachmentPath,omitempty"  gorm:"column:path"`
}
