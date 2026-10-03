package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"
)

// batchMappings are derived from already-migrated target library records.
// Ambiguous names or provenance never silently choose a destination.
type batchMappings struct {
	products map[string]string
	vips     map[string]string
}

func buildBatchMappings(required []lineageGroup, sourceProducts map[string]bool, sourceVIPs map[string]string, targetsProducts, targetsVIPs []auditTarget) (batchMappings, error) {
	productIDs := map[string][]string{}
	for _, item := range targetsProducts {
		if sourceID := auditSourceID(item.Metadata); sourceID != "" {
			productIDs[sourceID] = append(productIDs[sourceID], item.ID)
		}
	}
	vipNames := map[string][]string{}
	for _, item := range targetsVIPs {
		vipNames[item.Name] = append(vipNames[item.Name], item.ID)
	}
	mapped := batchMappings{products: map[string]string{}, vips: map[string]string{}}
	for _, group := range required {
		if group.ProductID == "" || !sourceProducts[group.ProductID] || len(productIDs[group.ProductID]) != 1 {
			return batchMappings{}, fmt.Errorf("source Product %q has no unique target mapping", group.ProductID)
		}
		name := sourceVIPs[group.VirtualIPID]
		if name == "" || len(vipNames[name]) != 1 {
			return batchMappings{}, fmt.Errorf("source Virtual IP %q has no unique target mapping", group.VirtualIPID)
		}
		mapped.products[group.ProductID] = productIDs[group.ProductID][0]
		mapped.vips[group.VirtualIPID] = vipNames[name][0]
	}
	return mapped, nil
}

func readBatchMappings(ctx context.Context, db *sql.DB, m manifest, cfg importConfig) (batchMappings, error) {
	products, err := sourceAuditIDs(ctx, db, "SELECT id FROM products")
	if err != nil {
		return batchMappings{}, err
	}
	vips, err := sourceAuditPairs(ctx, db, "SELECT id, nickname FROM virtual_ips")
	if err != nil {
		return batchMappings{}, err
	}
	targetProducts, err := listAuditTargets(ctx, cfg, "products")
	if err != nil {
		return batchMappings{}, err
	}
	targetVIPs, err := listAuditTargets(ctx, cfg, "virtual-ips")
	if err != nil {
		return batchMappings{}, err
	}
	return buildBatchMappings(m.Groups, products, vips, targetProducts, targetVIPs)
}

type batchResult struct {
	Kind       string `json:"kind"`
	SourceID   string `json:"source_id"`
	TargetID   string `json:"target_id,omitempty"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	FinishedAt string `json:"finished_at"`
}

func importAllHistory(ctx context.Context, db *sql.DB, m *manifest, cfg importConfig, reportPath string) error {
	if reportPath == "" {
		return errors.New("--import-all requires --report: a new private JSONL reconciliation file")
	}
	if cfg.ProductID != "" || cfg.VirtualIPID != "" {
		return errors.New("--import-all derives Product and Virtual IP mappings; do not pass single-item IDs")
	}
	if cfg.OrganizationID == "" || cfg.OwnerUserID == "" || cfg.WorkspaceID == "" || cfg.WorkflowID == "" || cfg.WorkflowVersion <= 0 || cfg.BearerToken == "" || cfg.MigrationToken == "" || cfg.WorkflowToken == "" {
		return errors.New("--import-all requires target organization, owner, Workspace, published Workflow, User API Token and both migration tokens")
	}
	for label, origin := range map[string]string{"source-assets-url": cfg.SourceAssetsURL, "target-gateway": cfg.GatewayURL, "materials-internal-url": cfg.MaterialsURL, "workflow-internal-url": cfg.WorkflowURL} {
		if err := validateOrigin(label, origin); err != nil {
			return err
		}
	}
	mappings, err := readBatchMappings(ctx, db, *m, cfg)
	if err != nil {
		return fmt.Errorf("batch mapping preflight: %w", err)
	}
	report, err := os.OpenFile(reportPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create private report: %w", err)
	}
	defer report.Close()
	record := func(kind, sourceID, targetID string, itemErr error) error {
		item := batchResult{Kind: kind, SourceID: sourceID, TargetID: targetID, Status: "imported", FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if itemErr != nil {
			item.Status, item.Error = "failed", itemErr.Error()
		}
		encoded, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := report.Write(append(encoded, '\n')); err != nil {
			return err
		}
		return report.Sync()
	}
	for _, excluded := range m.Excluded {
		item := batchResult{Kind: excluded.Kind, SourceID: excluded.SourceID, Status: "excluded", Error: excluded.Reason, FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		encoded, err := json.Marshal(item)
		if err != nil {
			return err
		}
		if _, err := report.Write(append(encoded, '\n')); err != nil {
			return err
		}
	}
	if err := report.Sync(); err != nil {
		return err
	}
	groups := append([]lineageGroup(nil), m.Groups...)
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	groupByPush := map[string]lineageGroup{}
	videoGroups := map[string]lineageGroup{}
	for _, group := range groups {
		if group.Kind == "video_push" {
			groupByPush[group.VideoPushID] = group
		}
		for _, videoID := range group.VideoIDs {
			if _, exists := videoGroups[videoID]; !exists {
				videoGroups[videoID] = group
			}
		}
	}
	failed := 0
	incompleteGroups := 0
	for _, group := range groups {
		if group.ExecutionState != "awaiting_resume" {
			continue
		}
		incompleteGroups++
		itemCfg := cfg
		itemCfg.ProductID, itemCfg.VirtualIPID, itemCfg.ImportJob = mappings.products[group.ProductID], mappings.vips[group.VirtualIPID], true
		jobID, err := importImageGroupID(ctx, db, m, group.ID, itemCfg)
		if writeErr := record("incomplete_group", group.ID, jobID, err); writeErr != nil {
			return writeErr
		}
		if err != nil {
			failed++
		}
	}
	videoIDs := make([]string, 0, len(videoGroups))
	for id := range videoGroups {
		videoIDs = append(videoIDs, id)
	}
	sort.Strings(videoIDs)
	jobIDs := map[string]string{}
	for _, videoID := range videoIDs {
		group := videoGroups[videoID]
		itemCfg := cfg
		itemCfg.ProductID, itemCfg.VirtualIPID = mappings.products[group.ProductID], mappings.vips[group.VirtualIPID]
		jobID, err := importCompletedVideoID(ctx, db, m, videoID, itemCfg)
		if writeErr := record("video", videoID, jobID, err); writeErr != nil {
			return writeErr
		}
		if err != nil {
			failed++
		} else {
			jobIDs[videoID] = jobID
		}
	}
	pushIDs := make([]string, 0, len(groupByPush))
	for id := range groupByPush {
		pushIDs = append(pushIDs, id)
	}
	sort.Strings(pushIDs)
	for _, pushID := range pushIDs {
		group := groupByPush[pushID]
		itemCfg := cfg
		itemCfg.ProductID, itemCfg.VirtualIPID = mappings.products[group.ProductID], mappings.vips[group.VirtualIPID]
		// Do not re-upload a failed member while recording the publication.
		var err error
		for _, memberID := range group.VideoIDs {
			if jobIDs[memberID] == "" {
				err = fmt.Errorf("member Video %s was not imported", memberID)
				break
			}
		}
		var targetID string
		if err == nil {
			targetID, err = importHistoricalVideoPushWithJobs(ctx, db, m, pushID, itemCfg, jobIDs)
		}
		if writeErr := record("video_push", pushID, targetID, err); writeErr != nil {
			return writeErr
		}
		if err != nil {
			failed++
		}
	}
	fmt.Printf("batch reconciliation: report=%s incomplete_groups=%d videos=%d video_pushes=%d excluded=%d failures=%d\n", reportPath, incompleteGroups, len(videoIDs), len(pushIDs), len(m.Excluded), failed)
	if failed != 0 {
		return fmt.Errorf("batch import has %d failures; inspect private report and rerun idempotently with a new --report path", failed)
	}
	return nil
}
