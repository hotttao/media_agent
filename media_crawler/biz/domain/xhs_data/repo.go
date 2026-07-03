package xhs_data

import "context"

// NoteFilters 笔记查询过滤（统一语义，repo 内部转各平台真实字段）。
type NoteFilters struct {
	Keywords      []string // 命中 source_keyword 或 title LIKE
	IDs           []string // note_id
	CreatorIDs    []string
	TimeStart     int64
	TimeEnd       int64
	SourceKeyword string
}

// NoteSort 排序（field 用语义名：time|liked_count|comment_count）。
type NoteSort struct {
	Field string // time|liked_count|comment_count
	Order string // desc(默认)/asc
}

// XhsRepo xhs_data domain 的仓库接口（由 dal/repo 实现，跨库只读 MySQL）。
// 排序 liked_count/comment_count 须 CAST AS UNSIGNED（varchar）；desc/type 字段反引号。
type XhsRepo interface {
	QueryNotes(ctx context.Context, filters NoteFilters, sort NoteSort, limit, offset int) ([]*XhsNote, int, error)
	GetNote(ctx context.Context, noteID string) (*XhsNote, error)
	QueryComments(ctx context.Context, noteID string, limit, offset int) ([]*XhsComment, int, error)
}
