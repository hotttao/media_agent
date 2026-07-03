package model

// XhsNote 对应爬虫 MySQL 的 xhs_note 表（只读，schema 由爬虫 schema/tables.sql 定义，不 migration）。
// 字段对齐 tables.sql：主键自增 id（不暴露），note_id 是业务唯一键。
// liked_count/comment_count 等是 varchar(16)（存 "1.2k"），排序须 CAST AS UNSIGNED。
// desc/type 是 MySQL 保留字，GORM 生成 SQL 时自动反引号。
type XhsNote struct {
	NoteID         string `gorm:"column:note_id"`
	UserID         string `gorm:"column:user_id"`
	Nickname       string `gorm:"column:nickname"`
	Title          string `gorm:"column:title"`
	Desc           string `gorm:"column:desc"`
	Type           string `gorm:"column:type"`
	LikedCount     string `gorm:"column:liked_count"`
	CommentCount   string `gorm:"column:comment_count"`
	CollectedCount string `gorm:"column:collected_count"`
	ShareCount     string `gorm:"column:share_count"`
	Time           int64  `gorm:"column:time"`
	LastUpdateTime int64  `gorm:"column:last_update_time"`
	SourceKeyword  string `gorm:"column:source_keyword"`
	NoteURL        string `gorm:"column:note_url"`
	ImageList      string `gorm:"column:image_list"`
	TagList        string `gorm:"column:tag_list"`
}

func (XhsNote) TableName() string { return "xhs_note" }
