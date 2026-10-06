//go:build e2e

package e2e

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"media_agent/media_materials/biz/dal"
	repo "media_agent/media_materials/biz/dal/repo/generated_asset"
	domain "media_agent/media_materials/biz/domain/generated_asset"
	service "media_agent/media_materials/biz/service/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
)

type noReplayAssetWrite struct{ domain.Repo }

func (r noReplayAssetWrite) Save(context.Context, *domain.GeneratedAsset) error {
	os.Exit(88)
	return errors.New("forbidden replay write")
}

type noReplayStorage struct{}

func (noReplayStorage) Stat(context.Context, string) (objectstorage.ObjectInfo, error) {
	os.Exit(88)
	return objectstorage.ObjectInfo{}, errors.New("forbidden replay storage access")
}
func (noReplayStorage) PresignGet(context.Context, string) (objectstorage.ResolvedObject, error) {
	os.Exit(88)
	return objectstorage.ResolvedObject{}, errors.New("forbidden replay storage access")
}

func TestGenerationRegistrationProcessHelper(t *testing.T) {
	mode := os.Getenv("MEDIA_E2E_RESULT_CHILD_MODE")
	if mode == "" {
		t.Skip("isolated subprocess helper")
	}
	db, err := gorm.Open(pgdriver.Open(os.Getenv("MEDIA_E2E_RESULT_CHILD_DSN")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("isolated child database unavailable")
	}
	r := repo.NewRepo(db)
	var assetRepo domain.Repo = r
	if mode == "replay" {
		assetRepo = noReplayAssetWrite{Repo: r}
	}
	s := service.NewService(assetRepo, registrationProduct{}, nil)
	s.ConfigureResultReceipts(r, dal.NewTxRunner(db))
	if mode == "replay" {
		s.ConfigureHistoryImport(noReplayStorage{}, registrationKinds{})
	} else {
		s.ConfigureHistoryImport(&registrationStorage{}, registrationKinds{})
	}
	s.ConfigureNativeOutputStorage("https://storage.example", "media")
	cmd := service.UpsertCommand{Scope: "ai_clothing", AssetKind: "first_frame", Name: "frame", OwnerUserID: "user",
		URL:   "https://storage.example/media/materials/org/ai_clothing/material/frame.png?signature=temporary",
		JobID: "job", StepRunID: "step", ResultKey: strings.Repeat("a", 64), TemplateID: "tpl", TemplateVersion: 1, SourceProductID: "product"}
	if mode == "replay" {
		cmd.URL = strings.Replace(cmd.URL, "signature=temporary", "signature=expired", 1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a, err := s.Upsert(ctx, cmd)
	if err != nil || a == nil || a.ID == "" {
		t.Fatal("child registration failed")
	}
	if mode == "commit-exit" {
		// Transaction has committed, but no resource identity is delivered to the
		// parent caller. os.Exit bypasses cleanup just like a killed process.
		os.Exit(77)
	}
	if mode != "replay" || a.ID != os.Getenv("MEDIA_E2E_RESULT_CHILD_EXPECTED_ID") {
		t.Fatal("new process did not return original identity")
	}
}

// Actual child process exits after committed acceptance. A separate child must
// replay using only PostgreSQL and must not save an Asset or consult Storage.
// Sources/Storage are controlled ports; this does not prove real TCP recovery.
func TestGenerationRegistrationRealProcessRestart(t *testing.T) {
	db := resultDatabase(t)
	dsn := db.Dialector.(*pgdriver.Dialector).DSN
	binary, err := os.Executable()
	if err != nil {
		t.Fatal("test executable unavailable")
	}
	run := func(mode, expected string, code int) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestGenerationRegistrationProcessHelper$", "-test.timeout=15s")
		cmd.Env = append(os.Environ(), "MEDIA_E2E_RESULT_CHILD_MODE="+mode, "MEDIA_E2E_RESULT_CHILD_DSN="+dsn, "MEDIA_E2E_RESULT_CHILD_EXPECTED_ID="+expected)
		// Do not include child logs, its DSN or resource payload in failure output.
		err := cmd.Run()
		if ctx.Err() != nil {
			t.Fatal("child observation timed out")
		}
		if code == 0 {
			if err != nil {
				t.Fatal("replay child failed or attempted a forbidden side effect")
			}
			return
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != code {
			t.Fatal("child did not exit at committed boundary")
		}
	}
	run("commit-exit", "", 77)
	var accepted string
	if db.Table("media_materials.generated_asset_results").Select("asset_id").Scan(&accepted).Error != nil || accepted == "" {
		t.Fatal("crashed child left no committed receipt")
	}
	run("replay", accepted, 0)
	for _, table := range []string{"generated_assets", "generated_asset_results"} {
		var count int64
		if db.Table("media_materials."+table).Count(&count).Error != nil || count != 1 {
			t.Fatal("restart duplicated or lost accepted resource")
		}
	}
}
