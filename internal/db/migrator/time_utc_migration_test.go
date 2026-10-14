package migrator

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type legacyTimeRow struct {
	ID         uint
	Name       string
	FinishedAt *time.Time
	CreatedAt  time.Time
}

func TestMigrateTimeColumnsToUTC_SQLite(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&legacyTimeRow{}); err != nil {
		t.Fatal(err)
	}
	// 历史数据：本地 +08:00（空格/T 两种格式）、已是 Z、NULL / legacy data in several formats
	for _, stmt := range []string{
		"INSERT INTO legacy_time_rows (id, name, created_at, finished_at) VALUES (1, 'space', '2026-10-14 13:31:03.96+08:00', '2026-10-14T14:00:00+08:00')",
		"INSERT INTO legacy_time_rows (id, name, created_at, finished_at) VALUES (2, 'utc', '2026-10-14T05:00:00.5Z', NULL)",
		"INSERT INTO legacy_time_rows (id, name, created_at, finished_at) VALUES (3, 'garbage', 'not-a-time', NULL)",
	} {
		if err := database.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	models := []interface{}{&legacyTimeRow{}}
	if err := MigrateTimeColumnsToUTC(database, "sqlite", models, ""); err != nil {
		t.Fatal(err)
	}
	read := func(id int, column string) string {
		var v *string
		database.Raw("SELECT "+column+" FROM legacy_time_rows WHERE id = ?", id).Scan(&v)
		if v == nil {
			return "<nil>"
		}
		return *v
	}
	instants := map[[2]interface{}]time.Time{
		{1, "created_at"}:  time.Date(2026, 10, 14, 5, 31, 3, 960_000_000, time.UTC),
		{1, "finished_at"}: time.Date(2026, 10, 14, 6, 0, 0, 0, time.UTC),
		{2, "created_at"}:  time.Date(2026, 10, 14, 5, 0, 0, 500_000_000, time.UTC),
	}
	for key, want := range instants {
		got := read(key[0].(int), key[1].(string))
		parsed, err := time.Parse(time.RFC3339Nano, got)
		if err != nil || !strings.HasSuffix(got, "Z") || !parsed.Equal(want) {
			t.Fatalf("row %v %v: got %q want %v", key[0], key[1], got, want)
		}
	}
	if got := read(2, "finished_at"); got != "<nil>" {
		t.Fatalf("NULL must stay NULL, got %q", got)
	}
	if got := read(3, "created_at"); got != "not-a-time" {
		t.Fatalf("unparseable value must be left untouched, got %q", got)
	}

	// 幂等：标记存在后再次执行不改写数据 / idempotent: rerun after the flag is a no-op
	database.Exec("UPDATE legacy_time_rows SET created_at = '2026-01-01 08:00:00+08:00' WHERE id = 2")
	if err := MigrateTimeColumnsToUTC(database, "sqlite", models, ""); err != nil {
		t.Fatal(err)
	}
	if got := read(2, "created_at"); !strings.HasSuffix(got, "+08:00") {
		t.Fatalf("second run must be a no-op, got %q", got)
	}
	var row legacyTimeRow
	if err := database.First(&row, 1).Error; err != nil {
		t.Fatal(err)
	}
	if !row.CreatedAt.Equal(time.Date(2026, 10, 14, 5, 31, 3, 960_000_000, time.UTC)) {
		t.Fatalf("migrated value not readable as the same instant: %v", row.CreatedAt)
	}
}

func TestFormatOffset(t *testing.T) {
	if formatOffset(8*3600) != "+08:00" || formatOffset(-(5*3600+1800)) != "-05:30" || formatOffset(0) != "+00:00" {
		t.Fatal("formatOffset")
	}
}

type legacyTimeRowB struct {
	ID        uint
	CreatedAt time.Time
}

// TestMigrateTimeColumnsToUTC_RollbackOnFailure 迁移中途失败时整体回滚且不写入标记，下次启动可重试。
// A failure mid-migration rolls everything back and leaves no flag so the next start retries.
func TestMigrateTimeColumnsToUTC_RollbackOnFailure(t *testing.T) {
	database, err := gorm.Open(sqlite.Open("file:rollback?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&legacyTimeRow{}, &legacyTimeRowB{}); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		"INSERT INTO legacy_time_rows (id, name, created_at) VALUES (1, 'a', '2026-10-14 13:31:03+08:00')",
		"INSERT INTO legacy_time_row_bs (id, created_at) VALUES (1, '2026-10-14 13:31:03+08:00')",
		"CREATE TRIGGER fail_b BEFORE UPDATE ON legacy_time_row_bs BEGIN SELECT RAISE(ABORT, 'boom'); END",
	} {
		if err := database.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	models := []interface{}{&legacyTimeRow{}, &legacyTimeRowB{}}
	if err := MigrateTimeColumnsToUTC(database, "sqlite", models, ""); err == nil {
		t.Fatal("expected migration failure")
	}
	var raw string
	database.Raw("SELECT CAST(created_at AS TEXT) FROM legacy_time_rows WHERE id = 1").Scan(&raw)
	if !strings.HasSuffix(raw, "+08:00") {
		t.Fatalf("first table must be rolled back, got %q", raw)
	}
	var flags int64
	database.Model(&SchemaFlag{}).Count(&flags)
	if flags != 0 {
		t.Fatal("flag must not be written on failure")
	}
	if err := database.Exec("DROP TRIGGER fail_b").Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateTimeColumnsToUTC(database, "sqlite", models, ""); err != nil {
		t.Fatalf("retry after fix must succeed: %v", err)
	}
	database.Raw("SELECT CAST(created_at AS TEXT) FROM legacy_time_row_bs WHERE id = 1").Scan(&raw)
	if !strings.HasSuffix(raw, "Z") {
		t.Fatalf("retry must migrate, got %q", raw)
	}
}
