package media_workflow

import (
	"errors"
	"github.com/spf13/cobra"
	cap "media_agent/hertz_gen/model/media_workflow/capability"
	tpl "media_agent/hertz_gen/model/media_workflow/template"
	wf "media_agent/hertz_gen/model/media_workflow/workflow"
)

var errQueryFailed = errors.New("resource query failed; response payload withheld")

func newCapabilityCommand() *cobra.Command {
	group := &cobra.Command{Use: "capability", Short: "查询已发布 Provider 能力契约"}
	var provider, method, kind, target string
	var page, size int32
	list := &cobra.Command{Use: "list", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if CapabilityCli == nil {
			return errQueryFailed
		}
		result, raw, err := CapabilityCli.ListCapabilities(c.Context(), &cap.ListCapabilitiesReq{Provider: provider, GenerationMethod: method, GenerationType: kind, ExecutionTarget: target, Page: page, PageSize: size})
		if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil {
			return errQueryFailed
		}
		return writeWorkflowSummary(c, result)
	}}
	list.Flags().StringVar(&provider, "provider", "", "Provider")
	list.Flags().StringVar(&method, "generation-method", "", "Generation method")
	list.Flags().StringVar(&kind, "generation-type", "", "first_frame_prompt | multi_image_prompt | text_prompt")
	list.Flags().StringVar(&target, "execution-target", "", "personal_worker | managed_worker")
	list.Flags().Int32Var(&page, "page", 1, "Page")
	list.Flags().Int32Var(&size, "page-size", 50, "Page size, at most 200")
	var key string
	var version int32
	get := &cobra.Command{Use: "get", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if CapabilityCli == nil || version <= 0 {
			return errQueryFailed
		}
		result, raw, err := CapabilityCli.GetCapabilityVersion(c.Context(), &cap.GetCapabilityVersionReq{Key: key, Version: version})
		if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil {
			return errQueryFailed
		}
		return writeWorkflowSummary(c, result)
	}}
	get.Flags().StringVar(&key, "key", "", "Capability key")
	get.Flags().Int32Var(&version, "version", 0, "Immutable version")
	_ = get.MarkFlagRequired("key")
	_ = get.MarkFlagRequired("version")
	group.AddCommand(list, get)
	return group
}

func newTemplateQueryCommand() *cobra.Command {
	group := &cobra.Command{Use: "template", Short: "查询模板权限与授权"}
	for _, op := range []string{"get", "grants"} {
		var id string
		command := &cobra.Command{Use: op, Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
			if TemplateCli == nil {
				return errQueryFailed
			}
			if op == "grants" {
				result, raw, err := TemplateCli.ListTemplateGrants(c.Context(), &tpl.GetTemplateReq{Id: id})
				if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil {
					return errQueryFailed
				}
				return writeWorkflowSummary(c, result)
			}
			result, raw, err := TemplateCli.GetTemplate(c.Context(), &tpl.GetTemplateReq{Id: id})
			if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil {
				return errQueryFailed
			}
			return writeWorkflowSummary(c, struct {
				ID         string   `json:"id"`
				Version    int32    `json:"version"`
				Permission string   `json:"my_permission"`
				Actions    []string `json:"allowed_actions"`
			}{result.GetId(), result.GetVersion(), result.GetMyPermission(), result.GetAllowedActions()})
		}}
		command.Flags().StringVar(&id, "id", "", "Template ID")
		_ = command.MarkFlagRequired("id")
		group.AddCommand(command)
	}
	return group
}

func newWorkflowVersionCommand() *cobra.Command {
	var id string
	var version int32
	command := &cobra.Command{Use: "version", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if WfCli == nil || version <= 0 {
			return errQueryFailed
		}
		result, raw, err := WfCli.GetWorkflowVersion(c.Context(), &wf.GetWorkflowVersionReq{Id: id, Version: version})
		if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil {
			return errQueryFailed
		}
		return writeWorkflowSummary(c, struct {
			ID         string   `json:"id"`
			Version    int32    `json:"version"`
			Source     string   `json:"definition_source"`
			StepCount  int      `json:"step_count"`
			Permission string   `json:"my_permission"`
			Actions    []string `json:"allowed_actions"`
		}{result.GetId(), result.GetVersion(), result.GetDefinitionSource(), len(result.GetSteps()), result.GetMyPermission(), result.GetAllowedActions()})
	}}
	command.Flags().StringVar(&id, "id", "", "Workflow ID")
	command.Flags().Int32Var(&version, "version", 0, "Frozen version")
	_ = command.MarkFlagRequired("id")
	_ = command.MarkFlagRequired("version")
	return command
}

func newWorkflowGrantsCommand() *cobra.Command {
	var id string
	command := &cobra.Command{Use: "grants", Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		if WfCli == nil {
			return errQueryFailed
		}
		result, raw, err := WfCli.ListWorkflowGrants(c.Context(), &wf.GetWorkflowReq{Id: id})
		if err != nil || raw == nil || raw.StatusCode() != 200 || result == nil {
			return errQueryFailed
		}
		return writeWorkflowSummary(c, result)
	}}
	command.Flags().StringVar(&id, "id", "", "Workflow ID")
	_ = command.MarkFlagRequired("id")
	return command
}

func init() { Cmd.AddCommand(newCapabilityCommand(), newTemplateQueryCommand()) }
