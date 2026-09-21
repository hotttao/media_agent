package main

import (
	"testing"

	"media_agent/hertz_infra/config"
)

type storageConfig struct {
	enabled bool
	driver  string
	root    string
}

func (c storageConfig) GetEnabled() bool     { return c.enabled }
func (c storageConfig) GetDriver() string    { return c.driver }
func (c storageConfig) GetLocalRoot() string { return c.root }

func TestNewStorageDriver(t *testing.T) {
	driver, err := newStorageDriver(storageConfig{})
	if err != nil || driver != nil {
		t.Fatalf("disabled driver = %T, %v", driver, err)
	}
	driver, err = newStorageDriver(storageConfig{enabled: true, driver: "local_file", root: t.TempDir()})
	if err != nil || driver == nil {
		t.Fatalf("local driver = %T, %v", driver, err)
	}
	if _, err := newStorageDriver(storageConfig{enabled: true, driver: "unknown"}); err == nil {
		t.Fatal("expected unsupported driver error")
	}
}

func TestDevConfigSelectsLocalFileDriver(t *testing.T) {
	loader, err := config.NewLoaderFromPath("conf/dev.yaml")
	if err != nil {
		t.Fatalf("load dev config: %v", err)
	}
	defer loader.Close()
	cfg := loader.Current().GetObjectStorage()
	if !cfg.GetEnabled() || cfg.GetDriver() != "local_file" || cfg.GetLocalRoot() == "" {
		t.Fatalf("unexpected object storage config: %+v", cfg)
	}
}
