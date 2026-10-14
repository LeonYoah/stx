package diagnostics

import (
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

func TestDiagnosticReportTimeUsesDisplayTimezone(t *testing.T) {
	defer timex.SetDisplayTimezone(timex.DefaultDisplayTimezone)
	at := time.Date(2026, 10, 14, 7, 30, 0, 0, time.UTC)
	if err := timex.SetDisplayTimezone("Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	if got := formatDiagnosticBundleTimeValue(at); got != "2026-10-14 15:30:00 CST" {
		t.Fatalf("got %q", got)
	}
	if err := timex.SetDisplayTimezone("UTC"); err != nil {
		t.Fatal(err)
	}
	if got := formatDiagnosticBundleTimeValue(at); got != "2026-10-14 07:30:00 UTC" {
		t.Fatalf("got %q", got)
	}
}
