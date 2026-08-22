package e2e

import("encoding/json";"fmt";"net/http";"os";"testing";"time")

func TestMediaControlTeamworkE2E(t *testing.T){
	a,ctrl:=os.Getenv("MEDIA_AUTH_ADDR"),os.Getenv("MEDIA_CONTROL_ADDR");if a==""||ctrl==""{t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required")};c:=httpClient(t);login(t,c,a,envOr("MEDIA_AUTH_E2E_USER","smoke"),envOr("MEDIA_AUTH_E2E_PASS","smoke123"))
	s,b:=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/workspaces",map[string]string{"name":fmt.Sprintf("teamwork-%d",time.Now().UnixNano()),"agent_type":"codex"});if s!=http.StatusCreated{t.Fatalf("workspace: %d %s",s,b)};var ws struct{ID string};if err:=json.Unmarshal(b,&ws);err!=nil{t.Fatal(err)}
	members:=`[{"member_id":"writer","agent_config_id":"cfg-writer","name":"Writer"},{"member_id":"reviewer","agent_config_id":"cfg-reviewer","name":"Reviewer"}]`;edges:=`[{"from_member_id":"writer","to_member_id":"reviewer","mode":"delegate"}]`;s,b=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/teamwork-topologies",map[string]string{"workspace_id":ws.ID,"name":"content-review","entry_member_id":"writer","members_json":members,"edges_json":edges});if s!=http.StatusCreated{t.Fatalf("topology: %d %s",s,b)};var topo struct{ID string;Version int64;Status string};if err:=json.Unmarshal(b,&topo);err!=nil{t.Fatal(err)};if topo.ID==""||topo.Version!=1||topo.Status!="active"{t.Fatalf("unexpected topology: %+v",topo)}
	s,b=doJSON(t,c,http.MethodPut,ctrl+"/api/v1/teamwork-topologies/"+topo.ID,map[string]any{"name":"content-review-v2","entry_member_id":"writer","members_json":members,"edges_json":edges,"expected_version":1});if s!=http.StatusOK||!jsonContains(b,`"version":2`){t.Fatalf("update topology: %d %s",s,b)}
	s,b=doJSON(t,c,http.MethodPost,ctrl+"/api/v1/teamwork-topologies/"+topo.ID+"/delete",map[string]any{"expected_version":2});if s!=http.StatusOK||!jsonContains(b,`"status":"deleted"`){t.Fatalf("delete topology: %d %s",s,b)}
}
