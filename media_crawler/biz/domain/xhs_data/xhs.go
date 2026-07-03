package xhs_data

// XhsNote 小红书笔记实体（xhs_data domain，对齐 xhs_note 表字段，不带 ORM tag）。
// 业务字段：note_id 业务唯一键，time 发布时间戳，liked_count 等原样 varchar（"1.2k"）。
type XhsNote struct {
	NoteID         string
	UserID         string
	Nickname       string
	Title          string
	Desc           string
	Type           string
	LikedCount     string
	CommentCount   string
	CollectedCount string
	ShareCount     string
	Time           int64
	LastUpdateTime int64
	SourceKeyword  string
	NoteURL        string
	ImageList      string
	TagList        string
}

// XhsComment 小红书评论实体（对齐 xhs_note_comment 表字段）。
type XhsComment struct {
	CommentID       string
	NoteID          string
	UserID          string
	Nickname        string
	Content         string
	LikeCount       string
	CreateTime      int64
	Pictures        string
	SubCommentCount int
}
