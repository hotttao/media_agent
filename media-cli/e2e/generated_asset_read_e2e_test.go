package e2e

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestImportedGeneratedAssetGatewayRead(t *testing.T) {
	addr, token, org, id := os.Getenv("MEDIA_GATEWAY_ADDR"), os.Getenv("MEDIA_API_TOKEN"), os.Getenv("MEDIA_E2E_ORGANIZATION_ID"), os.Getenv("MEDIA_E2E_IMPORTED_ASSET_ID")
	if addr == "" || token == "" || org == "" || id == "" {
		t.Skip("gateway, User API Token, Organization and imported Asset ID required")
	}
	status, body := doJSONBearer(t, addr, http.MethodGet, "/api/v1/generated-assets/"+id, token, nil)
	if status != http.StatusOK {
		t.Fatalf("generated asset detail status=%d", status)
	}
	var asset struct {
		ID         string `json:"id"`
		Origin     string `json:"origin"`
		ObjectPath string `json:"object_path"`
		URL        string `json:"url"`
	}
	if err := json.Unmarshal(body, &asset); err != nil || asset.ID != id || asset.Origin != "legacy_import" ||
		!strings.HasPrefix(asset.ObjectPath, "materials/"+org+"/") || asset.URL == "" {
		t.Fatalf("invalid imported Generated Asset projection: id=%s origin=%s err=%v", asset.ID, asset.Origin, err)
	}
	request, err := http.NewRequest(http.MethodGet, asset.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=0-0")
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("asset media is unreadable: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusPartialContent && response.StatusCode != http.StatusOK {
		t.Fatalf("asset media returned HTTP %d", response.StatusCode)
	}
	status, body = doJSONBearer(t, addr, http.MethodGet, "/api/v1/generated-assets?page_size=200", token, nil)
	if status != http.StatusOK {
		t.Fatalf("generated asset list status=%d", status)
	}
	var page struct {
		Items []struct {
			ID  string `json:"id"`
			URL string `json:"url"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID == id {
			if item.URL == "" {
				t.Fatal("list omitted signed URL for imported Generated Asset")
			}
			return
		}
	}
	t.Fatalf("imported Generated Asset %s missing from authorized list", id)
}
