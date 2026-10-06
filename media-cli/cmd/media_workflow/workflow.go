package media_workflow

import (
	"errors"

	"github.com/spf13/cobra"
	pb "media_agent/hertz_gen/model/media_workflow/workflow"
)

func newWorkflowCommand() *cobra.Command {
	group := &cobra.Command{Use: "workflow", Short: "发布工作流并查询发布状态"}
	for _, operation := range []string{"get", "publish"} {
		operation := operation
		var id string
		command := &cobra.Command{Use: operation, Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
			if WfCli == nil {
				return errors.New("client.media_workflow is not configured")
			}
			var result *pb.Workflow
			var err error
			status := 0
			if operation == "publish" {
				value, raw, callErr := WfCli.PublishWorkflow(c.Context(), &pb.GetWorkflowReq{Id: id})
				result, err = value, callErr
				if raw != nil {
					status = raw.StatusCode()
				}
			} else {
				value, raw, callErr := WfCli.GetWorkflow(c.Context(), &pb.GetWorkflowReq{Id: id})
				result, err = value, callErr
				if raw != nil {
					status = raw.StatusCode()
				}
			}
			if err != nil || result == nil || (status != 200 && !(operation == "publish" && status == 202)) {
				return errors.New("workflow request failed; query workflow get with the same ID before retrying publish")
			}
			return writeWorkflowSummary(c, struct {
				ID                string `json:"id"`
				Version           int32  `json:"version"`
				Status            string `json:"status"`
				PublicationID     string `json:"publication_id,omitempty"`
				PublicationStatus string `json:"publication_status,omitempty"`
			}{result.GetId(), result.GetVersion(), result.GetStatus().String(), result.GetPublicationId(), result.GetPublicationStatus()})
		}}
		command.Flags().StringVar(&id, "id", "", "Workflow ID")
		_ = command.MarkFlagRequired("id")
		group.AddCommand(command)
	}
	return group
}

func init() { Cmd.AddCommand(newWorkflowCommand()) }
