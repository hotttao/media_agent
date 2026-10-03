package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildBatchMappingsRequiresUniqueTargetFacts(t *testing.T) {
	groups := []lineageGroup{{ProductID: "source-product", VirtualIPID: "source-vip"}}
	products := map[string]bool{"source-product": true}
	vips := map[string]string{"source-vip": "Alex"}
	targetProducts := []auditTarget{{ID: "target-product", Metadata: json.RawMessage(`{"migration":{"source":"media_ai","source_id":"source-product"}}`)}}
	targetVIPs := []auditTarget{{ID: "target-vip", Name: "Alex"}}
	mapping, err := buildBatchMappings(groups, products, vips, targetProducts, targetVIPs)
	if err != nil || mapping.products["source-product"] != "target-product" || mapping.vips["source-vip"] != "target-vip" {
		t.Fatalf("mapping = %+v, err = %v", mapping, err)
	}
	if _, err := buildBatchMappings(groups, products, vips, append(targetProducts, targetProducts[0]), targetVIPs); err == nil || !strings.Contains(err.Error(), "Product") {
		t.Fatalf("ambiguous Product mapping was accepted: %v", err)
	}
	if _, err := buildBatchMappings(groups, products, vips, targetProducts, append(targetVIPs, auditTarget{ID: "other-vip", Name: "Alex"})); err == nil || !strings.Contains(err.Error(), "Virtual IP") {
		t.Fatalf("ambiguous Virtual IP mapping was accepted: %v", err)
	}
}
