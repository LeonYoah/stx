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

	warnLegacyObservabilitySections(loader)

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

// warnLegacyObservabilitySections 检测配置文件中残留的旧监控配置段并向 stderr 记一条 WARN；
// 只打印段名，不输出任何配置值（token 等敏感信息不落日志）。
// warnLegacyObservabilitySections warns once on stderr about leftover legacy monitoring sections;
// only section names are printed, never values (tokens and other secrets stay out of logs).
func warnLegacyObservabilitySections(loader *viper.Viper) {
	if loader == nil {
		return
	}
	leftovers := make([]string, 0, len(legacyObservabilitySections))
	for _, key := range legacyObservabilitySections {
		if loader.IsSet(key) {
			leftovers = append(leftovers, key)
		}
	}
	if len(leftovers) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "[Config] WARN: 检测到已废弃的旧外部监控配置段 [%s]，这些段不再生效，请从配置文件中移除 / deprecated external monitoring sections [%s] detected; they no longer take effect, please remove them from the config file\n",
		strings.Join(leftovers, ", "), strings.Join(leftovers, ", "))
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
	if strings.TrimSpace(c.Observability.Storage.Retention) == "" {
		c.Observability.Storage.Retention = "31d"
	}
	// 存储上限默认 30GiB；显式写 0 才表示不限，与 enabled 的 IsSet 默认值语义一致。
	// Storage cap defaults to 30GiB; an explicit 0 means unlimited, mirroring the IsSet-based
	// default semantics used by observability.enabled.
	if !storageMaxSizeSet {
		c.Observability.Storage.MaxSize = 30 << 30
	}
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
	// 与 Prometheus 默认 query.max-samples 一致；0 视为未配置，负值交给校验拒绝。
	// Same as the Prometheus default query.max-samples; 0 is treated as unset while
	// explicit negatives are rejected by validation.
	if c.Observability.MaxSamples == 0 {
		c.Observability.MaxSamples = 50000000
	}
}

func validateConfig(c *configModel) error {
	if c == nil {
		return nil
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
	if err := validateObservabilityDuration("observability.storage.retention", c.Observability.Storage.Retention); err != nil {
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
