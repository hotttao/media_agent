package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaAgentGatewayAgentTaskE2E(t *testing.T){
	authAddr,gatewayAddr,controlAddr:=os.Getenv("MEDIA_AUTH_ADDR"),os.Getenv("MEDIA_AGENT_GATEWAY_ADDR"),os.Getenv("MEDIA_CONTROL_ADDR");if authAddr==""||gatewayAddr==""||controlAddr==""{t.Skip("MEDIA_AUTH_ADDR, MEDIA_AGENT_GATEWAY_ADDR and MEDIA_CONTROL_ADDR are required")}
	owner:=registerGatewayUser(t,authAddr,"task-owner");status,body:=doJSON(t,owner,http.MethodPost,controlAddr+"/api/v1/workspaces",map[string]string{"name":fmt.Sprintf("task-%d",time.Now().UnixNano()),"agent_type":"codex"});if status!=http.StatusCreated{t.Fatalf("workspace: %d %s",status,body)};var workspace struct{ID string `json:"id"`};if err:=json.Unmarshal(body,&workspace);err!=nil{t.Fatal(err)}
	status,body=doJSON(t,owner,http.MethodPost,gatewayAddr+"/api/chat/sessions",map[string]string{"workspace_id":workspace.ID,"title":"Task E2E"});if status!=http.StatusCreated{t.Fatalf("session: %d %s",status,body)};var session struct{ID string `json:"id"`};if err:=json.Unmarshal(body,&session);err!=nil{t.Fatal(err)}
	request:=map[string]string{"workspace_id":workspace.ID,"session_id":session.ID,"source":"manual","source_resource_id":"request-1","idempotency_key":"task-key-1","input_json":`{"message":"hello"}`};status,body=doJSON(t,owner,http.MethodPost,gatewayAddr+"/api/v1/agent-tasks",request);if status!=http.StatusCreated{t.Fatalf("create task: %d %s",status,body)};var task struct{ID,Status string;Attempt int32};if err:=json.Unmarshal(body,&task);err!=nil{t.Fatal(err)};if task.ID==""||task.Status!="queued"||task.Attempt!=0{t.Fatalf("unexpected task: %+v",task)}
	status,body=doJSON(t,owner,http.MethodPost,gatewayAddr+"/api/v1/agent-tasks",request);if status!=http.StatusCreated{t.Fatalf("idempotent create: %d %s",status,body)};var repeated struct{ID string};json.Unmarshal(body,&repeated);if repeated.ID!=task.ID{t.Fatalf("idempotency returned %q, want %q",repeated.ID,task.ID)}
	other:=registerGatewayUser(t,authAddr,"task-other");status,body=doJSON(t,other,http.MethodGet,gatewayAddr+"/api/v1/agent-tasks/"+task.ID,nil);if status!=http.StatusForbidden{t.Fatalf("cross-user get: %d %s",status,body)}
	status,body=doJSON(t,owner,http.MethodPost,gatewayAddr+"/api/v1/agent-tasks/"+task.ID+"/cancel",map[string]any{});if status!=http.StatusOK||!jsonContains(body,`"status":"cancelled"`){t.Fatalf("cancel: %d %s",status,body)}
}
