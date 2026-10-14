package monitoring

import (
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

func TestFormatAlertNotificationTimeFollowsDisplayTimezone(t *testing.T) {
	defer timex.SetDisplayTimezone(timex.DefaultDisplayTimezone)
	at := time.Date(2026, 10, 14, 7, 30, 0, 0, time.UTC)
	for tz, want := range map[string]string{
		"Asia/Shanghai":    "2026-10-14 15:30:00 +08:00 (Asia/Shanghai)",
		"America/New_York": "2026-10-14 03:30:00 -04:00 (America/New_York)",
		"UTC":              "2026-10-14 07:30:00 +00:00 (UTC)",
	} {
		if err := timex.SetDisplayTimezone(tz); err != nil {
			t.Fatal(err)
		}
		got := formatAlertNotificationTime(at.In(time.Local))
		if !strings.Contains(got, want) || !strings.Contains(got, "2026-10-14T07:30:00Z (UTC)") {
			t.Fatalf("%s: got %q", tz, got)
		}
	}
}
