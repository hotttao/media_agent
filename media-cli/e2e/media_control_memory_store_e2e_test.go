package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlMemoryStoreE2E(t *testing.T) {
	authAddr, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || controlAddr == "" { t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required") }
	client:=httpClient(t);login(t,client,authAddr,envOr("MEDIA_AUTH_E2E_USER","smoke"),envOr("MEDIA_AUTH_E2E_PASS","smoke123"))
	status,body:=doJSON(t,client,http.MethodPost,controlAddr+"/api/v1/workspaces",map[string]string{"name":fmt.Sprintf("memory-%d",time.Now().UnixNano()),"agent_type":"codex"});if status!=http.StatusCreated{t.Fatalf("create workspace: status=%d body=%s",status,body)};var ws struct{ID string};if err:=json.Unmarshal(body,&ws);err!=nil{t.Fatal(err)}
	status,body=doJSON(t,client,http.MethodPost,controlAddr+"/api/v1/memory-stores",map[string]string{"workspace_id":ws.ID,"name":"memory","store_type":"redis","configuration_json":`{"database":0}`});if status!=http.StatusCreated{t.Fatalf("create memory store: status=%d body=%s",status,body)};var store struct{ID,Status string;Version int64};if err:=json.Unmarshal(body,&store);err!=nil{t.Fatal(err)};if store.ID==""||store.Status!="active"||store.Version!=1{t.Fatalf("unexpected store: %+v",store)}
	status,body=doJSON(t,client,http.MethodPost,controlAddr+"/api/v1/memory-stores/"+store.ID+"/status",map[string]any{"status":"disabled","expected_version":1});if status!=http.StatusOK||!jsonContains(body,`"status":"disabled"`){t.Fatalf("disable store: status=%d body=%s",status,body)}
}
