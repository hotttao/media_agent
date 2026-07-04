package asset_kind

import (
	"testing"

	domain "media_agent/media_materials/biz/domain/asset_kind"
)

func TestParseTypes(t *testing.T) {
	cases := []struct {
		rt, mt  string
		wantRT  domain.ResourceType
		wantMT  domain.MediaType
		wantErr bool
	}{
		{"MATERIAL", "IMAGE", domain.ResourceTypeMaterial, domain.MediaTypeImage, false},
		{"ASSET", "VIDEO", domain.ResourceTypeAsset, domain.MediaTypeVideo, false},
		{"TEXT", "TEXT", domain.ResourceTypeText, domain.MediaTypeText, false},
		{"BOGUS", "IMAGE", "", "", true},
		{"MATERIAL", "BOGUS", "", "", true},
	}
	for _, tc := range cases {
		rt, mt, err := parseTypes(tc.rt, tc.mt)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTypes(%q,%q) want err, got nil", tc.rt, tc.mt)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTypes(%q,%q) unexpected err: %v", tc.rt, tc.mt, err)
			continue
		}
		if rt != tc.wantRT || mt != tc.wantMT {
			t.Errorf("parseTypes(%q,%q) = (%v,%v), want (%v,%v)", tc.rt, tc.mt, rt, mt, tc.wantRT, tc.wantMT)
		}
	}
}

func TestSourceTableFor(t *testing.T) {
	cases := []struct {
		rt   domain.ResourceType
		want string
	}{
		{domain.ResourceTypeMaterial, "materials"},
		{domain.ResourceTypeAsset, "generated_assets"},
		{domain.ResourceTypeProduct, "products"},
		{domain.ResourceTypeVirtualIP, "virtual_ips"},
		{domain.ResourceTypeText, ""},
	}
	for _, tc := range cases {
		if got := sourceTableFor(tc.rt); got != tc.want {
			t.Errorf("sourceTableFor(%v) = %q, want %q", tc.rt, got, tc.want)
		}
	}
}

func TestValidStatus(t *testing.T) {
	if !validStatus(domain.StatusSystem) || !validStatus(domain.StatusEnabled) || !validStatus(domain.StatusDisabled) {
		t.Error("SYSTEM/ENABLED/DISABLED should be valid")
	}
	if validStatus(domain.Status("BOGUS")) {
		t.Error("BOGUS should be invalid")
	}
}
