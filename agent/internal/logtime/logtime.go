// Package logtime 解析 SeaTunnel/Agent 日志中不带偏移的时间戳并统一转为 UTC。
// Package logtime parses naive SeaTunnel/agent log timestamps and converts them to UTC.
package logtime

import (
	"os"
	"strings"
	"sync"
	"time"
)

// EnvSeatunnelTimezone 指定 SeaTunnel 日志时区（IANA），默认使用 Agent 主机本地时区。
// EnvSeatunnelTimezone sets the SeaTunnel log timezone (IANA); defaults to the agent host's local zone.
const EnvSeatunnelTimezone = "STX_SEATUNNEL_TIMEZONE"

var (
	once sync.Once
	loc  *time.Location
)

// Location 返回解析无偏移日志时间使用的时区。/ Location returns the zone used for naive log timestamps.
func Location() *time.Location {
	once.Do(func() {
		loc = time.Local
		if name := strings.TrimSpace(os.Getenv(EnvSeatunnelTimezone)); name != "" {
			if l, err := time.LoadLocation(name); err == nil {
				loc = l
			}
		}
	})
	return loc
}

// Parse 依次尝试 layouts，在日志时区解析并返回 UTC 时刻。/ Parse tries layouts in the log zone and returns a UTC instant.
func Parse(value string, layouts ...string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, Location()); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

// SetLocation 覆盖日志时区（主要用于测试或启动参数）。/ SetLocation overrides the log zone (tests or startup flags).
func SetLocation(l *time.Location) {
	once.Do(func() {})
	if l != nil {
		loc = l
	}
}
