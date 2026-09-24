package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMySQLDSNConvertsPrismaURL(t *testing.T) {
	got, err := mysqlDSN("mysql://user:p%40ss@db.example:3307/media_ai?loc=Local")
	if err != nil {
		t.Fatal(err)
	}
	want := "user:p@ss@tcp(db.example:3307)/media_ai?charset=utf8mb4&loc=Local&parseTime=true"
	if got != want {
		t.Fatalf("mysqlDSN() = %q, want %q", got, want)
	}
}

func TestParseLegacyLists(t *testing.T) {
	if got, want := parseStringList(`["直播","女装"]`), []string{"直播", "女装"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON tags = %#v, want %#v", got, want)
	}
	if got, want := parseStringList("展示, 转身\n特写"), []string{"展示", "转身", "特写"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("plain tags = %#v, want %#v", got, want)
	}
}

func TestFindExistingPrefersMigrationSourceID(t *testing.T) {
	metadata, _ := json.Marshal(`{"migration":{"source_id":"source-2"}}`)
	items := []targetItem{
		{ID: "same-name", Name: "商品"},
		{ID: "source-match", Name: "旧名称", Metadata: metadata},
	}
	if got := findExisting(items, "source-2", "商品", ""); got != "source-match" {
		t.Fatalf("findExisting() = %q, want source-match", got)
	}
}

func TestMigrateImageDownloadsAndUploadsWithStableKey(t *testing.T) {
	image := []byte("\x89PNG\r\n\x1a\ntest-image")
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(image)
	}))
	defer source.Close()

	var keys []string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/library-objects:upload" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if cookie, err := r.Cookie("ory_kratos_session"); err != nil || cookie.Value != "session" {
			t.Fatalf("session cookie missing: %v", err)
		}
		keys = append(keys, r.Header.Get("Idempotency-Key"))
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatal(err)
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		got, _ := io.ReadAll(file)
		if !reflect.DeepEqual(got, image) {
			t.Fatalf("uploaded image differs")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object_path":"materials/org/scope/file.png","checksum":"abc"}`)
	}))
	defer target.Close()

	client := &http.Client{Timeout: time.Second}
	m := &migrator{
		cfg:        config{sourceAssetsURL: source.URL},
		sourceHTTP: client,
		logger:     log.New(io.Discard, "", 0),
		target: &targetClient{baseURL: target.URL, organizationID: "org", scope: "scope", sessionCookieName: "ory_kratos_session",
			sessionCookie: "session", http: client},
	}
	for range 2 {
		got, err := m.migrateImage(context.Background(), "material", "source-1", "file", "/image.png", "material")
		if err != nil {
			t.Fatal(err)
		}
		if got != "materials/org/scope/file.png" {
			t.Fatalf("object path = %q", got)
		}
	}
	if len(keys) != 2 || keys[0] == "" || keys[0] != keys[1] || !strings.HasPrefix(keys[0], "media-ai-") {
		t.Fatalf("idempotency keys = %#v", keys)
	}
}

func TestMigrateMovementPreservesDomainFields(t *testing.T) {
	var payload map[string]any
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/materials" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, `{"id":"42"}`)
	}))
	defer target.Close()
	client := &http.Client{Timeout: time.Second}
	m := &migrator{cfg: config{scope: "ai_clothing"}, sourceHTTP: client, logger: log.New(io.Discard, "", 0), target: &targetClient{
		baseURL: target.URL, organizationID: "org", scope: "ai_clothing", http: client,
	}}
	item := sourceMovement{ID: "legacy-movement-1", Content: "转身展示", IsGeneral: false, PoseIDs: []string{"pose-1"}, CreatedAt: time.Now()}
	if err := m.migrateMovement(context.Background(), item, nil); err != nil {
		t.Fatal(err)
	}
	if payload["asset_kind"] != "movement" || payload["content"] != "转身展示" || payload["object_path"] != "" {
		t.Fatalf("unexpected movement payload: %#v", payload)
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(payload["metadata"].(string)), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["is_general"] != false || !reflect.DeepEqual(metadata["legacy_pose_ids"], []any{"pose-1"}) {
		t.Fatalf("movement metadata lost: %#v", metadata)
	}
}
