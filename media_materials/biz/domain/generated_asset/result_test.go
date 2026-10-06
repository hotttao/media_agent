package generated_asset

import (
	"strings"
	"testing"
	"time"
)

func TestResultDigestUsesDurableObjectAndImmutableProvenance(t *testing.T) {
	a := GeneratedAsset{Origin: "native", OrganizationID: "org", Scope: "scope", AssetKind: "image", Name: "step-result", OwnerUserID: "owner", JobID: "job", StepRunID: "step", TemplateID: "template", TemplateVersion: 1, ObjectPath: "materials/org/output.png", URL: "https://storage/output?signature=old", Visibility: VisibilityPrivate}
	first, err := ResultDigest(&a)
	if err != nil {
		t.Fatal(err)
	}
	b := a
	b.ID = "another-proposed-id"
	b.URL = "https://storage/output?signature=new"
	b.CreatedAt = time.Now()
	b.UpdatedAt = time.Now()
	if digest, err := ResultDigest(&b); err != nil || digest != first {
		t.Fatal("transport or timestamps changed result")
	}
	for _, field := range []string{"object", "content", "owner", "organization", "template", "source", "name"} {
		t.Run(field, func(t *testing.T) {
			changed := a
			switch field {
			case "object":
				changed.ObjectPath = "materials/org/new.png"
			case "content":
				changed.Content = "new text"
			case "owner":
				changed.OwnerUserID = "another-owner"
			case "organization":
				changed.OrganizationID = "other-org"
			case "template":
				changed.TemplateVersion++
			case "source":
				changed.SourceProductID = "another-product"
			case "name":
				changed.Name = "another-result"
			}
			digest, err := ResultDigest(&changed)
			if err != nil || digest == first {
				t.Fatal("changed result reused digest")
			}
		})
	}
	b = a
	b.ObjectPath = ""
	if _, err := ResultDigest(&b); err == nil {
		t.Fatal("unfrozen transport-only result accepted")
	}
	b.URL = ""
	b.Content = "text result"
	if _, err := ResultDigest(&b); err != nil {
		t.Fatal("text-only result rejected")
	}
}

func TestResultReceiptRejectsMissingOrInvalidIdentity(t *testing.T) {
	r := ResultReceipt{StepRunID: "step", AssetID: "asset", ResultKey: strings.Repeat("a", 64), RequestDigest: strings.Repeat("b", 64)}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"step", "asset", "key", "digest"} {
		changed := r
		switch field {
		case "step":
			changed.StepRunID = ""
		case "asset":
			changed.AssetID = ""
		case "key":
			changed.ResultKey = strings.Repeat("A", 64)
		case "digest":
			changed.RequestDigest = "not a digest"
		}
		if changed.Validate() == nil {
			t.Fatal("invalid receipt accepted")
		}
	}
}
