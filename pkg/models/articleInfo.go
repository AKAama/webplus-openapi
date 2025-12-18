package models

import (
	"reflect"
	"strings"
	"time"
)

type ArticleInfo struct {
	SiteId         string       `json:"siteId" gorm:"column:siteId" `                // 站点id (创建站点)
	SiteName       string       `json:"siteName" gorm:"column:siteName"`             // 站点名称 (创建站点)
	ArticleId      string       `json:"articleId" gorm:"column:articleId"  `         // 文章id
	FolderId       string       `json:"folderId" gorm:"column:folderId"`             //文件夹id
	ColumnId       []string     `json:"columnId" gorm:"column:columnId" `            // 栏目id (所有相关栏目)
	ColumnName     []string     `json:"columnName" gorm:"column:columnName"`         // 栏目名称 (所有相关栏目)
	Title          string       `json:"title" gorm:"column:title" `                  // 文章标题
	ShortTitle     string       `json:"shortTitle" gorm:"column:shortTitle"`         // 文章短标题
	AuxiliaryTitle string       `json:"auxiliaryTitle" gorm:"column:auxiliaryTitle"` // 文章副标题
	CreatorName    string       `json:"creatorName" gorm:"column:creatorName"`       // 作者
	Summary        string       `json:"summary" gorm:"column:summary"`               // 文章简介
	PublishTime    *time.Time   `json:"publishTime" gorm:"column:publishTime" `      // 发布时间
	LastModifyTime *time.Time   `json:"lastModifyTime" gorm:"column:lastModifyTime"` // 最后修改时间
	PublisherName  string       `json:"publisherName" gorm:"column:publisherName"`   // 发布人名称
	PublishOrgName string       `json:"publishOrgName" gorm:"column:publishOrgName"` // 发布单位名称
	VisitUrl       string       `json:"visitUrl"`                                    // 访问地址
	FirstImgPath   string       `json:"firstImgPath" gorm:"column:firstImgPath"`     // 封面图地址
	ImageDir       string       `json:"imageDir" gorm:"column:imageDir"`             // 图片目录
	FilePath       string       `json:"filePath" gorm:"column:filePath"`             // 附件散射目录
	CreateTime     string       `json:"createTime"  gorm:"column:createTime"`        // 文章创建时间
	Content        string       `json:"content" gorm:"column:content"`               // 文章内容
	Attachment     []Attachment `json:"attachment" gorm:"-"`
	ArticleFields
}
type Attachment struct {
	Name string `json:"attachmentName,omitempty"  gorm:"column:name"`
	Path string `json:"attachmentPath,omitempty"  gorm:"column:path"`
}

type ArticleFields struct {
	Field1  string `json:"field1,omitempty" gorm:"column:field1"`
	Field2  string `json:"field2,omitempty" gorm:"column:field2"`
	Field3  string `json:"field3,omitempty" gorm:"column:field3"`
	Field4  string `json:"field4,omitempty" gorm:"column:field4"`
	Field5  string `json:"field5,omitempty" gorm:"column:field5"`
	Field6  string `json:"field6,omitempty" gorm:"column:field6"`
	Field7  string `json:"field7,omitempty" gorm:"column:field7"`
	Field8  string `json:"field8,omitempty" gorm:"column:field8"`
	Field9  string `json:"field9,omitempty" gorm:"column:field9"`
	Field10 string `json:"field10,omitempty" gorm:"column:field10"`
	Field11 string `json:"field11,omitempty" gorm:"column:field11"`
	Field12 string `json:"field12,omitempty" gorm:"column:field12"`
	Field13 string `json:"field13,omitempty" gorm:"column:field13"`
	Field14 string `json:"field14,omitempty" gorm:"column:field14"`
	Field15 string `json:"field15,omitempty" gorm:"column:field15"`
	Field16 string `json:"field16,omitempty" gorm:"column:field16"`
	Field17 string `json:"field17,omitempty" gorm:"column:field17"`
	Field18 string `json:"field18,omitempty" gorm:"column:field18"`
	Field19 string `json:"field19,omitempty" gorm:"column:field19"`
	Field20 string `json:"field20,omitempty" gorm:"column:field20"`
	Field21 string `json:"field21,omitempty" gorm:"column:field21"`
	Field22 string `json:"field22,omitempty" gorm:"column:field22"`
	Field23 string `json:"field23,omitempty" gorm:"column:field23"`
	Field24 string `json:"field24,omitempty" gorm:"column:field24"`
	Field25 string `json:"field25,omitempty" gorm:"column:field25"`
	Field26 string `json:"field26,omitempty" gorm:"column:field26"`
	Field27 string `json:"field27,omitempty" gorm:"column:field27"`
	Field28 string `json:"field28,omitempty" gorm:"column:field28"`
	Field29 string `json:"field29,omitempty" gorm:"column:field29"`
	Field30 string `json:"field30,omitempty" gorm:"column:field30"`
	Field31 string `json:"field31,omitempty" gorm:"column:field31"`
	Field32 string `json:"field32,omitempty" gorm:"column:field32"`
	Field33 string `json:"field33,omitempty" gorm:"column:field33"`
	Field34 string `json:"field34,omitempty" gorm:"column:field34"`
	Field35 string `json:"field35,omitempty" gorm:"column:field35"`
	Field36 string `json:"field36,omitempty" gorm:"column:field36"`
	Field37 string `json:"field37,omitempty" gorm:"column:field37"`
	Field38 string `json:"field38,omitempty" gorm:"column:field38"`
	Field39 string `json:"field39,omitempty" gorm:"column:field39"`
	Field40 string `json:"field40,omitempty" gorm:"column:field40"`
	Field41 string `json:"field41,omitempty" gorm:"column:field41"`
	Field42 string `json:"field42,omitempty" gorm:"column:field42"`
	Field43 string `json:"field43,omitempty" gorm:"column:field43"`
	Field44 string `json:"field44,omitempty" gorm:"column:field44"`
	Field45 string `json:"field45,omitempty" gorm:"column:field45"`
	Field46 string `json:"field46,omitempty" gorm:"column:field46"`
	Field47 string `json:"field47,omitempty" gorm:"column:field47"`
	Field48 string `json:"field48,omitempty" gorm:"column:field48"`
	Field49 string `json:"field49,omitempty" gorm:"column:field49"`
	Field50 string `json:"field50,omitempty" gorm:"column:field50"`
}

// Column 栏目信息
type Column struct {
	ArticleId  int64  `json:"articleId" gorm:"column:articleId"`
	ColumnId   int    `json:"columnId" gorm:"column:columnId"`
	ColumnName string `json:"columnName" gorm:"column:columnName"`
	SiteId     string `json:"siteId" gorm:"column:siteId"`
	SiteName   string `json:"siteName" gorm:"column:siteName"`
	Url        string `json:"url" gorm:"column:url"`
}

// ToMap 以 fieldN 键返回所有扩展字段
func (f ArticleFields) ToMap() map[string]string {
	result := make(map[string]string, 50)
	val := reflect.ValueOf(f)
	typ := val.Type()
	for i := 0; i < typ.NumField(); i++ {
		name := strings.ToLower(typ.Field(i).Name)
		result[name] = val.Field(i).Interface().(string)
	}
	return result
}
