package media_workflow

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	capsvc "media_agent/hertz_gen/media_workflow/capability_service"
	tplsvc "media_agent/hertz_gen/media_workflow/template_service"
	wfsvc "media_agent/hertz_gen/media_workflow/workflow_service"
)

func TestQueryCommandsUseTypedRoutesAndSafeOutput(t *testing.T) {
	for _, tc := range []struct {
		name, method, path, body string
		args                     []string
		command                  func() *cobra.Command
	}{
		{"capability list", "GET", "/api/v1/capabilities", `{"items":[{"key":"jimeng.i2i.multi_image_prompt","version":1}],"total":1,"conductor_task_def":"private-internal"}`, []string{"list", "--provider", "jimeng"}, newCapabilityCommand},
		{"capability version", "GET", "/api/v1/capabilities/jimeng.i2i.multi_image_prompt/versions/1", `{"key":"jimeng.i2i.multi_image_prompt","version":1,"managed_pool_key":"private-internal"}`, []string{"get", "--key", "jimeng.i2i.multi_image_prompt", "--version", "1"}, newCapabilityCommand},
		{"template access", "GET", "/api/v1/templates/tpl", `{"id":"tpl","version":2,"my_permission":"editor","allowed_actions":["read","update"],"prompt_body":"private-prompt"}`, []string{"get", "--id", "tpl"}, newTemplateQueryCommand},
		{"template grants", "GET", "/api/v1/templates/tpl/grants", `{"items":[{"team_id":"dept","permission":"viewer"}]}`, []string{"grants", "--id", "tpl"}, newTemplateQueryCommand},
		{"workflow version", "GET", "/api/v1/workflows/wf/versions/1", `{"id":"wf","version":1,"definition_source":"snapshot","my_permission":"owner","allowed_actions":["read"],"steps":[{"key":"image","slot_bindings":{"text":{"const_value":"private-prompt"}}}]}`, []string{"version", "--id", "wf", "--version", "1"}, newWorkflowCommand},
		{"workflow grants", "GET", "/api/v1/workflows/wf/grants", `{"items":[{"team_id":"dept","permission":"editor"}]}`, []string{"grants", "--id", "wf"}, newWorkflowCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, status := range []int{200, 403, 503} {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					if r.Method != tc.method || r.URL.Path != tc.path {
						t.Error("wrong typed route")
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = w.Write([]byte(tc.body))
				}))
				oldCap, oldTpl, oldWf := CapabilityCli, TemplateCli, WfCli
				var err error
				CapabilityCli, err = capsvc.NewCapabilityServiceClient(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				TemplateCli, err = tplsvc.NewTemplateServiceClient(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				WfCli, err = wfsvc.NewWorkflowServiceClient(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				command := tc.command()
				var output bytes.Buffer
				command.SetOut(&output)
				command.SetErr(&output)
				command.SetArgs(tc.args)
				err = command.Execute()
				CapabilityCli, TemplateCli, WfCli = oldCap, oldTpl, oldWf
				server.Close()
				if calls != 1 || (err == nil) != (status == 200) {
					t.Fatal("wrong status or repeated request")
				}
				if strings.Contains(output.String(), "private-") || (err != nil && strings.Contains(err.Error(), "private-")) {
					t.Fatal("private response leaked")
				}
			}
		})
	}
}
