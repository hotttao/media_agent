package media_workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	pb "media_agent/hertz_gen/model/media_workflow/job"
)

// CLI output is a safe execution summary, not the raw API payload containing
// signed candidate URLs, resolved inputs or rendered prompts.
type stepSummary struct {
	ID               string `json:"id"`
	Key              string `json:"step_key"`
	Status           string `json:"status"`
	CandidateCount   int    `json:"candidate_count"`
	RetryCount       int32  `json:"retry_count"`
	SelectedIndex    int32  `json:"selected_index"`
	OutputAssetID    string `json:"output_asset_id,omitempty"`
	LastReviewID     string `json:"last_review_id,omitempty"`
	LastReviewStatus string `json:"last_review_status,omitempty"`
}

func summarizeStep(s *pb.StepRun) stepSummary {
	return stepSummary{ID: s.GetId(), Key: s.GetStepKey(), Status: s.GetStatus().String(), CandidateCount: len(s.GetCandidates()), RetryCount: s.GetRetryCount(), SelectedIndex: s.GetSelectedIndex(), OutputAssetID: s.GetOutputAssetId(), LastReviewID: s.GetLastReviewId(), LastReviewStatus: s.GetLastReviewStatus()}
}

func writeWorkflowSummary(c *cobra.Command, value any) error {
	return json.NewEncoder(c.OutOrStdout()).Encode(value)
}

func newJobCommand() *cobra.Command {
	group := &cobra.Command{Use: "job", Short: "查询工作流执行状态"}
	var id string
	get := &cobra.Command{Use: "get", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if JobCli == nil {
			return errors.New("client.media_workflow is not configured")
		}
		result, raw, err := JobCli.GetJob(c.Context(), &pb.GetJobReq{Id: id})
		if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil || result.GetJob() == nil {
			return errors.New("workflow job query failed")
		}
		steps := make([]stepSummary, 0, len(result.GetStepRuns()))
		for _, step := range result.GetStepRuns() {
			steps = append(steps, summarizeStep(step))
		}
		return writeWorkflowSummary(c, struct {
			ID     string        `json:"id"`
			Status string        `json:"status"`
			Steps  []stepSummary `json:"steps"`
		}{result.GetJob().GetId(), result.GetJob().GetStatus().String(), steps})
	}}
	get.Flags().StringVar(&id, "id", "", "Job ID")
	_ = get.MarkFlagRequired("id")
	group.AddCommand(get)
	return group
}

func newStepRunCommand() *cobra.Command {
	group := &cobra.Command{Use: "step-run", Short: "查询节点、确认候选或请求重新生成"}
	var getID string
	get := &cobra.Command{Use: "get", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if JobCli == nil {
			return errors.New("client.media_workflow is not configured")
		}
		result, raw, err := JobCli.GetStepRun(c.Context(), &pb.GetStepRunReq{StepRunId: getID})
		if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil {
			return errors.New("workflow step query failed")
		}
		return writeWorkflowSummary(c, summarizeStep(result))
	}}
	get.Flags().StringVar(&getID, "id", "", "StepRun ID")
	_ = get.MarkFlagRequired("id")
	group.AddCommand(get)
	group.AddCommand(newStepRetryCommand())
	for _, decision := range []string{"confirm", "regenerate"} {
		var id, key, feedback string
		var selected int32
		command := &cobra.Command{Use: decision, Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
			if JobCli == nil {
				return errors.New("client.media_workflow is not configured")
			}
			if strings.TrimSpace(id) == "" || strings.TrimSpace(key) == "" || len(strings.TrimSpace(key)) > 160 || len(feedback) > 8192 || selected < 0 {
				return errors.New("invalid review parameters")
			}
			var result *pb.StepRun
			var status int
			var err error
			if decision == "confirm" {
				out, raw, callErr := JobCli.ConfirmStepRun(c.Context(), &pb.ConfirmStepRunReq{StepRunId: id, SelectedIndex: selected, IdempotencyKey: key})
				result, err = out, callErr
				if raw != nil {
					status = raw.StatusCode()
				}
			} else {
				out, raw, callErr := JobCli.RegenerateStepRun(c.Context(), &pb.RegenerateStepRunReq{StepRunId: id, Feedback: feedback, IdempotencyKey: key})
				result, err = out, callErr
				if raw != nil {
					status = raw.StatusCode()
				}
			}
			if err != nil || status != 202 || result == nil {
				return fmt.Errorf("workflow review request failed (status=%d)", status)
			}
			if result.GetLastReviewId() == "" || result.GetLastReviewStatus() == "" {
				return errors.New("workflow review receipt missing")
			}
			return writeWorkflowSummary(c, summarizeStep(result))
		}}
		command.Flags().StringVar(&id, "id", "", "StepRun ID")
		command.Flags().StringVar(&key, "idempotency-key", "", "同一决定的稳定键；网络重试复用")
		_ = command.MarkFlagRequired("id")
		_ = command.MarkFlagRequired("idempotency-key")
		if decision == "confirm" {
			command.Flags().Int32Var(&selected, "selected-index", 0, "候选数组位置（从 0 开始）")
			_ = command.MarkFlagRequired("selected-index")
		} else {
			command.Flags().StringVar(&feedback, "feedback", "", "下一轮重新生成反馈")
		}
		group.AddCommand(command)
	}
	return group
}

func newStepRetryCommand() *cobra.Command {
	var id, reason string
	command := &cobra.Command{Use: "retry", Short: "重试失败节点；网络失败时先查询状态，不自动重复请求", Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if JobCli == nil {
				return errors.New("client.media_workflow is not configured")
			}
			if strings.TrimSpace(id) == "" {
				return errors.New("StepRun ID is required")
			}
			result, raw, err := JobCli.RetryStepRun(c.Context(), &pb.RetryStepRunReq{StepRunId: id, Reason: reason})
			status := 0
			if raw != nil {
				status = raw.StatusCode()
			}
			if err != nil || status != 200 || result == nil || result.GetId() != id {
				return fmt.Errorf("workflow step retry not confirmed (status=%d); query execution before trying again", status)
			}
			return writeWorkflowSummary(c, summarizeStep(result))
		}}
	command.Flags().StringVar(&id, "id", "", "failed StepRun ID")
	command.Flags().StringVar(&reason, "reason", "", "重试原因")
	_ = command.MarkFlagRequired("id")
	return command
}

func init() { Cmd.AddCommand(newJobCommand(), newStepRunCommand()) }
