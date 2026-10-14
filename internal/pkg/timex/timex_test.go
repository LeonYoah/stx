package timex

import (
	"testing"
	"time"
)

func TestParseAPITime(t *testing.T) {
	got, err := ParseAPITime("2026-10-14T13:31:03+08:00", "")
	if err != nil || got.Format(time.RFC3339) != "2026-10-14T05:31:03Z" {
		t.Fatalf("offset parse: %v %v", got, err)
	}
	if _, err := ParseAPITime("2026-10-14 13:31:03", ""); err == nil {
		t.Fatal("naive time without tz must be rejected")
	}
	got, err = ParseAPITime("2026-10-14 13:31:03", "Asia/Shanghai")
	if err != nil || got.Format(time.RFC3339) != "2026-10-14T05:31:03Z" {
		t.Fatalf("naive+tz parse: %v %v", got, err)
	}
	if _, err := ParseAPITime("2026-10-14", "Bad/Zone"); err == nil {
		t.Fatal("invalid tz must be rejected")
	}
}

func TestDisplayTimezone(t *testing.T) {
	t.Setenv("STX_DISPLAY_TIMEZONE", "")
	t.Setenv("STX_REPORT_TIMEZONE", "UTC")
	displayLoc = nil
	if DisplayTimezone() != "UTC" {
		t.Fatalf("legacy env alias not honored: %s", DisplayTimezone())
	}
	if err := SetDisplayTimezone("Nope/Nope"); err == nil {
		t.Fatal("expected error")
	}
	if err := SetDisplayTimezone(""); err != nil || DisplayTimezone() != DefaultDisplayTimezone {
		t.Fatalf("default not applied: %v %s", err, DisplayTimezone())
	}
	if FormatAPI(time.Time{}) != "" || FormatAPI(time.Date(2026, 1, 1, 8, 0, 0, 0, DisplayLocation())) != "2026-01-01T00:00:00Z" {
		t.Fatal("FormatAPI")
	}
}

func TestInvalidEnvDisplayTimezoneFallsBack(t *testing.T) {
	t.Setenv("STX_DISPLAY_TIMEZONE", "Not/AZone")
	displayMu.Lock()
	displayLoc, displayName = nil, ""
	displayMu.Unlock()
	if got := DisplayTimezone(); got != DefaultDisplayTimezone {
		t.Fatalf("invalid env must fall back to default, got %s", got)
	}
	t.Setenv("STX_DISPLAY_TIMEZONE", "Europe/Berlin")
	t.Setenv("STX_REPORT_TIMEZONE", "UTC")
	if EnvDisplayTimezone() != "Europe/Berlin" {
		t.Fatal("STX_DISPLAY_TIMEZONE must win over the legacy alias")
	}
}
