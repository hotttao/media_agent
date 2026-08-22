package agenthistory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrUnsafeCleanup = errors.New("legacy Agent history cleanup is not safe")

type Migrator struct{ Source, Target *sql.DB }

type Counts struct{ Turns, Messages, Events int64 }
type fingerprints struct{ Turns, Messages, Events string }

type session struct {
	ID, WorkspaceID, AgentID, Status, ChatStatus string
	ContextVersion                               int64
	RuntimeSessionID                             sql.NullString
	CreatedAt, UpdatedAt                         time.Time
}

func (m *Migrator) Import(ctx context.Context) error {
	if err := m.ensureLedger(ctx); err != nil {
		return err
	}
	sessions, err := m.legacySessions(ctx)
	if err != nil {
		return err
	}
	for _, value := range sessions {
		if err := m.importSession(ctx, value); err != nil {
			return fmt.Errorf("migrate session %s: %w", value.ID, err)
		}
	}
	return m.Verify(ctx)
}

func (m *Migrator) Verify(ctx context.Context) error {
	sessions, err := m.legacySessions(ctx)
	if err != nil {
		return err
	}
	for _, value := range sessions {
		source, err := counts(ctx, m.Source, "media_agent_gateway", value.ID)
		if err != nil {
			return err
		}
		target, err := counts(ctx, m.Target, "media_agents", value.ID)
		if err != nil {
			return err
		}
		if source != target {
			return fmt.Errorf("%w: session %s source=%+v target=%+v", ErrUnsafeCleanup, value.ID, source, target)
		}
		sourcePrints, err := historyFingerprints(ctx, m.Source, "media_agent_gateway", value.ID)
		if err != nil {
			return err
		}
		targetPrints, err := historyFingerprints(ctx, m.Target, "media_agents", value.ID)
		if err != nil {
			return err
		}
		if sourcePrints != targetPrints {
			return fmt.Errorf("%w: session %s content fingerprint differs", ErrUnsafeCleanup, value.ID)
		}
		var contextVersion int64
		if err := m.Target.QueryRowContext(ctx, `SELECT context_version FROM media_agents.runtime_sessions WHERE session_id=$1`, value.ID).Scan(&contextVersion); err != nil {
			return fmt.Errorf("%w: target session %s: %v", ErrUnsafeCleanup, value.ID, err)
		}
		if contextVersion != value.ContextVersion {
			return fmt.Errorf("%w: session %s context version %d != %d", ErrUnsafeCleanup, value.ID, contextVersion, value.ContextVersion)
		}
	}
	return nil
}

func (m *Migrator) Cleanup(ctx context.Context, confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("%w: explicit confirmation is required", ErrUnsafeCleanup)
	}
	if err := m.Verify(ctx); err != nil {
		return err
	}
	tx, err := m.Source.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DROP TABLE media_agent_gateway.universal_events, media_agent_gateway.messages, media_agent_gateway.turns`); err != nil {
		return err
	}
	return tx.Commit()
}

func (m *Migrator) ensureLedger(ctx context.Context) error {
	_, err := m.Source.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS media_agent_gateway.agent_history_migrations (
		session_id uuid PRIMARY KEY REFERENCES media_agent_gateway.agent_sessions(id) ON DELETE CASCADE,
		migrated_at timestamptz NOT NULL DEFAULT now(), turns bigint NOT NULL, messages bigint NOT NULL, events bigint NOT NULL
	)`)
	return err
}

func (m *Migrator) legacySessions(ctx context.Context) ([]session, error) {
	rows, err := m.Source.QueryContext(ctx, `SELECT s.id::text,s.workspace_id,s.agent_id,s.status,s.chat_status,s.context_version,s.runtime_session_id,s.created_at,s.updated_at
		FROM media_agent_gateway.agent_sessions s WHERE EXISTS (SELECT 1 FROM media_agent_gateway.turns t WHERE t.session_id=s.id) ORDER BY s.created_at,s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []session
	for rows.Next() {
		var value session
		if err := rows.Scan(&value.ID, &value.WorkspaceID, &value.AgentID, &value.Status, &value.ChatStatus, &value.ContextVersion, &value.RuntimeSessionID, &value.CreatedAt, &value.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (m *Migrator) importSession(ctx context.Context, value session) error {
	var running int
	if err := m.Source.QueryRowContext(ctx, `SELECT count(*) FROM media_agent_gateway.turns WHERE session_id=$1 AND status IN ('running','queued')`, value.ID).Scan(&running); err != nil {
		return err
	}
	if running != 0 {
		return fmt.Errorf("%w: %d non-terminal legacy Turns", ErrUnsafeCleanup, running)
	}
	tx, err := m.Target.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	contextRef := fmt.Sprintf("workspace:%s/agent:%s/context:%d", value.WorkspaceID, value.AgentID, value.ContextVersion)
	_, err = tx.ExecContext(ctx, `INSERT INTO media_agents.runtime_sessions(session_id,context_version,context_ref,status,chat_status,active_turn_id,next_turn_sequence,next_event_sequence,next_message_sequence,runtime_session_id,created_at,updated_at)
		VALUES($1,$2,$3,'active','human',NULL,1,1,1,$4,$5,$6) ON CONFLICT(session_id) DO NOTHING`, value.ID, value.ContextVersion, contextRef, value.RuntimeSessionID, value.CreatedAt, value.UpdatedAt)
	if err != nil {
		return err
	}
	var targetVersion int64
	if err := tx.QueryRowContext(ctx, `SELECT context_version FROM media_agents.runtime_sessions WHERE session_id=$1 FOR UPDATE`, value.ID).Scan(&targetVersion); err != nil {
		return err
	}
	if targetVersion != value.ContextVersion {
		return fmt.Errorf("context version conflict: target=%d source=%d", targetVersion, value.ContextVersion)
	}
	if err := m.copyTurns(ctx, tx, value.ID); err != nil {
		return err
	}
	if err := m.copyMessages(ctx, tx, value.ID); err != nil {
		return err
	}
	if err := m.copyEvents(ctx, tx, value.ID); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE media_agents.runtime_sessions SET
		next_turn_sequence=COALESCE((SELECT max(enqueue_sequence)+1 FROM media_agents.turns WHERE session_id=$1),1),
		next_message_sequence=COALESCE((SELECT max(sequence)+1 FROM media_agents.messages WHERE session_id=$1),1),
		next_event_sequence=COALESCE((SELECT max(sequence)+1 FROM media_agents.universal_events WHERE session_id=$1),1),updated_at=GREATEST(updated_at,$2)
		WHERE session_id=$1`, value.ID, value.UpdatedAt)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	c, err := counts(ctx, m.Source, "media_agent_gateway", value.ID)
	if err != nil {
		return err
	}
	_, err = m.Source.ExecContext(ctx, `INSERT INTO media_agent_gateway.agent_history_migrations(session_id,turns,messages,events) VALUES($1,$2,$3,$4)
		ON CONFLICT(session_id) DO UPDATE SET migrated_at=now(),turns=EXCLUDED.turns,messages=EXCLUDED.messages,events=EXCLUDED.events`, value.ID, c.Turns, c.Messages, c.Events)
	return err
}

func (m *Migrator) copyTurns(ctx context.Context, tx *sql.Tx, sessionID string) error {
	rows, err := m.Source.QueryContext(ctx, `SELECT id::text,idempotency_key,status,started_at,finished_at,row_number() OVER(ORDER BY started_at,id) FROM media_agent_gateway.turns WHERE session_id=$1 ORDER BY started_at,id`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, key, status string
		var started time.Time
		var finished sql.NullTime
		var sequence int64
		if err := rows.Scan(&id, &key, &status, &started, &finished, &sequence); err != nil {
			return err
		}
		input, _ := json.Marshal(map[string]any{"legacy_migration": true})
		var finishedValue any
		if finished.Valid {
			finishedValue = finished.Time
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO media_agents.turns(id,session_id,idempotency_key,enqueue_sequence,status,input,context_version,created_at,started_at,finished_at,error_class) SELECT $1,$2::varchar,$3,$4,$5,$6,s.context_version,$7,$7,$8,NULL FROM media_agents.runtime_sessions s WHERE s.session_id=$2::varchar ON CONFLICT(id) DO NOTHING`, id, sessionID, key, sequence, status, input, started, finishedValue)
		if err != nil {
			return err
		}
	}
	return rows.Err()
}

func (m *Migrator) copyMessages(ctx context.Context, tx *sql.Tx, sessionID string) error {
	rows, err := m.Source.QueryContext(ctx, `SELECT id::text,turn_id::text,role,content,sequence,created_at FROM media_agent_gateway.messages WHERE session_id=$1 ORDER BY sequence`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, turnID, role string
		var content []byte
		var sequence int64
		var created time.Time
		if err := rows.Scan(&id, &turnID, &role, &content, &sequence, &created); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO media_agents.messages(id,session_id,turn_id,role,content_ref,sequence,created_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO NOTHING`, id, sessionID, turnID, role, content, sequence, created); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (m *Migrator) copyEvents(ctx context.Context, tx *sql.Tx, sessionID string) error {
	rows, err := m.Source.QueryContext(ctx, `SELECT id::text,turn_id::text,type,payload,sequence,created_at FROM media_agent_gateway.universal_events WHERE session_id=$1 ORDER BY sequence`, sessionID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, turnID, eventType string
		var payload []byte
		var sequence int64
		var created time.Time
		if err := rows.Scan(&id, &turnID, &eventType, &payload, &sequence, &created); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO media_agents.universal_events(id,session_id,turn_id,type,payload,sequence,created_at) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(id) DO NOTHING`, id, sessionID, turnID, eventType, payload, sequence, created); err != nil {
			return err
		}
	}
	return rows.Err()
}

func counts(ctx context.Context, db *sql.DB, schema, sessionID string) (Counts, error) {
	if schema != "media_agent_gateway" && schema != "media_agents" {
		return Counts{}, errors.New("unsupported schema")
	}
	var value Counts
	query := fmt.Sprintf(`SELECT (SELECT count(*) FROM %s.turns WHERE session_id=$1),(SELECT count(*) FROM %s.messages WHERE session_id=$1),(SELECT count(*) FROM %s.universal_events WHERE session_id=$1)`, schema, schema, schema)
	err := db.QueryRowContext(ctx, query, sessionID).Scan(&value.Turns, &value.Messages, &value.Events)
	return value, err
}

func historyFingerprints(ctx context.Context, db *sql.DB, schema, sessionID string) (fingerprints, error) {
	if schema != "media_agent_gateway" && schema != "media_agents" {
		return fingerprints{}, errors.New("unsupported schema")
	}
	turnSequence, messageContent := "enqueue_sequence", "content_ref"
	if schema == "media_agent_gateway" {
		turnSequence, messageContent = "row_number() OVER(ORDER BY started_at,id)", "content"
	}
	turns := fmt.Sprintf(`SELECT COALESCE(md5(string_agg(id||'|'||idempotency_key||'|'||status||'|'||sequence,E'\n' ORDER BY sequence)),md5('')) FROM (SELECT id::text,idempotency_key,status,(%s)::text sequence FROM %s.turns WHERE session_id=$1) rows`, turnSequence, schema)
	messages := fmt.Sprintf(`SELECT COALESCE(md5(string_agg(id::text||'|'||turn_id::text||'|'||role||'|'||content_value::text||'|'||sequence::text,E'\n' ORDER BY sequence)),md5('')) FROM (SELECT id,turn_id,role,%s content_value,sequence FROM %s.messages WHERE session_id=$1) rows`, messageContent, schema)
	events := fmt.Sprintf(`SELECT COALESCE(md5(string_agg(id::text||'|'||COALESCE(turn_id::text,'')||'|'||type||'|'||payload::text||'|'||sequence::text,E'\n' ORDER BY sequence)),md5('')) FROM %s.universal_events WHERE session_id=$1`, schema)
	var value fingerprints
	if err := db.QueryRowContext(ctx, turns, sessionID).Scan(&value.Turns); err != nil {
		return value, err
	}
	if err := db.QueryRowContext(ctx, messages, sessionID).Scan(&value.Messages); err != nil {
		return value, err
	}
	if err := db.QueryRowContext(ctx, events, sessionID).Scan(&value.Events); err != nil {
		return value, err
	}
	return value, nil
}
