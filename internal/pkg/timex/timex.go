/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package timex 提供全局统一的时区策略：存储一律 UTC，展示按配置时区或用户时区。
// Package timex provides the unified timezone policy: persist in UTC, display in the configured or user timezone.
package timex

import (
	"context"
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

	// engineLocationLookup 由控制面注入：按集群取 Master 节点 Agent 上报的 IANA 时区。
	// engineLocationLookup is injected by the control plane: the IANA zone the cluster master Agent reported.
	engineLocationMu     sync.RWMutex
	engineLocationLookup func(ctx context.Context, clusterID uint) *time.Location
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

// SetEngineLocationLookup 注入按集群解析引擎时区的回调（通常读 Master 节点心跳上报的 timezone）。
// 回调应使用调用方 ctx；GetNodes 会查库，EngineLocation 会给没有截止时间的 ctx 加上超时。
// SetEngineLocationLookup injects the per-cluster engine-zone lookup (usually the master node's heartbeat timezone).
// The callback should use the caller's ctx; GetNodes hits the DB, and EngineLocation adds a timeout when the ctx has
// no deadline.
func SetEngineLocationLookup(fn func(ctx context.Context, clusterID uint) *time.Location) {
	engineLocationMu.Lock()
	engineLocationLookup = fn
	engineLocationMu.Unlock()
}

// LoadIANA 解析 IANA 时区名；空串、"Local" 或非法名称返回 nil。
// LoadIANA parses an IANA zone name; empty, "Local", or invalid names return nil.
func LoadIANA(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "Local") {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

func envSeatunnelLocation() *time.Location {
	return LoadIANA(os.Getenv("STX_SEATUNNEL_TIMEZONE"))
}

func localOrUTC() *time.Location {
	if loc := time.Local; loc != nil {
		return loc
	}
	return time.UTC
}

func lookupEngineLocation(ctx context.Context, clusterID uint) *time.Location {
	if clusterID == 0 {
		return nil
	}
	engineLocationMu.RLock()
	fn := engineLocationLookup
	engineLocationMu.RUnlock()
	if fn == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return fn(ctx, clusterID)
}

// SeatunnelLocation 返回解析 SeaTunnel 引擎/日志中无偏移时间时使用的时区：
// 1) 环境变量 STX_SEATUNNEL_TIMEZONE（最高优先级）；2) 控制面进程本地时区（time.Local）。
// 有集群上下文时请用 SeatunnelLocationForCluster，以便采用 Agent 心跳上报的节点时区。
// SeatunnelLocation returns the zone for naive SeaTunnel engine/log timestamps: STX_SEATUNNEL_TIMEZONE first, then the
// control-plane process local zone (time.Local). Prefer SeatunnelLocationForCluster when a cluster id is known so the
// Agent-reported node zone is used.
func SeatunnelLocation() *time.Location {
	return SeatunnelLocationForCluster(context.Background(), 0)
}

// SeatunnelLocationForCluster 解析指定集群的引擎无偏移时间：STX_SEATUNNEL_TIMEZONE > Master 心跳时区 > time.Local。
// SeatunnelLocationForCluster parses naive engine times for a cluster: STX_SEATUNNEL_TIMEZONE, then the master
// heartbeat zone, then time.Local.
func SeatunnelLocationForCluster(ctx context.Context, clusterID uint) *time.Location {
	if loc := envSeatunnelLocation(); loc != nil {
		return loc
	}
	if loc := lookupEngineLocation(ctx, clusterID); loc != nil {
		return loc
	}
	return localOrUTC()
}

// SeatunnelLocationForHost 在已知节点 IANA 时区时解析无偏移时间：STX_SEATUNNEL_TIMEZONE > 节点时区 > time.Local。
// SeatunnelLocationForHost parses naive times when a node IANA zone is known: STX_SEATUNNEL_TIMEZONE, then that zone,
// then time.Local.
func SeatunnelLocationForHost(iana string) *time.Location {
	if loc := envSeatunnelLocation(); loc != nil {
		return loc
	}
	if loc := LoadIANA(iana); loc != nil {
		return loc
	}
	return localOrUTC()
}
