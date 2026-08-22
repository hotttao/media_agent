package platform_administration

import (
	"testing"
	"time"
)

func TestStatisticCanonicalDimensionsAndDecimal(t *testing.T) {
	period, refreshed := time.Unix(100, 0).UTC(), time.Unix(200, 0).UTC()
	first, err := ValidateStatistic("workspace.active", `{"region":"cn","tier":"gpu"}`, "10.50", period, refreshed)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ValidateStatistic("workspace.active", `{"tier":"gpu","region":"cn"}`, "10.50", period, refreshed)
	if err != nil || first.DimensionsHash != second.DimensionsHash || first.DimensionsJSON != second.DimensionsJSON {
		t.Fatalf("dimensions were not canonical: first=%+v second=%+v err=%v", first, second, err)
	}
	for _, value := range []string{"1e3", "01", "1.1234567890123456789", "not-number"} {
		if _, err := ValidateStatistic("workspace.active", `{}`, value, period, refreshed); err == nil {
			t.Fatalf("accepted invalid decimal %q", value)
		}
	}
}
