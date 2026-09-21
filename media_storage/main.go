package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"media_agent/hertz_infra/config"
	"media_agent/hertz_infra/serverhertz"
	"media_agent/media_storage/biz/dal"
	"media_agent/media_storage/biz/dal/driver/localfile"
	storedobjectrepo "media_agent/media_storage/biz/dal/repo/stored_object"
	domain "media_agent/media_storage/biz/domain/stored_object"
	objecthandler "media_agent/media_storage/biz/handler/object"
	storedobjectsvc "media_agent/media_storage/biz/service/stored_object"
)

func main() {
	loader, err := config.NewLoader()
	if err != nil {
		log.Fatalf("init config loader: %v", err)
	}
	defer loader.Close()
	cfg := loader.Current()

	logCfg := cfg.GetLog()
	if logCfg.GetDir() == "" {
		logCfg.Dir = resolveDefaultLogDir(cfg.GetApp().GetName())
	}

	suite := serverhertz.NewServerSuite(loader)

	if err := loader.Attach(suite.KVSource()); err != nil {
		log.Fatalf("attach kv source: %v", err)
	}
	if err := loader.Watch(); err != nil {
		log.Fatalf("start config watch: %v", err)
	}

	shutdownObs, err := suite.InitObservability()
	if err != nil {
		log.Fatalf("init observability: %v", err)
	}
	defer shutdownObs(context.Background())

	dbCfg := cfg.GetDatabase()
	db, err := dal.NewDB(dbCfg)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	if err := dal.Migrate("migrations", dbCfg); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	storageDriver, err := newStorageDriver(cfg.GetObjectStorage())
	if err != nil {
		log.Fatalf("init object storage: %v", err)
	}
	storageRepo := storedobjectrepo.NewRepo(db)
	objecthandler.Svc = storedobjectsvc.NewService(storageDriver, storageRepo, normalizedDriverName(cfg.GetObjectStorage().GetDriver()))
	objecthandler.InternalToken = os.Getenv("MEDIA_STORAGE_INTERNAL_TOKEN")

	h := suite.NewServer()
	register(h)
	suite.RegisterRoutes(h)

	if err := suite.RegisterService(); err != nil {
		log.Fatalf("register service: %v", err)
	}
	defer func() {
		if err := suite.DeregisterService(); err != nil {
			log.Printf("deregister service: %v", err)
		}
	}()

	log.Printf("media-storage: listening on %s", cfg.GetServer().GetAddress())
	h.Spin()
}

func normalizedDriverName(value string) string {
	if value == "" {
		return "local_file"
	}
	return value
}

func newStorageDriver(cfg interface {
	GetEnabled() bool
	GetDriver() string
	GetLocalRoot() string
}) (domain.Driver, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, nil
	}
	switch cfg.GetDriver() {
	case "local_file", "":
		return localfile.New(cfg.GetLocalRoot())
	default:
		return nil, fmt.Errorf("unsupported object storage driver %q", cfg.GetDriver())
	}
}

func resolveDefaultLogDir(appName string) string {
	wd, _ := os.Getwd()
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return filepath.Join(dir, "media_logs", appName)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
