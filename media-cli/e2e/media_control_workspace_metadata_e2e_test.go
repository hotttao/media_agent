package e2e

import("encoding/json";"fmt";"net/http";"os";"testing";"time")

func TestMediaControlWorkspaceMetadataE2E(t *testing.T){
	a,ctrl:=os.Getenv("MEDIA_AUTH_ADDR"),os.Getenv("MEDIA_CONTROL_ADDR");if a==""||ctrl==""{t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")};c:=httpClient(t);login(t,c,a,envOr("MEDIA_AUTH_E2E_USER","smoke"),envOr("MEDIA_AUTH_E2E_PASS","smoke123"))
	s,b:=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/workspaces",map[string]string{"name":fmt.Sprintf("metadata-%d",time.Now().UnixNano()),"agent_type":"codex"});if s!=http.StatusCreated{t.Fatalf("workspace: %d %s",s,b)};var ws struct{ID string};json.Unmarshal(b,&ws)
	s,b=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/workspace-tags",map[string]string{"workspace_id":ws.ID,"name":"urgent","color":"#ff0000"});if s!=http.StatusCreated{t.Fatalf("tag: %d %s",s,b)};var tag struct{ID string;Version int64};json.Unmarshal(b,&tag);if tag.ID==""||tag.Version!=1{t.Fatalf("unexpected tag: %+v",tag)}
	s,b=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/workspace-tags/"+tag.ID+"/attach",map[string]string{"workspace_id":ws.ID,"resource_type":"prompt","resource_id":"prompt-1"});if s!=http.StatusCreated{t.Fatalf("attach: %d %s",s,b)}
	s,b=doJSON(t,c,http.MethodGet,ctrl+"/api/v1/workspace-resources/prompt/prompt-1/tags?workspace_id="+ws.ID,nil);if s!=http.StatusOK||!jsonContains(b,`"name":"urgent"`){t.Fatalf("list resource tags: %d %s",s,b)}
	s,b=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/workspace-tags/"+tag.ID+"/detach",map[string]string{"workspace_id":ws.ID,"resource_type":"prompt","resource_id":"prompt-1"});if s!=http.StatusOK{t.Fatalf("detach: %d %s",s,b)}
	s,b=doJSON(t,c,http.MethodPut,ctrl+"/api/v1/workspace-tags/"+tag.ID,map[string]any{"name":"urgent-updated","color":"#00ff00","expected_version":1});if s!=http.StatusOK||!jsonContains(b,`"version":2`){t.Fatalf("update: %d %s",s,b)}
	s,b=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/workspace-tags/"+tag.ID+"/delete",map[string]any{"expected_version":2});if s!=http.StatusOK{t.Fatalf("delete: %d %s",s,b)}
}
