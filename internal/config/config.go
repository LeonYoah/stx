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

package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/pkg/timex"
	"github.com/prometheus/common/model"
	"github.com/spf13/viper"
)

var (
	Config          *configModel
	serverConfigErr error
	// detectedLegacyObservabilitySections 是最近一次 loadConfig 检测到的残留旧监控段名（只存段名，
	// 不存值），由 router.Serve() 启动时记一次 WARN，避免每条 CLI 命令都打 stderr。
	// detectedLegacyObservabilitySections holds legacy monitoring section names found by the last
	// loadConfig call (names only, never values); router.Serve() logs one WARN at startup so every
	// CLI invocation does not spam stderr.
	detectedLegacyObservabilitySections []string
)

func init() {
	Config, serverConfigErr = loadConfig()
	if Config != nil {
		if err := timex.SetDisplayTimezone(Config.App.DisplayTimezone); err != nil {
			fmt.Fprintf(os.Stderr, "[Config] %v, fallback to %s\n", err, timex.DefaultDisplayTimezone)
			_ = timex.SetDisplayTimezone(timex.DefaultDisplayTimezone)
			Config.App.DisplayTimezone = timex.DefaultDisplayTimezone
		}
	}
}

// ServerConfigError 返回服务端配置加载或校验错误，CLI 本地命令不受该错误影响。
// ServerConfigError returns the server configuration loading or validation error without blocking local CLI commands.
func ServerConfigError() error {
	return serverConfigErr
}

// loadConfig 加载服务端配置；失败时返回可供本地 CLI 使用的安全默认配置。
// loadConfig loads server configuration and returns safe defaults for local CLI usage on failure.
func loadConfig() (*configModel, error) {
	// 加载配置文件路径。/ Load the configuration file path.
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	loader := viper.New()
	loader.SetConfigFile(configPath)
	loader.AutomaticEnv()

	// 本地 CLI 必须能在没有服务端配置文件时运行，因此保留错误并返回默认配置。
	// Local CLI commands must work without a server config file, so preserve the error and return defaults.
	if err := loader.ReadInConfig(); err != nil {
		fallback := &configModel{}
		setDefaults(fallback, false, false)
		applyEnvironmentOverrides(fallback)
		return fallback, fmt.Errorf("read server config %q: %w", configPath, err)
	}

	detectedLegacyObservabilitySections = detectLegacyObservabilitySections(loader)

	// 解析配置到结构体。/ Decode the configuration into the model.
	var c configModel
	if err := loader.Unmarshal(&c); err != nil {
		fallback := &configModel{}
		setDefaults(fallback, false, false)
		applyEnvironmentOverrides(fallback)
		return fallback, fmt.Errorf("parse server config %q: %w", configPath, err)
	}

	// 设置默认值并应用环境变量。/ Apply defaults and environment overrides.
	setDefaults(&c, loader.IsSet("observability.enabled"), loader.IsSet("observability.storage.max_size"))
	applyEnvironmentOverrides(&c)

	if os.Getenv("GO_TEST") != "1" && !isTestEnvironment() {
		if err := validateConfig(&c); err != nil {
			return &c, fmt.Errorf("validate server config %q: %w", configPath, err)
		}
	}

	return &c, nil
}

// legacyObservabilitySections 是内嵌可观测落地后退役的旧外部监控栈配置段；配置文件里残留这些段
// 时忽略不阻塞，但启动记一条 WARN 提示清理。
// legacyObservabilitySections lists the external monitoring stack sections retired by the embedded
// stack; leftover sections are ignored but trigger one startup WARN to prompt cleanup.
var legacyObservabilitySections = []string{
	"observability.prometheus",
	"observability.alertmanager",
	"observability.grafana",
	"observability.remote_ingest",
}

// LegacyObservabilitySections 返回最近一次加载配置时检测到的残留旧监控段名（供服务启动时记
// 一次 WARN；加载失败或无残留时为空）。返回副本，调用方不能改动内部状态。
// LegacyObservabilitySections returns legacy monitoring section names detected during the last
// config load (for one WARN at server startup; empty when the load failed or nothing remains).
// Returns a copy so callers cannot mutate internal state.
func LegacyObservabilitySections() []string {
	return append([]string(nil), detectedLegacyObservabilitySections...)
}

// detectLegacyObservabilitySections 返回配置文件里残留的旧监控段名；只收集段名，不读取配置值
// （token 等敏感信息不会出现在返回值里，也就不会落日志）。
// detectLegacyObservabilitySections returns leftover legacy monitoring section names; it only
// collects names and never reads values (tokens cannot appear in the result, so they stay out of logs).
func detectLegacyObservabilitySections(loader *viper.Viper) []string {
	if loader == nil {
		return nil
	}
	leftovers := make([]string, 0, len(legacyObservabilitySections))
	for _, key := range legacyObservabilitySections {
		if loader.IsSet(key) {
			leftovers = append(leftovers, key)
		}
	}
	return leftovers
}

// applyEnvironmentOverrides 从环境变量覆盖关键配置（便于容器化与 E2E 测试环境动态切换）
// applyEnvironmentOverrides overrides key configurations from environment variables (useful for containerization and dynamic E2E switching)
func applyEnvironmentOverrides(c *configModel) {
	if c == nil {
		return
	}

	// 展示时区环境变量覆盖（兼容 STX_REPORT_TIMEZONE）/ Display timezone env override (legacy STX_REPORT_TIMEZONE alias)
	if tz := timex.EnvDisplayTimezone(); tz != "" {
		c.App.DisplayTimezone = tz
	}

	// 数据库环境变量覆盖 / Database environment variable overrides
	if dbType := os.Getenv("STX_DATABASE_TYPE"); dbType != "" {
		c.Database.Type = dbType
	}
	if dbHost := os.Getenv("STX_DATABASE_HOST"); dbHost != "" {
		c.Database.Host = dbHost
	}
	if dbPortStr := os.Getenv("STX_DATABASE_PORT"); dbPortStr != "" {
		if p, err := strconv.Atoi(dbPortStr); err == nil {
			c.Database.Port = p
		}
	}
	if dbUser := os.Getenv("STX_DATABASE_USERNAME"); dbUser != "" {
		c.Database.Username = dbUser
	}
	if dbPass := os.Getenv("STX_DATABASE_PASSWORD"); dbPass != "" {
		c.Database.Password = dbPass
	}
	if dbName := os.Getenv("STX_DATABASE_DATABASE"); dbName != "" {
		c.Database.Database = dbName
	}
	if sqlitePath := os.Getenv("STX_DATABASE_SQLITE_PATH"); sqlitePath != "" {
		c.Database.SQLitePath = sqlitePath
	}
}

// isTestEnvironment 检测是否在测试环境中运行
func isTestEnvironment() bool {
	// 检查是否通过 go test 运行
	for _, arg := range os.Args {
		if len(arg) > 5 && arg[:5] == "-test" {
			return true
		}
	}
	return false
}

// setDefaults 设置配置默认值，并保留用户显式设置的可观测性开关。
// setDefaults applies defaults while preserving an explicitly configured observability switch.
func setDefaults(c *configModel, observabilityEnabledSet bool, storageMaxSizeSet bool) {
	if strings.TrimSpace(c.App.DisplayTimezone) == "" {
		c.App.DisplayTimezone = timex.DefaultDisplayTimezone
	}
	if strings.TrimSpace(c.App.Pprof.Addr) == "" {
		c.App.Pprof.Addr = "127.0.0.1:6060"
	}
	// 数据库默认配置
	if c.Database.Type == "" {
		c.Database.Type = "sqlite"
	}
	if c.Database.SQLitePath == "" {
		c.Database.SQLitePath = "./data/stx.db"
	}

	if c.Sync.PreviewDataTTLMinutes <= 0 && c.Sync.PreviewDataTTLHours <= 0 {
		c.Sync.PreviewDataTTLMinutes = 24 * 60
	}
	if c.Sync.JobReconcilerIntervalSeconds == 0 {
		c.Sync.JobReconcilerIntervalSeconds = 10
	}
	if c.Sync.SubmitTimeoutSeconds <= 0 {
		c.Sync.SubmitTimeoutSeconds = 300
	}
	if c.Sync.StopTimeoutSeconds <= 0 {
		c.Sync.StopTimeoutSeconds = 300
	}
	if c.Sync.ExternalJobDiscoveryIntervalSeconds <= 0 {
		c.Sync.ExternalJobDiscoveryIntervalSeconds = 60
	}
	if c.Sync.ExternalJobDiscoveryMaxNewPerRound <= 0 {
		c.Sync.ExternalJobDiscoveryMaxNewPerRound = 200
	}
	if c.Sync.JobReconcilerClusterConcurrency <= 0 {
		c.Sync.JobReconcilerClusterConcurrency = 4
	}
	if c.Sync.JobReconcilerMaxBackoffSeconds <= 0 {
		c.Sync.JobReconcilerMaxBackoffSeconds = 120
	}
	if c.Sync.EngineRequestTimeoutSeconds <= 0 {
		c.Sync.EngineRequestTimeoutSeconds = 15
	}

	// 认证默认配置
	if c.Auth.DefaultAdminUsername == "" {
		c.Auth.DefaultAdminUsername = "admin"
	}
	if c.Auth.DefaultAdminPassword == "" {
		c.Auth.DefaultAdminPassword = "admin123"
	}
	if c.Auth.BcryptCost == 0 {
		c.Auth.BcryptCost = 10
	}

	// 日志默认配置
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "console"
	}
	if c.Log.Output == "" {
		c.Log.Output = "stdout"
	}

	// gRPC 默认配置
	if c.GRPC.Port == 0 {
		c.GRPC.Port = 17890
	}
	if c.GRPC.MaxRecvMsgSize == 0 {
		c.GRPC.MaxRecvMsgSize = 16 // 16MB
	}
	if c.GRPC.MaxSendMsgSize == 0 {
		c.GRPC.MaxSendMsgSize = 16 // 16MB
	}
	if c.GRPC.HeartbeatInterval == 0 {
		c.GRPC.HeartbeatInterval = 10 // 10 seconds
	}
	if c.GRPC.HeartbeatTimeout == 0 {
		c.GRPC.HeartbeatTimeout = 30 // 30 seconds
	}

	// 存储默认配置
	if c.Storage.BaseDir == "" {
		c.Storage.BaseDir = "./data/storage"
	}
	if c.Storage.PackagesDir == "" {
		c.Storage.PackagesDir = "./lib/packages"
	}
	if c.Storage.PluginsDir == "" {
		c.Storage.PluginsDir = "./lib/plugins"
	}
	if c.Storage.TempDir == "" {
		c.Storage.TempDir = "./data/storage/temp"
	}
	if c.Storage.MaxPackageSize == 0 {
		c.Storage.MaxPackageSize = 2048 // 2GB
	}
	if c.Storage.CleanupIntervalHours == 0 {
		c.Storage.CleanupIntervalHours = 24
	}

	// 可观测性默认配置（prometheus.url / alertmanager.url 与 remote_ingest 已随进程内告警评估
	// 退役，不再有默认值）。
	// Observability defaults (prometheus.url / alertmanager.url and remote_ingest retired with
	// in-process alert evaluation and no longer carry defaults).

	// 默认启用可观测中心（仅在用户未显式配置时）
	if !observabilityEnabledSet {
		c.Observability.Enabled = true
	}

	if c.Observability.SeatunnelMetric.Path == "" {
		// 默认使用 SeaTunnel Engine Telemetry 文档中的 Hazelcast REST metrics 路径：
		// http://{instanceHost}:5801/hazelcast/rest/instance/metrics
		c.Observability.SeatunnelMetric.Path = "/hazelcast/rest/instance/metrics"
	}
	if c.Observability.SeatunnelMetric.ProbeTimeoutSeconds <= 0 {
		c.Observability.SeatunnelMetric.ProbeTimeoutSeconds = 2
	}

	// 内嵌 tsdb 存储与查询默认配置
	// Embedded tsdb storage and query defaults
	if strings.TrimSpace(c.Observability.Storage.Dir) == "" {
		c.Observability.Storage.Dir = "./data/tsdb"
	}
	// 时间保留期缺省留空：默认只按容量清理、不按时间删除；显式写 "0" 同义。
	// Time retention stays empty by default: data is pruned by capacity only; an explicit "0" is equivalent.
	c.Observability.Storage.MaxSizeSet = storageMaxSizeSet
	// max_size 不在配置层填默认值：实际缺省值依赖数据目录所在磁盘（min(30GiB, 磁盘总容量 20%)，
	// 下限 1GiB），由 embedts 启动时计算；显式写 0 表示容量不限。
	// max_size gets no config-level default: the effective cap depends on the data-dir disk
	// (min(30GiB, 20% of disk total), floored at 1GiB) and is computed when embedts starts;
	// an explicit 0 means unlimited.
	if strings.TrimSpace(c.Observability.ScrapeInterval) == "" {
		c.Observability.ScrapeInterval = "15s"
	}
	if strings.TrimSpace(c.Observability.EvalInterval) == "" {
		c.Observability.EvalInterval = "15s"
	}
	// 8s 与原远程查询 HTTP 客户端超时时长一致，保持降级语义不变。
	// 8s matches the legacy remote-query HTTP client timeout, preserving degradation behavior.
	if strings.TrimSpace(c.Observability.QueryTimeout) == "" {
		c.Observability.QueryTimeout = "8s"
	}
	// 8G 部署机上收紧后的单次查询上限（约 160MB）；0 视为未配置，负值交给校验拒绝。
	// Tightened per-query cap for 8G hosts (~160MB); 0 is treated as unset while
	// explicit negatives are rejected by validation.
	if c.Observability.MaxSamples == 0 {
		c.Observability.MaxSamples = 10000000
	}
	// 每目标抓取样本上限；0 视为未配置。 / Per-target scrape sample cap; 0 means unset.
	if c.Observability.SampleLimit == 0 {
		c.Observability.SampleLimit = 50000
	}
}

func validateConfig(c *configModel) error {
	if c == nil {
		return nil
	}
	if err := validatePprofConfig(c.App.Pprof); err != nil {
		return err
	}
	if !c.Observability.Enabled {
		return nil
	}

	if err := validateRequiredHTTPURL("app.external_url", c.App.ExternalURL); err != nil {
		return err
	}
	// 内嵌 tsdb 存储与查询参数校验
	// Embedded tsdb storage and query parameter validation
	if strings.TrimSpace(c.Observability.Storage.Dir) == "" {
		return fmt.Errorf("observability.storage.dir is required when observability.enabled=true")
	}
	if err := validateObservabilityRetention(c.Observability.Storage.Retention); err != nil {
		return err
	}
	if c.Observability.Storage.MaxSize < 0 {
		return fmt.Errorf("observability.storage.max_size must be >= 0 (0 means unlimited)")
	}
	if err := validateObservabilityDuration("observability.scrape_interval", c.Observability.ScrapeInterval); err != nil {
		return err
	}
	if err := validateObservabilityDuration("observability.eval_interval", c.Observability.EvalInterval); err != nil {
		return err
	}
	if err := validateObservabilityDuration("observability.query_timeout", c.Observability.QueryTimeout); err != nil {
		return err
	}
	if c.Observability.MaxSamples <= 0 {
		return fmt.Errorf("observability.max_samples must be > 0")
	}
	if c.Observability.SampleLimit < 0 {
		return fmt.Errorf("observability.sample_limit must be >= 0")
	}
	return nil
}

// validatePprofConfig 仅在开启时校验 host:port；关闭时允许任意占位地址。
// validatePprofConfig checks host:port only when pprof is on; disabled configs may keep a placeholder addr.
func validatePprofConfig(p PprofConfig) error {
	if !p.Enabled {
		return nil
	}
	addr := strings.TrimSpace(p.Addr)
	if addr == "" {
		return fmt.Errorf("app.pprof.addr is required when app.pprof.enabled=true")
	}
	if _, port, err := net.SplitHostPort(addr); err != nil {
		return fmt.Errorf("app.pprof.addr must be host:port: %w", err)
	} else if strings.TrimSpace(port) == "" {
		return fmt.Errorf("app.pprof.addr must include a port")
	}
	return nil
}

func validateRequiredHTTPURL(name, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("%s is required when observability.enabled=true", name)
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("%s parse failed: %w", name, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must start with http:// or https://", name)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("%s must include host", name)
	}
	return nil
}

func validateRequiredPath(name, raw string) error {
	path := strings.TrimSpace(raw)
	if path == "" {
		return fmt.Errorf("%s is required when observability.enabled=true", name)
	}
	if !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%s must start with '/'", name)
	}
	return nil
}

// validateObservabilityDuration 校验可观测时长配置：非空、可被 Prometheus duration 语法解析
// （支持 s/m/h/d/w/y，如 15s、31d）、且大于 0。
// validateObservabilityDuration validates an observability duration setting: non-empty, parseable
// by the Prometheus duration syntax (s/m/h/d/w/y, e.g. 15s or 31d), and greater than zero.
func validateObservabilityDuration(name, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("%s is required when observability.enabled=true", name)
	}
	d, err := model.ParseDuration(trimmed)
	if err != nil {
		return fmt.Errorf("%s must be a valid duration such as 15s or 31d: %w", name, err)
	}
	if d <= 0 {
		return fmt.Errorf("%s must be greater than 0", name)
	}
	return nil
}

// validateObservabilityRetention 校验时间保留期：允许留空或 "0"（不按时间删除，默认只按容量清理）；
// 非空时必须是合法的 Prometheus duration 且不小于 0。
// validateObservabilityRetention validates the time retention: empty or "0" disables time-based
// deletion (capacity-only retention is the default); a non-empty value must be a valid
// non-negative Prometheus duration.
func validateObservabilityRetention(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	d, err := model.ParseDuration(trimmed)
	if err != nil {
		return fmt.Errorf("observability.storage.retention must be a valid duration such as 30d or 90d: %w", err)
	}
	if d < 0 {
		return fmt.Errorf("observability.storage.retention must be >= 0 (empty or 0 disables time-based retention)")
	}
	return nil
}

// GetDisplayTimezone 获取系统默认展示时区 / GetDisplayTimezone returns the system display timezone.
func GetDisplayTimezone() string {
	return Config.App.DisplayTimezone
}

// GetDatabaseLegacyTimezone 返回历史数据写入时区（仅用于 UTC 迁移）/ legacy write timezone used only for the UTC migration.
func GetDatabaseLegacyTimezone() string {
	return Config.Database.LegacyTimezone
}

// GetDatabaseType 获取数据库类型
func GetDatabaseType() string {
	return Config.Database.Type
}

// GetSQLitePath 获取 SQLite 文件路径
func GetSQLitePath() string {
	return Config.Database.SQLitePath
}

// GetAuthConfig 获取认证配置
func GetAuthConfig() authConfig {
	return Config.Auth
}

// GetStorageConfig 获取存储配置
func GetStorageConfig() StorageConfig {
	return Config.Storage
}

// GetPackagesDir 获取安装包存储目录
func GetPackagesDir() string {
	if Config.Storage.PackagesDir != "" {
		return Config.Storage.PackagesDir
	}
	return "./lib/packages"
}

// GetPluginsDir 获取插件存储目录
func GetPluginsDir() string {
	if Config.Storage.PluginsDir != "" {
		return Config.Storage.PluginsDir
	}
	return "./lib/plugins"
}

// GetTempDir 获取临时文件目录
func GetTempDir() string {
	if Config.Storage.TempDir != "" {
		return Config.Storage.TempDir
	}
	return "./data/storage/temp"
}

// GetMaxPackageSize 获取最大安装包大小（字节）
func GetMaxPackageSize() int64 {
	if Config.Storage.MaxPackageSize > 0 {
		return Config.Storage.MaxPackageSize * 1024 * 1024 // MB to bytes
	}
	return 2048 * 1024 * 1024 // 默认 2GB
}

// GetGRPCConfig 获取 gRPC 配置
// GetGRPCConfig returns the gRPC configuration
func GetGRPCConfig() GRPCConfig {
	return Config.GRPC
}

// GetExternalURL 获取外部访问 URL
// GetExternalURL returns the external URL for accessing the Control Plane
func GetExternalURL() string {
	if Config.App.ExternalURL != "" {
		return Config.App.ExternalURL
	}
	// Fallback: if external_url is not set, return empty string
	// 回退：如果未设置 external_url，返回空字符串
	// The caller should handle this case appropriately
	// 调用者应适当处理这种情况
	return ""
}

// GetConsoleURL 返回控制台（前端）的访问地址，去掉末尾的斜杠；未配置时返回空字符串。
// GetConsoleURL returns the console (frontend) address without a trailing slash; empty when not configured.
func GetConsoleURL() string {
	if Config == nil {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(Config.App.ConsoleURL), "/")
}

// IsGRPCEnabled 检查 gRPC 是否启用
// IsGRPCEnabled checks if gRPC server is enabled
func IsGRPCEnabled() bool {
	return Config.GRPC.Enabled
}

// GetGRPCPort 获取 gRPC 端口
// GetGRPCPort returns the gRPC server port
func GetGRPCPort() int {
	if Config.GRPC.Port > 0 {
		return Config.GRPC.Port
	}
	return 17890
}
