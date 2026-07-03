package model

// XhsComment 对应爬虫 MySQL 的 xhs_note_comment 表（只读，不 migration）。
type XhsComment struct {
	CommentID       string `gorm:"column:comment_id"`
	NoteID          string `gorm:"column:note_id"`
	UserID          string `gorm:"column:user_id"`
	Nickname        string `gorm:"column:nickname"`
	Content         string `gorm:"column:content"`
	LikeCount       string `gorm:"column:like_count"`
	CreateTime      int64  `gorm:"column:create_time"`
	Pictures        string `gorm:"column:pictures"`
	SubCommentCount int    `gorm:"column:sub_comment_count"`
}

func (XhsComment) TableName() string { return "xhs_note_comment" }
