// Package timex 提供全局统一的时区策略：存储一律 UTC，展示按配置时区或用户时区。
// Package timex provides the unified timezone policy: persist in UTC, display in the configured or user timezone.
package timex

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	// 内嵌 IANA 时区库，避免精简镜像缺少 tzdata 时 LoadLocation 失败。
	// Embed the IANA timezone database so LoadLocation works on images without tzdata.
	_ "time/tzdata"
)

// DefaultDisplayTimezone 系统默认展示时区。/ System default display timezone.
const DefaultDisplayTimezone = "Asia/Shanghai"

var (
	displayMu   sync.RWMutex
	displayName string
	displayLoc  *time.Location
)

// Now 返回当前 UTC 时间，作为 GORM NowFunc 与所有持久化写入的时间源。
// Now returns the current UTC time; used as GORM NowFunc and for all persisted writes.
func Now() time.Time { return time.Now().UTC() }

// Bound 将查询区间边界规范化为 UTC，保证与 UTC 存储的列在所有驱动（尤其 SQLite 文本比较）下语义一致。
// Bound normalizes a range-query bound to UTC so comparisons against UTC-stored columns are correct on every driver
// (SQLite compares DATETIME as text).
func Bound(t time.Time) time.Time { return t.UTC() }

// BoundPtr 是 Bound 的指针版本。/ BoundPtr is the pointer variant of Bound.
func BoundPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	v := t.UTC()
	return &v
}

// EnvDisplayTimezone 读取环境变量中的展示时区（STX_DISPLAY_TIMEZONE，兼容旧的 STX_REPORT_TIMEZONE）。
// EnvDisplayTimezone reads the display timezone from env (STX_DISPLAY_TIMEZONE, legacy alias STX_REPORT_TIMEZONE).
func EnvDisplayTimezone() string {
	if v := strings.TrimSpace(os.Getenv("STX_DISPLAY_TIMEZONE")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("STX_REPORT_TIMEZONE"))
}

// SetDisplayTimezone 设置系统默认展示时区；非法名称返回错误且不修改当前值。
// SetDisplayTimezone sets the system display timezone; invalid names return an error and leave the value unchanged.
func SetDisplayTimezone(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		name = DefaultDisplayTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return fmt.Errorf("invalid display timezone %q: %w", name, err)
	}
	displayMu.Lock()
	displayName, displayLoc = name, loc
	displayMu.Unlock()
	return nil
}

// DisplayTimezone 返回系统默认展示时区名称。/ DisplayTimezone returns the system display timezone name.
func DisplayTimezone() string {
	name, _ := current()
	return name
}

// DisplayLocation 返回系统默认展示时区。/ DisplayLocation returns the system display location.
func DisplayLocation() *time.Location {
	_, loc := current()
	return loc
}

func current() (string, *time.Location) {
	displayMu.RLock()
	name, loc := displayName, displayLoc
	displayMu.RUnlock()
	if loc != nil {
		return name, loc
	}
	if err := SetDisplayTimezone(EnvDisplayTimezone()); err != nil {
		_ = SetDisplayTimezone(DefaultDisplayTimezone)
	}
	displayMu.RLock()
	defer displayMu.RUnlock()
	if displayLoc == nil {
		// tzdata 缺失时的兜底：明确使用 UTC，而不是伪造固定偏移。/ Fallback when tzdata is missing: UTC, never a fake fixed offset.
		return "UTC", time.UTC
	}
	return displayName, displayLoc
}

// FormatAPI 以 RFC3339（UTC，带 Z）序列化时间，零值返回空串。
// FormatAPI serializes a time as RFC3339 in UTC (with Z); zero time yields "".
func FormatAPI(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ParseAPITime 解析 API 时间参数：优先 RFC3339（必须带偏移或 Z）；
// 无偏移的 "2006-01-02 15:04:05"/"2006-01-02T15:04:05"/"2006-01-02" 仅在显式提供 tz 时接受。
// ParseAPITime parses API time input: RFC3339 with offset/Z; naive layouts are accepted only with an explicit tz.
func ParseAPITime(value, tz string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty time value")
	}
	if t, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return t.UTC(), nil
	}
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return time.Time{}, fmt.Errorf("time %q must be RFC3339 with offset (e.g. 2026-01-02T15:04:05Z) or be accompanied by a tz parameter", value)
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid tz %q: %w", tz, err)
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format %q", value)
}

// BoundAny 对 time.Time / *time.Time 值做 Bound，其他类型原样返回。
// BoundAny applies Bound to time.Time or *time.Time values and returns other values unchanged.
func BoundAny(v interface{}) interface{} {
	switch t := v.(type) {
	case time.Time:
		return t.UTC()
	case *time.Time:
		return BoundPtr(t)
	}
	return v
}

// SeatunnelLocation 返回解析 SeaTunnel 引擎/日志中无偏移时间时使用的时区：
// 环境变量 STX_SEATUNNEL_TIMEZONE，未设置时使用系统展示时区。
// SeatunnelLocation returns the zone for naive SeaTunnel engine/log timestamps: STX_SEATUNNEL_TIMEZONE or the display zone.
func SeatunnelLocation() *time.Location {
	if name := strings.TrimSpace(os.Getenv("STX_SEATUNNEL_TIMEZONE")); name != "" {
		if loc, err := time.LoadLocation(name); err == nil {
			return loc
		}
	}
	return DisplayLocation()
}
