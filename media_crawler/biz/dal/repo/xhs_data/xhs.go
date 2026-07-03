package xhs_data

import (
	"context"
	"strings"

	"gorm.io/gorm"

	"media_agent/media_crawler/biz/dal"
	"media_agent/media_crawler/biz/dal/model"
	domain "media_agent/media_crawler/biz/domain/xhs_data"
)

// XhsRepo 实现 domain.XhsRepo（MySQL 只读，跨库查爬虫内容数据）。
// 不走 GORM Gen query_read 的 Order——因 liked_count/comment_count 是 varchar，
// 排序须 CAST AS UNSIGNED，gen 不直接支持，故用 gorm raw Where + Order。
func (r *XhsRepo) QueryNotes(ctx context.Context, filters domain.NoteFilters, sort domain.NoteSort, limit, offset int) ([]*domain.XhsNote, int, error) {
	db := dal.FromContext(ctx, r.db).Model(&model.XhsNote{})
	db = applyNoteFilters(db, filters)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []*model.XhsNote
	if err := db.Order(sortExpr(sort)).Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	notes := make([]*domain.XhsNote, len(rows))
	for i, m := range rows {
		notes[i] = noteToDomain(m)
	}
	return notes, int(total), nil
}

func (r *XhsRepo) GetNote(ctx context.Context, noteID string) (*domain.XhsNote, error) {
	var m model.XhsNote
	if err := dal.FromContext(ctx, r.db).Where("note_id = ?", noteID).First(&m).Error; err != nil {
		return nil, err
	}
	return noteToDomain(&m), nil
}

func (r *XhsRepo) QueryComments(ctx context.Context, noteID string, limit, offset int) ([]*domain.XhsComment, int, error) {
	db := dal.FromContext(ctx, r.db).Model(&model.XhsComment{}).Where("note_id = ?", noteID)
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []*model.XhsComment
	if err := db.Order("create_time DESC").Limit(limit).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	comments := make([]*domain.XhsComment, len(rows))
	for i, m := range rows {
		comments[i] = commentToDomain(m)
	}
	return comments, int(total), nil
}

type XhsRepo struct{ db *gorm.DB }

func NewXhsRepo(db *gorm.DB) *XhsRepo { return &XhsRepo{db: db} }

func applyNoteFilters(db *gorm.DB, f domain.NoteFilters) *gorm.DB {
	if len(f.IDs) > 0 {
		db = db.Where("note_id IN ?", f.IDs)
	}
	if f.SourceKeyword != "" {
		db = db.Where("source_keyword = ?", f.SourceKeyword)
	}
	if f.TimeStart > 0 {
		db = db.Where("time >= ?", f.TimeStart)
	}
	if f.TimeEnd > 0 {
		db = db.Where("time <= ?", f.TimeEnd)
	}
	if len(f.Keywords) > 0 {
		ors := make([]string, 0, len(f.Keywords))
		args := make([]any, 0, len(f.Keywords)*2)
		for _, kw := range f.Keywords {
			ors = append(ors, "source_keyword = ? OR title LIKE ?")
			args = append(args, kw, "%"+kw+"%")
		}
		db = db.Where(strings.Join(ors, " OR "), args...)
	}
	return db
}

// sortExpr liked_count/comment_count 是 varchar，排序须 CAST；time 反引号避免保留字。
func sortExpr(sort domain.NoteSort) string {
	field := "`time`"
	switch sort.Field {
	case "liked_count":
		field = "CAST(liked_count AS UNSIGNED)"
	case "comment_count":
		field = "CAST(comment_count AS UNSIGNED)"
	case "time":
		field = "`time`"
	}
	order := "DESC"
	if sort.Order == "asc" {
		order = "ASC"
	}
	return field + " " + order
}

func noteToDomain(m *model.XhsNote) *domain.XhsNote {
	return &domain.XhsNote{
		NoteID:         m.NoteID,
		UserID:         m.UserID,
		Nickname:       m.Nickname,
		Title:          m.Title,
		Desc:           m.Desc,
		Type:           m.Type,
		LikedCount:     m.LikedCount,
		CommentCount:   m.CommentCount,
		CollectedCount: m.CollectedCount,
		ShareCount:     m.ShareCount,
		Time:           m.Time,
		LastUpdateTime: m.LastUpdateTime,
		SourceKeyword:  m.SourceKeyword,
		NoteURL:        m.NoteURL,
		ImageList:      m.ImageList,
		TagList:        m.TagList,
	}
}

func commentToDomain(m *model.XhsComment) *domain.XhsComment {
	return &domain.XhsComment{
		CommentID:       m.CommentID,
		NoteID:          m.NoteID,
		UserID:          m.UserID,
		Nickname:        m.Nickname,
		Content:         m.Content,
		LikeCount:       m.LikeCount,
		CreateTime:      m.CreateTime,
		Pictures:        m.Pictures,
		SubCommentCount: m.SubCommentCount,
	}
}

var _ domain.XhsRepo = (*XhsRepo)(nil)
