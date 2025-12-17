package models

const TableNameArticleAttachment = "article_attachment"

// ArticleAttachment 文章附件表
type ArticleAttachment struct {
	Id        int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	ArticleId int64  `json:"articleId" gorm:"column:articleId;index"`
	Name      string `json:"attachmentName,omitempty" gorm:"column:name;type:varchar(255)"`
	Path      string `json:"attachmentPath,omitempty" gorm:"column:path;type:varchar(1024)"`
}

func (*ArticleAttachment) TableName() string {
	return TableNameArticleAttachment
}
