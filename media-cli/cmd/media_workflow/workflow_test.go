package media_workflow

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	wfsvc "media_agent/hertz_gen/media_workflow/workflow_service"
)

func TestWorkflowPublicationCommandReceiptsAndSafeFailures(t *testing.T) {
	for _, status := range []int{200, 202, 409, 503} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != "POST" || r.URL.Path != "/api/v1/workflows/wf/publish" {
					t.Error("incorrect request")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"id":"wf","version":1,"status":4,"publication_id":"receipt","publication_status":"pending","description":"private-prompt","steps":[{"template_id":"private-template"}]}`))
			}))
			defer server.Close()
			old := WfCli
			defer func() { WfCli = old }()
			var err error
			WfCli, err = wfsvc.NewWorkflowServiceClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			command := newWorkflowCommand()
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs([]string{"publish", "--id", "wf"})
			err = command.Execute()
			if (err == nil) != (status == 200 || status == 202) || requests.Load() != 1 {
				t.Fatal("incorrect outcome or repeated publication")
			}
			if strings.Contains(output.String(), "private-") || (err != nil && strings.Contains(err.Error(), "private-")) {
				t.Fatal("private payload exposed")
			}
			if err == nil && !strings.Contains(output.String(), `"publication_id":"receipt"`) {
				t.Fatal("missing receipt")
			}
		})
	}
}
