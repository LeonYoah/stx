package logtime

import (
	"testing"
	"time"
)

func TestParseReturnsUTC(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	once.Do(func() {})
	loc = shanghai
	got, ok := Parse("2026-10-14 13:31:03,960", "2006-01-02 15:04:05,000")
	if !ok || got.Location() != time.UTC || !got.Equal(time.Date(2026, 10, 14, 5, 31, 3, 960_000_000, time.UTC)) {
		t.Fatalf("got %v %v", got, ok)
	}
	if _, ok := Parse("nope", "2006-01-02"); ok {
		t.Fatal("expected failure")
	}
}
