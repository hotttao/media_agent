package agent_session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"media_agent/media_agent_gateway/biz/dal"
	"media_agent/media_agent_gateway/biz/dal/model"
	domain "media_agent/media_agent_gateway/biz/domain/agent_session"
)

type Repository struct{ db *gorm.DB }

func New(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) FindLatestActive(ctx context.Context, userID string) (*domain.Session, error) {
	var row model.AgentSession
	err := dal.FromContext(ctx, r.db).Where("user_id = ? AND status = ?", userID, "active").Order("updated_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return toSession(row), nil
}

func (r *Repository) CreateSession(ctx context.Context, s *domain.Session) error {
	return dal.FromContext(ctx, r.db).Create(&model.AgentSession{ID: s.ID, UserID: s.UserID, WorkspaceID: s.WorkspaceID, AgentID: s.AgentID, AgentType: s.AgentType, Title: s.Title, ContextVersion: s.ContextVersion, Status: s.Status, ChatStatus: s.ChatStatus, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}).Error
}

func (r *Repository) ListOwned(ctx context.Context, userID, workspaceID string) ([]domain.Session, error) {
	var rows []model.AgentSession
	db := dal.FromContext(ctx, r.db).Where("user_id = ? AND status <> ?", userID, "deleted")
	if workspaceID != "" {
		db = db.Where("workspace_id = ?", workspaceID)
	}
	if err := db.Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		result = append(result, *toSession(row))
	}
	return result, nil
}

func (r *Repository) GetOwned(ctx context.Context, sessionID, userID string) (*domain.Session, error) {
	var row model.AgentSession
	err := dal.FromContext(ctx, r.db).Where("id = ?", sessionID).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.UserID != userID {
		return nil, domain.ErrForbidden
	}
	return toSession(row), nil
}

func (r *Repository) GetRunningTurn(ctx context.Context, sessionID string) (*domain.Turn, error) {
	var row model.Turn
	err := dal.FromContext(ctx, r.db).Where("session_id = ? AND status = ?", sessionID, "running").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &domain.Turn{ID: row.ID, SessionID: row.SessionID, IdempotencyKey: row.IdempotencyKey, Status: row.Status, StartedAt: row.StartedAt, FinishedAt: row.FinishedAt}, nil
}

func (r *Repository) ListMessages(ctx context.Context, sessionID string) ([]domain.Message, error) {
	var rows []model.Message
	if err := dal.FromContext(ctx, r.db).Where("session_id = ?", sessionID).Order("sequence ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Message, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Message{ID: row.ID, SessionID: row.SessionID, TurnID: row.TurnID, Role: row.Role, Content: row.Content, Sequence: row.Sequence, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (r *Repository) ListEventsAfter(ctx context.Context, sessionID string, sequence int64) ([]domain.Event, error) {
	var rows []model.UniversalEvent
	if err := dal.FromContext(ctx, r.db).Where("session_id = ? AND sequence > ?", sessionID, sequence).Order("sequence ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]domain.Event, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Event{ID: row.ID, SessionID: row.SessionID, TurnID: row.TurnID, Type: row.Type, Payload: row.Payload, Sequence: row.Sequence, CreatedAt: row.CreatedAt})
	}
	return out, nil
}

func (r *Repository) FindCompletedItemEvent(ctx context.Context, turnID, itemID, itemKind string) (*domain.Event, error) {
	var row model.UniversalEvent
	err := dal.FromContext(ctx, r.db).
		Where("turn_id = ? AND type = ?", turnID, "item.completed").
		Where("payload -> 'item' ->> 'item_id' = ?", itemID).
		Where("payload -> 'item' ->> 'kind' = ?", itemKind).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &domain.Event{ID: row.ID, SessionID: row.SessionID, TurnID: row.TurnID, Type: row.Type, Payload: row.Payload, Sequence: row.Sequence, CreatedAt: row.CreatedAt}, nil
}

func (r *Repository) NextMessageSequence(ctx context.Context, sessionID string) (int64, error) {
	if err := r.lockSession(ctx, sessionID); err != nil {
		return 0, err
	}
	var seq int64
	err := dal.FromContext(ctx, r.db).Raw("SELECT COALESCE(MAX(sequence), 0) + 1 FROM media_agent_gateway.messages WHERE session_id = ?", sessionID).Scan(&seq).Error
	return seq, err
}

func (r *Repository) NextEventSequence(ctx context.Context, sessionID string) (int64, error) {
	if err := r.lockSession(ctx, sessionID); err != nil {
		return 0, err
	}
	var seq int64
	err := dal.FromContext(ctx, r.db).Raw("SELECT COALESCE(MAX(sequence), 0) + 1 FROM media_agent_gateway.universal_events WHERE session_id = ?", sessionID).Scan(&seq).Error
	return seq, err
}

func (r *Repository) lockSession(ctx context.Context, sessionID string) error {
	var lockedID string
	result := dal.FromContext(ctx, r.db).Raw("SELECT id FROM media_agent_gateway.agent_sessions WHERE id = ? FOR UPDATE", sessionID).Scan(&lockedID)
	if result.Error != nil {
		return result.Error
	}
	if lockedID == "" {
		return domain.ErrNotFound
	}
	return nil
}

func (r *Repository) CreateTurnAndMessage(ctx context.Context, turn *domain.Turn, msg *domain.Message) error {
	db := dal.FromContext(ctx, r.db)
	if err := db.Create(&model.Turn{ID: turn.ID, SessionID: turn.SessionID, IdempotencyKey: turn.IdempotencyKey, Status: turn.Status, StartedAt: turn.StartedAt}).Error; err != nil {
		return classify(err)
	}
	if err := db.Create(&model.Message{ID: msg.ID, SessionID: msg.SessionID, TurnID: msg.TurnID, Role: msg.Role, Content: msg.Content, Sequence: msg.Sequence, CreatedAt: msg.CreatedAt}).Error; err != nil {
		return err
	}
	return db.Model(&model.AgentSession{}).Where("id = ?", turn.SessionID).Updates(map[string]any{"chat_status": domain.ChatStatusAgent, "updated_at": time.Now().UTC()}).Error
}

func (r *Repository) AppendEvent(ctx context.Context, event *domain.Event, msg *domain.Message, runtimeSessionID string) error {
	db := dal.FromContext(ctx, r.db)
	if err := db.Create(&model.UniversalEvent{ID: event.ID, SessionID: event.SessionID, TurnID: event.TurnID, Type: event.Type, Payload: event.Payload, Sequence: event.Sequence, CreatedAt: event.CreatedAt}).Error; err != nil {
		return err
	}
	if msg != nil {
		if err := db.Create(&model.Message{ID: msg.ID, SessionID: msg.SessionID, TurnID: msg.TurnID, Role: msg.Role, Content: msg.Content, Sequence: msg.Sequence, CreatedAt: msg.CreatedAt}).Error; err != nil {
			return err
		}
	}
	updates := map[string]any{"updated_at": time.Now().UTC()}
	if runtimeSessionID != "" {
		updates["runtime_session_id"] = runtimeSessionID
	}
	return db.Model(&model.AgentSession{}).Where("id = ?", event.SessionID).Updates(updates).Error
}

func (r *Repository) FinishTurn(ctx context.Context, turnID, status string) error {
	now := time.Now().UTC()
	db := dal.FromContext(ctx, r.db)
	var turn model.Turn
	if err := db.Where("id = ?", turnID).First(&turn).Error; err != nil {
		return err
	}
	if err := db.Model(&model.Turn{}).Where("id = ?", turnID).Updates(map[string]any{"status": status, "finished_at": now}).Error; err != nil {
		return err
	}
	return db.Model(&model.AgentSession{}).Where("id = ?", turn.SessionID).Updates(map[string]any{"chat_status": domain.ChatStatusHuman, "updated_at": now}).Error
}

func classify(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		if pg.ConstraintName == "turns_one_running_per_session_idx" {
			return domain.ErrTurnInFlight
		}
		return domain.ErrAlreadyExists
	}
	return fmt.Errorf("agent session repository: %w", err)
}

func toSession(row model.AgentSession) *domain.Session {
	return &domain.Session{ID: row.ID, UserID: row.UserID, WorkspaceID: row.WorkspaceID, AgentID: row.AgentID, AgentType: row.AgentType, Title: row.Title, ContextVersion: row.ContextVersion, RuntimeSessionID: row.RuntimeSessionID, Status: row.Status, ChatStatus: row.ChatStatus, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}
}
