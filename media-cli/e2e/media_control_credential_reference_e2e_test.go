package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func TestMediaControlCredentialReferenceE2E(t *testing.T) {
	authAddr, controlAddr := os.Getenv("MEDIA_AUTH_ADDR"), os.Getenv("MEDIA_CONTROL_ADDR")
	if authAddr == "" || controlAddr == "" { t.Skip("MEDIA_AUTH_ADDR and MEDIA_CONTROL_ADDR are required") }
	client := httpClient(t)
	login(t, client, authAddr, envOr("MEDIA_AUTH_E2E_USER", "smoke"), envOr("MEDIA_AUTH_E2E_PASS", "smoke123"))
	status, body := doJSON(t, client, http.MethodPost, controlAddr+"/api/v1/providers", map[string]string{"name":fmt.Sprintf("cred-provider-%d",time.Now().UnixNano()),"provider_type":"model","endpoint":"https://provider.example","capabilities_json":`{}`})
	if status != http.StatusCreated { t.Fatalf("create provider: status=%d body=%s",status,body) }
	var provider struct{ID string};if err:=json.Unmarshal(body,&provider);err!=nil{t.Fatal(err)}
	secretRef := "vault://media-agent/provider/demo"
	status,body=doJSON(t,client,http.MethodPost,controlAddr+"/api/v1/credential-references",map[string]string{"name":"demo-ref","provider_id":provider.ID,"secret_ref":secretRef})
	if status!=http.StatusCreated{t.Fatalf("create credential reference: status=%d body=%s",status,body)}
	if jsonContains(body,"secret_ref")||jsonContains(body,secretRef){t.Fatalf("credential response exposed secret reference: %s",body)}
	var credential struct{ID,Status string;Version int64};if err:=json.Unmarshal(body,&credential);err!=nil{t.Fatal(err)}
	if credential.ID==""||credential.Status!="active"||credential.Version!=1{t.Fatalf("unexpected credential: %+v",credential)}
	status,body=doJSON(t,client,http.MethodPost,controlAddr+"/api/v1/credential-references/"+credential.ID+"/revoke",map[string]any{"expected_version":1})
	if status!=http.StatusOK||!jsonContains(body,`"status":"revoked"`){t.Fatalf("revoke credential: status=%d body=%s",status,body)}
}
