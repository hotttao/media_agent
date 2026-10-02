// media_ai_history_migrate inventories the source and imports one verified
// image lineage at a time. Historical Jobs and resume are separate stages.
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type workflow struct {
	ID      string          `json:"id"`
	Code    string          `json:"code"`
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Config  json.RawMessage `json:"config,omitempty"`
}

type workflowTemplate struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	VirtualIPID string    `json:"virtual_ip_id"`
	PoseID      string    `json:"pose_id"`
	SceneID     string    `json:"scene_id"`
	MovementIDs []string  `json:"movement_ids"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type video struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"task_id"`
	UserID       string    `json:"user_id"`
	TeamID       string    `json:"team_id"`
	URL          string    `json:"url"`
	Name         string    `json:"name"`
	Thumbnail    string    `json:"thumbnail"`
	Prompt       string    `json:"prompt"`
	Duration     *int64    `json:"duration,omitempty"`
	Size         *int64    `json:"size,omitempty"`
	ProductID    string    `json:"product_id"`
	IPID         string    `json:"ip_id"`
	SceneID      string    `json:"scene_id"`
	PoseID       string    `json:"pose_id"`
	MovementID   string    `json:"movement_id"`
	FirstFrameID string    `json:"first_frame_id"`
	StyleImageID string    `json:"style_image_id"`
	ModelImageID string    `json:"model_image_id"`
	CreatedAt    time.Time `json:"created_at"`
}

type videoTask struct {
	ID             string          `json:"id"`
	UserID         string          `json:"user_id"`
	TeamID         string          `json:"team_id"`
	WorkflowID     string          `json:"workflow_id"`
	IPID           string          `json:"ip_id"`
	Status         string          `json:"status"`
	Params         json.RawMessage `json:"params,omitempty"`
	Result         json.RawMessage `json:"result,omitempty"`
	Error          string          `json:"error,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	Videos         []video         `json:"videos"`
	Classification string          `json:"classification"`
}

type manifest struct {
	SchemaVersion int                `json:"schema_version"`
	Source        string             `json:"source"`
	Workflows     []workflow         `json:"workflows"`
	Templates     []workflowTemplate `json:"workflow_templates"`
	Tasks         []videoTask        `json:"video_tasks"`
	ModelImages   []sourceImage      `json:"model_images"`
	StyleImages   []sourceImage      `json:"style_images"`
	FirstFrames   []sourceImage      `json:"first_frames"`
	VideoPushes   []sourceVideoPush  `json:"video_pushes"`
	Lineages      []videoLineage     `json:"video_lineages"`
	Groups        []lineageGroup     `json:"lineage_groups"`
	Issues        []lineageIssue     `json:"issues"`
	Excluded      []lineageExclusion `json:"excluded"`
}

func main() {
	var sourceDSN, outputPath, onlyTask, importGroup, inspectGroup string
	var auditReadiness bool
	var importCfg importConfig
	flag.StringVar(&sourceDSN, "source-dsn", os.Getenv("MEDIA_AI_DATABASE_URL"), "media_ai MySQL DSN; prefer environment variable")
	flag.StringVar(&outputPath, "out", "", "write private JSON manifest to a new file (0600)")
	flag.StringVar(&onlyTask, "only-task", "", "export one legacy video_task; omit for all data")
	flag.StringVar(&importGroup, "import-group", "", "import one intact image-only lineage group through target APIs")
	flag.StringVar(&inspectGroup, "inspect-group", "", "inspect one source lineage and its pose/scene dependencies without importing")
	flag.BoolVar(&auditReadiness, "audit-readiness", false, "read-only audit of all candidate input mappings and source media")
	flag.StringVar(&importCfg.SourceAssetsURL, "source-assets-url", envOr("MEDIA_AI_ASSETS_URL", "http://192.168.2.38"), "legacy media origin")
	flag.StringVar(&importCfg.GatewayURL, "target-gateway", envOr("MEDIA_GATEWAY_ADDR", "http://192.168.2.41:18080"), "target public gateway")
	flag.StringVar(&importCfg.MaterialsURL, "materials-internal-url", envOr("MEDIA_MATERIALS_INTERNAL_URL", "http://127.0.0.1:8003"), "trusted materials API")
	flag.StringVar(&importCfg.OrganizationID, "organization-id", os.Getenv("MEDIA_MATERIALS_ORGANIZATION_ID"), "target organization")
	flag.StringVar(&importCfg.OwnerUserID, "owner-user-id", os.Getenv("MEDIA_MATERIALS_OWNER_USER_ID"), "target owner")
	flag.StringVar(&importCfg.ProductID, "product-id", "", "mapped target product for one group")
	flag.StringVar(&importCfg.VirtualIPID, "virtual-ip-id", "", "mapped target virtual IP for one group")
	flag.StringVar(&importCfg.BearerToken, "api-token", os.Getenv("MEDIA_MATERIALS_BEARER_TOKEN"), "target API token; prefer environment")
	flag.StringVar(&importCfg.MigrationToken, "migration-token", os.Getenv("MEDIA_MATERIALS_MIGRATION_TOKEN"), "migration-only internal token; prefer environment")
	flag.BoolVar(&importCfg.ImportJob, "import-job", false, "also create one awaiting_resume Job after importing its images")
	flag.StringVar(&importCfg.WorkflowURL, "workflow-internal-url", envOr("MEDIA_WORKFLOW_INTERNAL_URL", "http://127.0.0.1:8002"), "trusted media_workflow API")
	flag.StringVar(&importCfg.WorkflowToken, "workflow-migration-token", os.Getenv("MEDIA_WORKFLOW_MIGRATION_TOKEN"), "workflow migration-only internal token; prefer environment")
	flag.StringVar(&importCfg.WorkflowID, "workflow-id", "", "mapped published four-step workflow")
	flag.IntVar(&importCfg.WorkflowVersion, "workflow-version", 1, "published workflow version")
	flag.StringVar(&importCfg.WorkspaceID, "workspace-id", "", "target Workspace owned by the organization")
	flag.Parse()
	if importGroup != "" && onlyTask != "" {
		fail(errors.New("--import-group and --only-task cannot be combined"))
	}
	if inspectGroup != "" && (importGroup != "" || onlyTask != "") {
		fail(errors.New("--inspect-group cannot be combined with --import-group or --only-task"))
	}
	if auditReadiness && (importGroup != "" || inspectGroup != "" || onlyTask != "") {
		fail(errors.New("--audit-readiness requires the full source inventory and cannot import"))
	}
	if importCfg.ImportJob && importGroup == "" {
		fail(errors.New("--import-job requires --import-group"))
	}
	if sourceDSN == "" {
		fail(errors.New("MEDIA_AI_DATABASE_URL is required"))
	}
	dsn, err := mysqlDSN(sourceDSN)
	if err != nil {
		fail(err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fail(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	timeout := 2 * time.Minute
	if importGroup != "" || auditReadiness {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fail(fmt.Errorf("connect source: %w", err))
	}
	m, err := readManifest(ctx, db, onlyTask)
	if err != nil {
		fail(err)
	}
	counts := map[string]int{}
	videoCount := 0
	matchedTemplateCount := 0
	groupKinds := map[string]int{}
	exclusionReasons := map[string]int{}
	groupedVideos := map[string]bool{}
	for _, task := range m.Tasks {
		counts[task.Classification]++
		videoCount += len(task.Videos)
	}
	for _, lineage := range m.Lineages {
		if len(lineage.TemplateIDs) > 0 {
			matchedTemplateCount++
		}
	}
	for _, group := range m.Groups {
		groupKinds[group.Kind]++
		for _, id := range group.VideoIDs {
			groupedVideos[id] = true
		}
	}
	for _, exclusion := range m.Excluded {
		exclusionReasons[exclusion.Kind+":"+exclusion.Reason]++
	}
	fmt.Printf("source inventory: workflows=%d workflow_templates=%d model_images=%d style_images=%d first_frames=%d video_tasks=%d videos=%d video_pushes=%d lineage_groups=%d excluded=%d template_matched_videos=%d issues=%d classes=%v\n", len(m.Workflows), len(m.Templates), len(m.ModelImages), len(m.StyleImages), len(m.FirstFrames), len(m.Tasks), videoCount, len(m.VideoPushes), len(m.Groups), len(m.Excluded), matchedTemplateCount, len(m.Issues), counts)
	fmt.Printf("migration candidates: unique_videos=%d group_kinds=%v exclusion_reasons=%v\n", len(groupedVideos), groupKinds, exclusionReasons)
	if inspectGroup != "" {
		if err := inspectLineageGroup(m, inspectGroup); err != nil {
			fail(err)
		}
	}
	if auditReadiness {
		if err := auditMigrationReadiness(ctx, db, m, importCfg); err != nil {
			fail(err)
		}
	}
	if importGroup != "" {
		if err := importImageGroup(ctx, db, &m, importGroup, importCfg); err != nil {
			fail(err)
		}
	}
	if outputPath == "" {
		return
	}
	encoded, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fail(err)
	}
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		fail(err)
	}
	if _, err := output.Write(append(encoded, '\n')); err != nil {
		output.Close()
		fail(err)
	}
	if err := output.Close(); err != nil {
		fail(err)
	}
	sum := sha256.Sum256(encoded)
	fmt.Printf("manifest=%s sha256=%s\n", outputPath, hex.EncodeToString(sum[:]))
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }

func mysqlDSN(raw string) (string, error) {
	if !strings.HasPrefix(raw, "mysql://") {
		if strings.Contains(raw, "parseTime=") {
			return raw, nil
		}
		sep := "?"
		if strings.Contains(raw, "?") {
			sep = "&"
		}
		return raw + sep + "parseTime=true&charset=utf8mb4", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.User == nil || u.User.Username() == "" || strings.TrimPrefix(u.Path, "/") == "" {
		return "", errors.New("source DSN needs user and database")
	}
	password, _ := u.User.Password()
	host := u.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "3306")
	}
	params := u.Query()
	params.Set("parseTime", "true")
	params.Set("charset", "utf8mb4")
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s", u.User.Username(), password, host, strings.TrimPrefix(u.Path, "/"), params.Encode()), nil
}

func readManifest(ctx context.Context, db *sql.DB, onlyTask string) (manifest, error) {
	m := manifest{SchemaVersion: 1, Source: "media_ai"}
	rows, err := db.QueryContext(ctx, "SELECT id, code, name, version, config FROM workflows ORDER BY id")
	if err != nil {
		return m, fmt.Errorf("workflows: %w", err)
	}
	for rows.Next() {
		var w workflow
		var config sql.NullString
		if err := rows.Scan(&w.ID, &w.Code, &w.Name, &w.Version, &config); err != nil {
			rows.Close()
			return m, err
		}
		if config.Valid {
			if json.Valid([]byte(config.String)) {
				w.Config = json.RawMessage(config.String)
			} else {
				w.Config, _ = json.Marshal(config.String)
			}
		}
		m.Workflows = append(m.Workflows, w)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return m, err
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, "SELECT id, name, virtual_ip_id, pose_id, scene_id, created_at, updated_at FROM workflow_templates ORDER BY id")
	if err != nil {
		return m, fmt.Errorf("workflow_templates: %w", err)
	}
	for rows.Next() {
		var t workflowTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.VirtualIPID, &t.PoseID, &t.SceneID, &t.CreatedAt, &t.UpdatedAt); err != nil {
			rows.Close()
			return m, err
		}
		m.Templates = append(m.Templates, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return m, err
	}
	rows.Close()
	links, err := db.QueryContext(ctx, "SELECT template_id, movement_id FROM workflow_template_movements ORDER BY template_id, movement_id")
	if err != nil {
		return m, fmt.Errorf("workflow_template_movements: %w", err)
	}
	byTemplate := map[string]*workflowTemplate{}
	for i := range m.Templates {
		byTemplate[m.Templates[i].ID] = &m.Templates[i]
	}
	for links.Next() {
		var tid, mid string
		if err := links.Scan(&tid, &mid); err != nil {
			links.Close()
			return m, err
		}
		if t := byTemplate[tid]; t != nil {
			t.MovementIDs = append(t.MovementIDs, mid)
		}
	}
	if err := links.Err(); err != nil {
		links.Close()
		return m, err
	}
	links.Close()
	query := "SELECT id, user_id, team_id, workflow_id, COALESCE(ip_id,''), status, COALESCE(params,''), COALESCE(result,''), COALESCE(error,''), created_at, started_at, completed_at FROM video_tasks"
	args := []any{}
	if onlyTask != "" {
		query += " WHERE id = ?"
		args = append(args, onlyTask)
	}
	query += " ORDER BY created_at, id"
	rows, err = db.QueryContext(ctx, query, args...)
	if err != nil {
		return m, fmt.Errorf("video_tasks: %w", err)
	}
	for rows.Next() {
		var t videoTask
		var params, result string
		var startedAt, completedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.UserID, &t.TeamID, &t.WorkflowID, &t.IPID, &t.Status, &params, &result, &t.Error, &t.CreatedAt, &startedAt, &completedAt); err != nil {
			rows.Close()
			return m, err
		}
		if startedAt.Valid {
			t.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		if params != "" {
			if !json.Valid([]byte(params)) {
				rows.Close()
				return m, fmt.Errorf("task %s invalid params JSON", t.ID)
			}
			t.Params = json.RawMessage(params)
		}
		if result != "" {
			if !json.Valid([]byte(result)) {
				rows.Close()
				return m, fmt.Errorf("task %s invalid result JSON", t.ID)
			}
			t.Result = json.RawMessage(result)
		}
		t.Classification = classify(t.Params)
		m.Tasks = append(m.Tasks, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return m, err
	}
	rows.Close()
	if onlyTask != "" && len(m.Tasks) == 0 {
		return m, fmt.Errorf("video_task %s not found", onlyTask)
	}
	byTask := map[string]*videoTask{}
	for i := range m.Tasks {
		byTask[m.Tasks[i].ID] = &m.Tasks[i]
	}
	query = "SELECT id, task_id, user_id, team_id, url, COALESCE(name,''), COALESCE(thumbnail,''), COALESCE(prompt,''), duration, size, COALESCE(product_id,''), COALESCE(ip_id,''), COALESCE(scene_id,''), COALESCE(pose_id,''), COALESCE(movement_id,''), COALESCE(first_frame_id,''), COALESCE(style_image_id,''), COALESCE(model_image_id,''), created_at FROM videos"
	args = nil
	if onlyTask != "" {
		query += " WHERE task_id = ?"
		args = append(args, onlyTask)
	}
	query += " ORDER BY created_at, id"
	rows, err = db.QueryContext(ctx, query, args...)
	if err != nil {
		return m, fmt.Errorf("videos: %w", err)
	}
	for rows.Next() {
		var v video
		var duration, size sql.NullInt64
		if err := rows.Scan(&v.ID, &v.TaskID, &v.UserID, &v.TeamID, &v.URL, &v.Name, &v.Thumbnail, &v.Prompt, &duration, &size, &v.ProductID, &v.IPID, &v.SceneID, &v.PoseID, &v.MovementID, &v.FirstFrameID, &v.StyleImageID, &v.ModelImageID, &v.CreatedAt); err != nil {
			rows.Close()
			return m, err
		}
		if duration.Valid {
			v.Duration = &duration.Int64
		}
		if size.Valid {
			v.Size = &size.Int64
		}
		if t := byTask[v.TaskID]; t != nil {
			t.Videos = append(t.Videos, v)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return m, err
	}
	rows.Close()
	if err := readLineage(ctx, db, &m, onlyTask); err != nil {
		return m, err
	}
	return m, nil
}

func classify(raw json.RawMessage) string {
	var params struct {
		Source string `json:"source"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &params) != nil {
		return "unknown"
	}
	if params.Source == "manual_upload" {
		return "manual_upload"
	}
	if params.Source == "" {
		return "unknown"
	}
	return params.Source
}
