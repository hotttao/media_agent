package job

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"media_agent/media_workflow/biz/dal"
	"media_agent/media_workflow/biz/dal/model"
	"media_agent/media_workflow/biz/dal/query"
	domain "media_agent/media_workflow/biz/domain/job"
)

// —— JobRepo ——

type JobRepo struct{ db *gorm.DB }

func NewJobRepo(db *gorm.DB) *JobRepo { return &JobRepo{db: db} }

func (r *JobRepo) Save(ctx context.Context, j *domain.Job, stepRuns []*domain.StepRun) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	inputJSON, _ := json.Marshal(j.Input)
	jobRow := &model.Job{
		ID: j.ID, Scope: j.Scope, WorkflowID: j.WorkflowID, WorkflowVersion: j.WorkflowVersion,
		Input: datatypes.JSON(inputJSON), Status: string(j.Status),
		SubmittedBy: j.SubmittedBy, SubmittedAt: time.Now(),
		RetryHistory: datatypes.JSON("[]"),
	}
	if err := Q.Job.WithContext(ctx).Create(jobRow); err != nil {
		return err
	}
	for _, sr := range stepRuns {
		srRow := &model.StepRun{
			ID: sr.ID, JobID: j.ID, StepKey: sr.StepKey, StepOrder: sr.StepOrder,
			TemplateID: sr.TemplateID, TemplateVersion: sr.TemplateVersion,
			ApprovalPolicy: string(sr.ApprovalPolicy),
			Status:         string(sr.Status), ResolvedInputs: datatypes.JSON("{}"),
			Candidates: datatypes.JSON("[]"),
			Platform:   sr.Platform, GenWay: sr.GenWay, GenType: sr.GenType,
			AssetKind: sr.AssetKind, NeedsConfirm: sr.NeedsConfirm, CandidateCount: sr.CandidateCount,
		}
		// 注：domain StepRun 需要加反范式字段。先检查。
		if err := Q.StepRun.WithContext(ctx).Create(srRow); err != nil {
			return err
		}
	}
	return nil
}

func (r *JobRepo) Get(ctx context.Context, id string) (*domain.Job, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.Job.WithContext(ctx).Where(Q.Job.ID.Eq(id)).First()
	if err != nil {
		return nil, err
	}
	return jobToDomain(row), nil
}

func (r *JobRepo) GetWithStepRuns(ctx context.Context, id string) (*domain.Job, []*domain.StepRun, error) {
	j, err := r.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	Q := query.Use(dal.FromContext(ctx, r.db))
	rows, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.JobID.Eq(id)).Order(Q.StepRun.StepOrder).Find()
	if err != nil {
		return nil, nil, err
	}
	srs := make([]*domain.StepRun, len(rows))
	for i := range rows {
		srs[i] = stepRunToDomain(rows[i])
	}
	return j, srs, nil
}

func (r *JobRepo) List(ctx context.Context, q domain.ListQuery) ([]*domain.Job, int, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	tx := Q.Job.WithContext(ctx)
	if q.Scope != "" {
		tx = tx.Where(Q.Job.Scope.Eq(q.Scope))
	}
	if q.WorkflowID != "" {
		tx = tx.Where(Q.Job.WorkflowID.Eq(q.WorkflowID))
	}
	if q.Status != "" {
		tx = tx.Where(Q.Job.Status.Eq(q.Status))
	}
	total, err := tx.Count()
	if err != nil {
		return nil, 0, err
	}
	if q.Limit > 0 {
		tx = tx.Limit(q.Limit)
	}
	if q.Offset > 0 {
		tx = tx.Offset(q.Offset)
	}
	rows, err := tx.Order(Q.Job.SubmittedAt.Desc()).Find()
	if err != nil {
		return nil, 0, err
	}
	out := make([]*domain.Job, len(rows))
	for i := range rows {
		out[i] = jobToDomain(rows[i])
	}
	return out, int(total), nil
}

func (r *JobRepo) UpdateStatus(ctx context.Context, id string, status domain.JobStatus, failureReason string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	updates := map[string]any{"status": string(status), "failure_reason": failureReason}
	if status.IsTerminal() {
		now := time.Now()
		updates["finished_at"] = now
	}
	if status == domain.JobRunning {
		now := time.Now()
		updates["started_at"] = now
	}
	_, err := Q.Job.WithContext(ctx).Where(Q.Job.ID.Eq(id)).Updates(updates)
	return err
}

func (r *JobRepo) UpdateInput(ctx context.Context, id string, input domain.JobInput) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	inputJSON, _ := json.Marshal(input)
	_, err := Q.Job.WithContext(ctx).Where(Q.Job.ID.Eq(id)).Updates(map[string]any{"input": datatypes.JSON(inputJSON)})
	return err
}

func (r *JobRepo) AppendRetryHistory(ctx context.Context, id string, record domain.RetryRecord, retryCount int) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	j, err := Q.Job.WithContext(ctx).Where(Q.Job.ID.Eq(id)).First()
	if err != nil {
		return err
	}
	var history []domain.RetryRecord
	_ = json.Unmarshal(j.RetryHistory, &history)
	history = append(history, record)
	histJSON, _ := json.Marshal(history)
	_, err = Q.Job.WithContext(ctx).Where(Q.Job.ID.Eq(id)).Updates(map[string]any{
		"retry_history": datatypes.JSON(histJSON), "retry_count": retryCount,
		"status": string(domain.JobRunning), "failure_reason": "", "finished_at": nil,
	})
	return err
}

// —— StepRunRepo ——

type StepRunRepo struct{ db *gorm.DB }

func NewStepRunRepo(db *gorm.DB) *StepRunRepo { return &StepRunRepo{db: db} }

func (r *StepRunRepo) Get(ctx context.Context, id string) (*domain.StepRun, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	row, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.ID.Eq(id)).First()
	if err != nil {
		return nil, err
	}
	return stepRunToDomain(row), nil
}

func (r *StepRunRepo) ListByJob(ctx context.Context, jobID string) ([]*domain.StepRun, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	rows, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.JobID.Eq(jobID)).Order(Q.StepRun.StepOrder).Find()
	if err != nil {
		return nil, err
	}
	out := make([]*domain.StepRun, len(rows))
	for i := range rows {
		out[i] = stepRunToDomain(rows[i])
	}
	return out, nil
}

func (r *StepRunRepo) ListByJobAndStatus(ctx context.Context, jobID string, status domain.StepRunStatus) ([]*domain.StepRun, error) {
	Q := query.Use(dal.FromContext(ctx, r.db))
	rows, err := Q.StepRun.WithContext(ctx).
		Where(Q.StepRun.JobID.Eq(jobID), Q.StepRun.Status.Eq(string(status))).Find()
	if err != nil {
		return nil, err
	}
	out := make([]*domain.StepRun, len(rows))
	for i := range rows {
		out[i] = stepRunToDomain(rows[i])
	}
	return out, nil
}

// ClaimAtomic 原子领取一个 PENDING StepRun（FOR UPDATE SKIP LOCKED）。
func (r *StepRunRepo) ClaimAtomic(ctx context.Context, q domain.ClaimQuery) (*domain.StepRun, error) {
	db := dal.FromContext(ctx, r.db)
	var row model.StepRun
	// 原子 UPDATE...RETURNING：领一个 PENDING + platform 匹配的 step_run
	err := db.Raw(`
		UPDATE media_workflow.step_runs
		SET status = 'claimed', claimed_by = ?, claimed_at = now()
		WHERE id = (
			SELECT id FROM media_workflow.step_runs
			WHERE status = 'pending' AND platform = ? AND gen_way = ?
			ORDER BY step_order LIMIT 1 FOR UPDATE SKIP LOCKED
		)
		RETURNING *`, q.WorkerID, q.Platform, q.GenWay).Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return stepRunToDomain(&row), nil
}

func (r *StepRunRepo) UpdateStatus(ctx context.Context, id string, status domain.StepRunStatus, updates map[string]any) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	if updates == nil {
		updates = map[string]any{}
	}
	updates["status"] = string(status)
	if status == domain.SRSucceeded || status == domain.SRFailed {
		now := time.Now()
		updates["finished_at"] = now
	}
	if status == domain.SRClaimed {
		now := time.Now()
		updates["claimed_at"] = now
		updates["started_at"] = now
	}
	_, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.ID.Eq(id)).Updates(updates)
	return err
}

func (r *StepRunRepo) UpdateResolvedInputs(ctx context.Context, id string, inputs map[string]any, prompt string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	inputsJSON, _ := json.Marshal(inputs)
	_, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.ID.Eq(id)).Updates(map[string]any{
		"resolved_inputs": datatypes.JSON(inputsJSON), "rendered_prompt": prompt,
	})
	return err
}

func (r *StepRunRepo) UpdateCandidates(ctx context.Context, id string, candidates []domain.Candidate) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	candJSON, _ := json.Marshal(candidates)
	_, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.ID.Eq(id)).Updates(map[string]any{
		"candidates": datatypes.JSON(candJSON),
	})
	return err
}

func (r *StepRunRepo) UpdateOutput(ctx context.Context, id string, outputAssetID string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.ID.Eq(id)).Updates(map[string]any{
		"output_asset_id": outputAssetID,
	})
	return err
}

func (r *StepRunRepo) UpdateFailure(ctx context.Context, id string, reason string) error {
	Q := query.Use(dal.FromContext(ctx, r.db))
	_, err := Q.StepRun.WithContext(ctx).Where(Q.StepRun.ID.Eq(id)).Updates(map[string]any{
		"failure_reason": reason,
	})
	return err
}

// —— 转换 ——

func jobToDomain(row *model.Job) *domain.Job {
	j := &domain.Job{
		ID: row.ID, Scope: row.Scope, WorkflowID: row.WorkflowID, WorkflowVersion: row.WorkflowVersion,
		Status: domain.JobStatus(row.Status), RetryCount: row.RetryCount,
		SubmittedBy: row.SubmittedBy, SubmittedAt: row.SubmittedAt,
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt, FailureReason: row.FailureReason,
	}
	_ = json.Unmarshal(row.Input, &j.Input)
	_ = json.Unmarshal(row.RetryHistory, &j.RetryHistory)
	return j
}

func stepRunToDomain(row *model.StepRun) *domain.StepRun {
	sr := &domain.StepRun{
		ID: row.ID, JobID: row.JobID, StepKey: row.StepKey, StepOrder: row.StepOrder,
		TemplateID: row.TemplateID, TemplateVersion: row.TemplateVersion,
		Status: domain.StepRunStatus(row.Status), RetryCount: row.RetryCount,
		RenderedPrompt: row.RenderedPrompt, SelectedIndex: row.SelectedIndex,
		OutputAssetID: row.OutputAssetID, FailureReason: row.FailureReason,
		ClaimedBy: row.ClaimedBy, ClaimedAt: row.ClaimedAt,
		StartedAt: row.StartedAt, FinishedAt: row.FinishedAt,
		// 反范式
		Platform: row.Platform, GenWay: row.GenWay, GenType: row.GenType,
		AssetKind: row.AssetKind, NeedsConfirm: row.NeedsConfirm, CandidateCount: row.CandidateCount,
		ApprovalPolicy: domain.ApprovalPolicy(row.ApprovalPolicy),
	}
	_ = json.Unmarshal(row.ResolvedInputs, &sr.ResolvedInputs)
	_ = json.Unmarshal(row.Candidates, &sr.Candidates)
	if sr.ResolvedInputs == nil {
		sr.ResolvedInputs = map[string]any{}
	}
	return sr
}

var _ = fmt.Sprintf

// ReclaimStale 把 CLAIMED 超时的 step_run 回 PENDING（worker 崩溃保护）。
func (r *StepRunRepo) ReclaimStale(ctx context.Context, timeoutSeconds int) (int64, error) {
	db := dal.FromContext(ctx, r.db)
	res := db.Exec(`
		UPDATE media_workflow.step_runs
		SET status = 'pending', claimed_by = '', claimed_at = NULL, started_at = NULL
		WHERE status = 'claimed' AND claimed_at < now() - (? || ' seconds')::interval`,
		timeoutSeconds)
	return res.RowsAffected, res.Error
}
