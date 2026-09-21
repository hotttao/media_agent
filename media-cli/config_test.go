package main

import (
	"testing"

	"media_agent/hertz_infra/config"
)

func TestGeneratedMediaStorageClientConfig(t *testing.T) {
	for _, path := range []string{"conf/dev.yaml", "conf/local.yaml"} {
		t.Run(path, func(t *testing.T) {
			loader, err := config.NewLoaderFromPath(path)
			if err != nil {
				t.Fatalf("load config: %v", err)
			}
			defer loader.Close()
			client, ok := loader.Current().GetClient()["media_storage"]
			if !ok || !client.GetEnabled() || client.GetServiceName() != "media-storage" {
				t.Fatalf("unexpected media_storage client: %+v", client)
			}
		})
	}
}
