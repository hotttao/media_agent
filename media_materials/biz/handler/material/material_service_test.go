package material

import (
	"reflect"
	"testing"
)

func TestParseAssetKinds(t *testing.T) {
	want := []string{"scene", "movement", "pose"}
	got := parseAssetKinds(" scene,movement,scene, ,pose ")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseAssetKinds() = %#v, want %#v", got, want)
	}
	if got := parseAssetKinds(""); len(got) != 0 {
		t.Fatalf("empty filter = %#v, want no kinds", got)
	}
}
