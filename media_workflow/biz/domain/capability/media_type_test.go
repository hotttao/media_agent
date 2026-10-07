package capability

import "testing"

func TestProviderMediaTypeDoesNotAcceptBusinessAssetKinds(t *testing.T) {
	for _, value := range []MediaType{MediaImage, MediaVideo, MediaText} {
		if !value.Valid() {
			t.Fatal("technical media type rejected")
		}
	}
	for _, value := range []MediaType{"", "model_image", "style_image", "first_frame", "image", "UNKNOWN"} {
		if value.Valid() {
			t.Fatal("business kind or unknown value accepted as technical type")
		}
	}
}

func TestMediaConstraintOnlyAppliesToResourceInputs(t *testing.T) {
	for _, inputType := range []InputType{InputResourceRef, InputResourceRefList} {
		if !(Input{Type: inputType, MediaType: MediaImage}).ValidMediaConstraint() {
			t.Fatal("resource media constraint rejected")
		}
	}
	if !(Input{Type: InputString}).ValidMediaConstraint() {
		t.Fatal("legacy unconstrained input rejected")
	}
	if (Input{Type: InputString, MediaType: MediaImage}).ValidMediaConstraint() || (Input{Type: InputResourceRef, MediaType: "model_image"}).ValidMediaConstraint() {
		t.Fatal("invalid media constraint accepted")
	}
}
