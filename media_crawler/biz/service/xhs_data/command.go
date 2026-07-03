package xhs_data

// QueryXhsNotesCommand 查询小红书笔记用例输入。
type QueryXhsNotesCommand struct {
	Keywords      []string
	IDs           []string
	CreatorIDs    []string
	TimeStart     int64
	TimeEnd       int64
	SourceKeyword string
	SortField     string // time|liked_count|comment_count
	SortOrder     string // desc/asc
	Limit         int
	Offset        int
}

// QueryXhsCommentsCommand 查询小红书评论用例输入。
type QueryXhsCommentsCommand struct {
	NoteID string
	Limit  int
	Offset int
}
