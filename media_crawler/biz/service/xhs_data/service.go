package xhs_data

import (
	"context"

	domain "media_agent/media_crawler/biz/domain/xhs_data"
)

// Service xhs_data 用例编排（只读查询，无事务）。
type Service struct {
	repo domain.XhsRepo
}

func NewService(repo domain.XhsRepo) *Service { return &Service{repo: repo} }

// QueryNotes 查询小红书笔记列表。
func (s *Service) QueryNotes(ctx context.Context, cmd QueryXhsNotesCommand) ([]*domain.XhsNote, int, error) {
	filters := domain.NoteFilters{
		Keywords:      cmd.Keywords,
		IDs:           cmd.IDs,
		CreatorIDs:    cmd.CreatorIDs,
		TimeStart:     cmd.TimeStart,
		TimeEnd:       cmd.TimeEnd,
		SourceKeyword: cmd.SourceKeyword,
	}
	sort := domain.NoteSort{Field: cmd.SortField, Order: cmd.SortOrder}
	return s.repo.QueryNotes(ctx, filters, sort, cmd.Limit, cmd.Offset)
}

// GetNote 查询单条小红书笔记。
func (s *Service) GetNote(ctx context.Context, noteID string) (*domain.XhsNote, error) {
	return s.repo.GetNote(ctx, noteID)
}

// QueryComments 查询小红书评论。
func (s *Service) QueryComments(ctx context.Context, cmd QueryXhsCommentsCommand) ([]*domain.XhsComment, int, error) {
	return s.repo.QueryComments(ctx, cmd.NoteID, cmd.Limit, cmd.Offset)
}
