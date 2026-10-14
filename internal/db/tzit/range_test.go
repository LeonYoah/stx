// Package tzit 是跨数据库驱动的时间区间集成测试：默认跑 SQLite；设置 STX_TEST_MYSQL_PORT 等变量时额外跑 MySQL。
// Package tzit holds cross-driver time-range integration tests: SQLite always, MySQL when STX_TEST_MYSQL_PORT is set.
package tzit

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	configapp "github.com/LeonYoah/stx/internal/apps/config"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"gorm.io/gorm"
)

func openTargets(t *testing.T) map[string]*gorm.DB {
	t.Helper()
	targets := map[string]*gorm.DB{}
	sqliteDB, err := db.OpenDB(config.DatabaseConfig{Type: "sqlite", SQLitePath: filepath.Join(t.TempDir(), "tz.db")})
	if err != nil {
		t.Fatal(err)
	}
	targets["sqlite"] = sqliteDB
	if port := os.Getenv("STX_TEST_MYSQL_PORT"); port != "" {
		p, _ := strconv.Atoi(port)
		mysqlDB, err := db.OpenDB(config.DatabaseConfig{
			Type: "mysql", Host: envOr("STX_TEST_MYSQL_HOST", "127.0.0.1"), Port: p,
			Username: envOr("STX_TEST_MYSQL_USER", "root"), Password: os.Getenv("STX_TEST_MYSQL_PASSWORD"),
			Database: envOr("STX_TEST_MYSQL_DATABASE", "stx_tz_it"),
		})
		if err != nil {
			t.Fatal(err)
		}
		targets["mysql"] = mysqlDB
	}
	return targets
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestScheduledWindowAndConfigRangeAcrossDrivers(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	for name, database := range openTargets(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			if err := database.Migrator().DropTable(&syncapp.JobInstance{}, &configapp.Config{}); err != nil {
				t.Fatal(err)
			}
			if err := database.AutoMigrate(&syncapp.JobInstance{}, &configapp.Config{}); err != nil {
				t.Fatal(err)
			}
			now := time.Now()
			job := &syncapp.JobInstance{TaskID: 7, TaskVersion: 1, RunType: syncapp.RunTypeSchedule, CreatedAt: now.In(shanghai)}
			if err := database.Create(job).Error; err != nil {
				t.Fatal(err)
			}
			repo := syncapp.NewRepository(database)
			windowStart := now.Truncate(time.Minute)
			for label, bounds := range map[string][2]time.Time{
				"utc":      {windowStart.UTC(), windowStart.Add(time.Minute).UTC()},
				"shanghai": {windowStart.In(shanghai), windowStart.Add(time.Minute).In(shanghai)},
			} {
				exists, err := repo.HasScheduledJobInstanceInWindow(ctx, 7, bounds[0], bounds[1])
				if err != nil || !exists {
					t.Fatalf("%s window: expected existing scheduled job, got %v %v", label, exists, err)
				}
			}
			if exists, _ := repo.HasScheduledJobInstanceInWindow(ctx, 7, windowStart.Add(time.Hour), windowStart.Add(time.Hour+time.Minute)); exists {
				t.Fatal("future window must not match")
			}

			cfg := &configapp.Config{ClusterID: 3, ConfigType: "seatunnel"}
			if err := database.Create(cfg).Error; err != nil {
				t.Fatal(err)
			}
			items, err := configapp.NewRepository(database).ListUpdatedByClusterBetween(ctx, 3, now.Add(-time.Minute).In(shanghai), now.Add(time.Minute).In(shanghai))
			if err != nil || len(items) != 1 {
				t.Fatalf("config range: expected 1 item, got %d %v", len(items), err)
			}
		})
	}
}
