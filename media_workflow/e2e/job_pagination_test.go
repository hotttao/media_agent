//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"gorm.io/datatypes"

	"media_agent/media_workflow/biz/dal/model"
	repo "media_agent/media_workflow/biz/dal/repo/job"
	domain "media_agent/media_workflow/biz/domain/job"
)

func TestJobPaginationPreservesEqualTimeRecords(t *testing.T) {
	db := newDatabase(t)
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	row := func(id, owner string, submitted time.Time) model.Job {
		return model.Job{
			ID: id, Scope: "pagination", WorkflowID: "pagination-workflow", WorkflowVersion: 1,
			OrganizationID: "pagination-org", WorkspaceID: "pagination-workspace",
			Input: datatypes.JSON(`{}`), RetryHistory: datatypes.JSON(`[]`),
			Status: string(domain.JobSucceeded), SubmittedBy: owner, SubmittedAt: submitted,
		}
	}
	rows := []model.Job{row("a-newest", "pagination-owner", at.Add(time.Hour))}
	expected := []string{"a-newest"}
	for i := 0; i < 130; i++ {
		rows = append(rows, row(fmt.Sprintf("tie-%03d", i), "pagination-owner", at))
	}
	for i := 129; i >= 0; i-- {
		expected = append(expected, fmt.Sprintf("tie-%03d", i))
	}
	rows = append(rows, row("z-oldest", "pagination-owner", at.Add(-time.Hour)),
		row("other-owner", "another-owner", at.Add(2*time.Hour)))
	expected = append(expected, "z-oldest")
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal("isolated pagination fixture failed")
	}
	jobs := repo.NewJobRepo(db)
	for _, size := range []int{7, 23, 100} {
		t.Run(fmt.Sprintf("page-size-%d", size), func(t *testing.T) {
			seen := make(map[string]bool)
			for offset := 0; offset < len(expected); offset += size {
				items, total, err := jobs.List(context.Background(), domain.ListQuery{
					Scope: "pagination", SubmittedBy: "pagination-owner", Limit: size, Offset: offset,
				})
				if err != nil || total != len(expected) {
					t.Fatalf("unexpected visible total: total=%d err=%v", total, err)
				}
				want := min(size, len(expected)-offset)
				if len(items) != want {
					t.Fatalf("page at %d: got %d records, want %d", offset, len(items), want)
				}
				for i, item := range items {
					if item.ID != expected[offset+i] || seen[item.ID] {
						t.Fatalf("unstable page at %d: got %q, want %q", offset+i, item.ID, expected[offset+i])
					}
					seen[item.ID] = true
				}
			}
			if len(seen) != len(expected) {
				t.Fatal("pagination omitted visible records")
			}
		})
	}
}
