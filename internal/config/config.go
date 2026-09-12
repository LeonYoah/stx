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
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/viper"
)

var Config *configModel

func init() {
	// 加载配置文件路径
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	// 设置配置文件
	viper.SetConfigFile(configPath)
	viper.AutomaticEnv()

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		// 在测试环境中，如果配置文件不存在，使用默认配置
		if os.Getenv("GO_TEST") == "1" || isTestEnvironment() {
			log.Printf("[Config] 测试环境，使用默认配置: %v\n", err)
			Config = &configModel{}
			setDefaults(Config)
			return
		}
		log.Fatalf("[Config] read config failed: %v\n", err)
	}

	// 解析配置到结构体
	var c configModel
	if err := viper.Unmarshal(&c); err != nil {
		log.Fatalf("[Config] parse config failed: %v\n", err)
	}

	// 设置默认值
	setDefaults(&c)
	if os.Getenv("GO_TEST") != "1" && !isTestEnvironment() {
		if err := validateConfig(&c); err != nil {
			log.Fatalf("[Config] validate config failed: %v\n", err)
		}
	}

	// 设置全局配置
	Config = &c
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

// setDefaults 设置配置默认值
func setDefaults(c *configModel) {
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
		c.GRPC.Port = 9000
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

	// 可观测性默认配置
	if c.Observability.Prometheus.URL == "" {
		c.Observability.Prometheus.URL = "http://127.0.0.1:9090"
	}
	if c.Observability.Prometheus.HTTPSDPath == "" {
		c.Observability.Prometheus.HTTPSDPath = "/api/v1/monitoring/prometheus/discovery"
	}
	if c.Observability.Alertmanager.URL == "" {
		c.Observability.Alertmanager.URL = "http://127.0.0.1:9093"
	}
	if c.Observability.Alertmanager.WebhookPath == "" {
		c.Observability.Alertmanager.WebhookPath = "/api/v1/monitoring/alertmanager/webhook"
	}
	if c.Observability.Grafana.URL == "" {
		c.Observability.Grafana.URL = "http://127.0.0.1:3000"
	}

	// 默认启用可观测中心（仅在用户未显式配置时）
	if !viper.IsSet("observability.enabled") {
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
	if err := validateOptionalHTTPURL("observability.prometheus.url", c.Observability.Prometheus.URL); err != nil {
		return err
	}
	if err := validateOptionalHTTPURL("observability.alertmanager.url", c.Observability.Alertmanager.URL); err != nil {
		return err
	}
	if err := validateOptionalHTTPURL("observability.grafana.url", c.Observability.Grafana.URL); err != nil {
		return err
	}
	if err := validateRequiredPath("observability.prometheus.http_sd_path", c.Observability.Prometheus.HTTPSDPath); err != nil {
		return err
	}
	if err := validateRequiredPath("observability.alertmanager.webhook_path", c.Observability.Alertmanager.WebhookPath); err != nil {
		return err
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

func validateOptionalHTTPURL(name, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return validateRequiredHTTPURL(name, trimmed)
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
	return 9000
}
