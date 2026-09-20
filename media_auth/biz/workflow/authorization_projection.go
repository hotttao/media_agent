package workflow

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	domain "media_agent/media_auth/biz/domain/authorizationprojection"
)

type AuthorizationProjection struct {
	outbox    domain.Outbox
	snapshots domain.SnapshotSource
	tuples    domain.TupleStore
	interval  time.Duration
}

func NewAuthorizationProjection(outbox domain.Outbox, snapshots domain.SnapshotSource, tuples domain.TupleStore) *AuthorizationProjection {
	return &AuthorizationProjection{outbox: outbox, snapshots: snapshots, tuples: tuples, interval: time.Second}
}

func (w *AuthorizationProjection) ProcessPending(ctx context.Context) error {
	events, err := w.outbox.ClaimPending(ctx, 20)
	if err != nil {
		return err
	}
	for _, event := range events {
		if err := w.project(ctx, event); err != nil {
			delay := time.Second << min(event.Attempts, 8)
			if markErr := w.outbox.MarkFailed(ctx, event, err.Error(), time.Now().Add(delay)); markErr != nil {
				return fmt.Errorf("projection event=%s aggregate=%s sequence=%d: %v; mark failed: %w", event.ID, event.AggregateID, event.Sequence, err, markErr)
			}
			log.Printf("authorization projection failed event_id=%s aggregate_id=%s sequence=%d error=%v", event.ID, event.AggregateID, event.Sequence, err)
			continue
		}
		if err := w.outbox.MarkDelivered(ctx, event); err != nil {
			return fmt.Errorf("mark projection delivered event=%s: %w", event.ID, err)
		}
	}
	return nil
}

func (w *AuthorizationProjection) project(ctx context.Context, event domain.Event) error {
	switch event.AggregateType {
	case "organization":
		snapshot, err := w.snapshots.Organization(ctx, event.AggregateID)
		if err != nil {
			return err
		}
		current, err := w.tuples.List(ctx, "Organization", snapshot.ID)
		if err != nil {
			return err
		}
		return reconcile(ctx, w.tuples, current, organizationTuples(snapshot))
	case "team":
		snapshot, err := w.snapshots.Team(ctx, event.AggregateID)
		if err != nil {
			return err
		}
		current, err := w.tuples.List(ctx, "Team", snapshot.ID)
		if err != nil {
			return err
		}
		return reconcile(ctx, w.tuples, current, teamTuples(snapshot))
	default:
		return fmt.Errorf("unsupported aggregate type %q", event.AggregateType)
	}
}

func (w *AuthorizationProjection) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if err := w.ProcessPending(ctx); err != nil {
			log.Printf("authorization projection poll failed: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func organizationTuples(snapshot domain.OrganizationSnapshot) []domain.Tuple {
	if snapshot.Status != "active" {
		return nil
	}
	var out []domain.Tuple
	for _, member := range snapshot.Members {
		relation := "members"
		if member.Role == "admin" {
			relation = "admins"
		}
		out = append(out, domain.Tuple{Namespace: "Organization", Object: snapshot.ID, Relation: relation, SubjectID: member.UserID})
	}
	features := map[string]bool{}
	for _, key := range snapshot.Entitlements {
		switch {
		case key == "materials" || strings.HasPrefix(key, "materials."):
			features["materials_entitled"] = true
		case key == "workflow" || strings.HasPrefix(key, "workflow."):
			features["workflow_entitled"] = true
		}
	}
	for relation := range features {
		for _, subjectRelation := range []string{"members", "admins"} {
			out = append(out, domain.Tuple{Namespace: "Organization", Object: snapshot.ID, Relation: relation, SubjectSet: &domain.SubjectSet{Namespace: "Organization", Object: snapshot.ID, Relation: subjectRelation}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return tupleKey(out[i]) < tupleKey(out[j]) })
	return out
}

func teamTuples(snapshot domain.TeamSnapshot) []domain.Tuple {
	if snapshot.Status != "active" {
		return nil
	}
	out := []domain.Tuple{{Namespace: "Team", Object: snapshot.ID, Relation: "organization", SubjectID: snapshot.OrganizationID}}
	for _, member := range snapshot.Members {
		relation := "members"
		if member.Role == "admin" {
			relation = "admins"
		}
		out = append(out, domain.Tuple{Namespace: "Team", Object: snapshot.ID, Relation: relation, SubjectID: member.UserID})
	}
	effective := snapshot.OrganizationEntitlements
	if snapshot.EntitlementMode == "restricted" {
		allowed := make(map[string]struct{}, len(snapshot.TeamEntitlements))
		for _, key := range snapshot.TeamEntitlements {
			allowed[key] = struct{}{}
		}
		effective = nil
		for _, key := range snapshot.OrganizationEntitlements {
			if _, ok := allowed[key]; ok {
				effective = append(effective, key)
			}
		}
	}
	features := map[string]bool{}
	for _, key := range effective {
		switch {
		case key == "materials" || strings.HasPrefix(key, "materials."):
			features["materials_users"] = true
		case key == "workflow" || strings.HasPrefix(key, "workflow."):
			features["workflow_users"] = true
		case key == "workspace" || strings.HasPrefix(key, "workspace."):
			features["workspace_creators"] = true
		}
	}
	for relation := range features {
		for _, subjectRelation := range []string{"members", "admins"} {
			out = append(out, domain.Tuple{Namespace: "Team", Object: snapshot.ID, Relation: relation, SubjectSet: &domain.SubjectSet{Namespace: "Team", Object: snapshot.ID, Relation: subjectRelation}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return tupleKey(out[i]) < tupleKey(out[j]) })
	return out
}

func reconcile(ctx context.Context, store domain.TupleStore, current, desired []domain.Tuple) error {
	have := make(map[string]domain.Tuple, len(current))
	want := make(map[string]domain.Tuple, len(desired))
	for _, tuple := range current {
		have[tupleKey(tuple)] = tuple
	}
	for _, tuple := range desired {
		want[tupleKey(tuple)] = tuple
		if _, ok := have[tupleKey(tuple)]; !ok {
			if err := store.Put(ctx, tuple); err != nil {
				return err
			}
		}
	}
	for key, tuple := range have {
		if _, ok := want[key]; !ok {
			if err := store.Delete(ctx, tuple); err != nil {
				return err
			}
		}
	}
	return nil
}

func tupleKey(tuple domain.Tuple) string {
	subject := tuple.SubjectID
	if tuple.SubjectSet != nil {
		subject = tuple.SubjectSet.Namespace + ":" + tuple.SubjectSet.Object + "#" + tuple.SubjectSet.Relation
	}
	return tuple.Namespace + ":" + tuple.Object + "#" + tuple.Relation + "@" + subject
}
