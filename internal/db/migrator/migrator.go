/*
 * MIT License
 *
 * Copyright (c) 2025 linux.do
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package migrator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	"github.com/LeonYoah/stx/internal/apps/diagnostics"
	"github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/apps/host"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/apps/plugin"
	"github.com/LeonYoah/stx/internal/apps/stupgrade"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"gorm.io/gorm"
)

// Migrate 执行当前全局数据库的自动迁移与初始化
// Migrate executes auto-migration and initialization for the current global database
func Migrate() {
	// 先初始化数据库连接 / Initialize database connection first
	if err := db.InitDatabase(); err != nil {
		log.Fatalf("[Database] 初始化数据库失败: %v\n", err)
	}

	// 检查数据库是否已初始化 / Check if database is initialized
	if !db.IsDatabaseInitialized() {
		log.Println("[Database] 数据库未启用，跳过迁移")
		return
	}

	if err := MigrateWithDB(db.GetDB(context.Background()), db.GetDatabaseType()); err != nil {
		log.Fatalf("[Database] auto migrate failed: %v\n", err)
	}
	log.Printf("[Database] auto migrate success\n")
}

// MigrateWithDB 使用指定的 GORM 数据库实例和数据库类型执行全量迁移
// MigrateWithDB executes full migration using the specified GORM database instance and database type
func MigrateWithDB(database *gorm.DB, dbType string) error {
	if database == nil {
		return errors.New("数据库连接未初始化 / database connection is nil")
	}

	// 兼容迁移：若历史数据库中 auth_users 表包含 o_auth_id 列但无 oauth_id 列，自动重命名为 oauth_id
	if database.Migrator().HasTable(&auth.User{}) {
		if database.Migrator().HasColumn(&auth.User{}, "o_auth_id") && !database.Migrator().HasColumn(&auth.User{}, "oauth_id") {
			if err := database.Migrator().RenameColumn(&auth.User{}, "o_auth_id", "oauth_id"); err != nil {
				log.Printf("[Database] failed to rename o_auth_id to oauth_id: %v\n", err)
			}
		}
	}

	// 执行数据库表迁移，包含用户表
	// 注意：auth.User 是统一的用户表，同时支持密码登录和 OAuth 登录
	// Execute database table migration, including user table
	// Note: auth.User is the unified user table, supporting both password and OAuth login
	if err := database.AutoMigrate(
		&auth.User{},                            // 统一用户表（支持密码认证和 OAuth 认证）/ Unified user table
		&auth.CLIToken{},                        // CLI 令牌表 / CLI token table
		&host.Host{},                            // 主机管理表 / Host management table
		&cluster.Cluster{},                      // 集群表 / Cluster table
		&cluster.ClusterNode{},                  // 集群节点表 / Cluster node table
		&audit.CommandLog{},                     // 命令日志表 / Command log table
		&audit.AuditLog{},                       // 审计日志表 / Audit log table
		&execution.Execution{},                  // 公共执行记录表 / Shared execution record table
		&execution.Confirmation{},               // 一次性操作确认表 / One-time operation confirmation table
		&plugin.InstalledPlugin{},               // 已安装插件表 / Installed plugin table
		&plugin.PluginDependencyConfig{},        // 插件依赖配置表 / Plugin dependency config table
		&plugin.PluginDependencyDisable{},       // 插件官方依赖禁用表 / Plugin official dependency disable table
		&plugin.PluginCatalogEntry{},            // 插件目录表 / Plugin catalog table
		&plugin.PluginDependencyProfile{},       // 插件官方依赖画像表 / Plugin official dependency profile table
		&plugin.PluginDependencyProfileItem{},   // 插件官方依赖画像子项表 / Plugin official dependency profile item table
		&appconfig.Config{},                     // 配置文件表 / Config file table
		&appconfig.ConfigVersion{},              // 配置版本表 / Config version table
		&monitor.MonitorConfig{},                // 监控配置表 / Monitor config table (Requirements: 5.2)
		&monitor.ProcessEvent{},                 // 进程事件表 / Process event table (Requirements: 6.1)
		&monitoringapp.AlertRule{},              // 监控告警规则表 / Monitoring alert rule table
		&monitoringapp.AlertPolicy{},            // 统一告警策略表 / Unified alert policy table
		&monitoringapp.AlertEventState{},        // 告警事件状态表 / Alert event state table
		&monitoringapp.AlertState{},             // 统一告警状态表 / Unified alert state table
		&monitoringapp.NotificationChannel{},    // 通知渠道表 / Notification channel table
		&monitoringapp.NotificationRoute{},      // 通知路由表 / Notification route table
		&monitoringapp.NotificationDelivery{},   // 通知投递记录表 / Notification delivery table
		&monitoringapp.RemoteAlertRecord{},      // 远程告警记录表 / Remote alert record table
		&diagnostics.SeatunnelErrorGroup{},      // 诊断错误组表 / Diagnostics error group table
		&diagnostics.SeatunnelErrorEvent{},      // 诊断错误事件表 / Diagnostics error event table
		&diagnostics.SeatunnelLogCursor{},       // 诊断日志游标表 / Diagnostics log cursor table
		&diagnostics.ClusterInspectionReport{},  // 诊断巡检报告表 / Diagnostics inspection report table
		&diagnostics.ClusterInspectionFinding{}, // 诊断巡检发现项表 / Diagnostics inspection finding table
		&diagnostics.DiagnosticTask{},           // 诊断任务表 / Diagnostics task table
		&diagnostics.DiagnosticTaskStep{},       // 诊断任务步骤表 / Diagnostics task step table
		&diagnostics.DiagnosticNodeExecution{},  // 诊断任务节点执行表 / Diagnostics node execution table
		&diagnostics.DiagnosticStepLog{},        // 诊断任务日志表 / Diagnostics task log table
		&diagnostics.InspectionAutoPolicy{},     // 诊断自动巡检策略表 / Diagnostics auto-inspection policy table
		&diagnostics.TroubleshootingMemory{},    // 诊断排障经验记忆库表 / Diagnostics troubleshooting memory table
		&stupgrade.UpgradePlanRecord{},          // SeaTunnel 升级计划表 / SeaTunnel upgrade plan table
		&stupgrade.UpgradeTask{},                // SeaTunnel 升级任务表 / SeaTunnel upgrade task table
		&stupgrade.UpgradeTaskStep{},            // SeaTunnel 升级步骤表 / SeaTunnel upgrade step table
		&stupgrade.UpgradeNodeExecution{},       // SeaTunnel 升级节点执行表 / SeaTunnel upgrade node execution table
		&stupgrade.UpgradeStepLog{},             // SeaTunnel 升级日志表 / SeaTunnel upgrade log table
		&syncapp.Task{},                         // 数据同步任务表 / Sync task table
		&syncapp.TaskVersion{},                  // 数据同步任务版本表 / Sync task version table
		&syncapp.JobInstance{},                  // 数据同步作业实例表 / Sync job instance table
		&syncapp.GlobalVariable{},               // 数据同步全局变量表 / Sync global variable table
		&syncapp.CuratedTemplate{},              // 数据同步精选模板表 / Sync curated template table
		&syncapp.PreviewSession{},               // 数据同步预览会话表 / Sync preview session table
		&syncapp.PreviewTable{},                 // 数据同步预览表分组表 / Sync preview table table
		&syncapp.PreviewRow{},                   // 数据同步预览数据行表 / Sync preview row table
	); err != nil {
		return fmt.Errorf("auto migrate failed: %w", err)
	}
	log.Printf("[Database] auto migrate success\n")

	upgradeMigrator := database.Migrator()
	if upgradeMigrator.HasIndex(&stupgrade.UpgradeTaskStep{}, "idx_st_upgrade_task_step_code") {
		if err := upgradeMigrator.DropIndex(&stupgrade.UpgradeTaskStep{}, "idx_st_upgrade_task_step_code"); err != nil {
			log.Printf("[Database] failed to recreate st upgrade task step index: %v\n", err)
		}
	}
	if err := upgradeMigrator.CreateIndex(&stupgrade.UpgradeTaskStep{}, "idx_st_upgrade_task_step_code"); err != nil {
		log.Printf("[Database] failed to create st upgrade task step index: %v\n", err)
	}
	if upgradeMigrator.HasIndex(&plugin.PluginDependencyConfig{}, "idx_plugin_dep") {
		if err := upgradeMigrator.DropIndex(&plugin.PluginDependencyConfig{}, "idx_plugin_dep"); err != nil {
			log.Printf("[Database] failed to recreate plugin dependency index: %v\n", err)
		}
	}
	if err := upgradeMigrator.CreateIndex(&plugin.PluginDependencyConfig{}, "idx_plugin_dep"); err != nil {
		log.Printf("[Database] failed to create plugin dependency index: %v\n", err)
	}
	if upgradeMigrator.HasIndex(&plugin.PluginDependencyDisable{}, "idx_plugin_dep_disable") {
		if err := upgradeMigrator.DropIndex(&plugin.PluginDependencyDisable{}, "idx_plugin_dep_disable"); err != nil {
			log.Printf("[Database] failed to recreate plugin dependency disable index: %v\n", err)
		}
	}
	if err := upgradeMigrator.CreateIndex(&plugin.PluginDependencyDisable{}, "idx_plugin_dep_disable"); err != nil {
		log.Printf("[Database] failed to create plugin dependency disable index: %v\n", err)
	}

	// 初始化默认管理员用户 / Initialize default admin user
	if err := InitDefaultAdminUserWithDB(database); err != nil {
		log.Printf("[Database] 初始化默认管理员用户失败: %v\n", err)
	}

	// 播种预置排障经验库（若排障经验表为空）/ Seed preset troubleshooting memories (if table is empty)
	troubleshootingRepo := diagnostics.NewTroubleshootingRepository(database)
	if err := troubleshootingRepo.SeedPresetMemories(context.Background()); err != nil {
		log.Printf("[Database] 播种预置排障经验库失败 / Failed to seed preset troubleshooting memories: %v\n", err)
	}

	// 创建存储过程（仅 MySQL 支持）/ Create stored procedures (MySQL only)
	if dbType == "mysql" {
		if err := createStoredProceduresWithDB(database); err != nil {
			log.Printf("[Database] create stored procedures failed (may not be supported): %v\n", err)
		}
	} else {
		log.Printf("[Database] 跳过存储过程创建（当前数据库类型: %s）\n", dbType)
	}

	return nil
}

// InitDefaultAdminUserWithDB 使用指定的数据库实例初始化默认管理员用户
// InitDefaultAdminUserWithDB initializes the default admin user with the specified database instance
// 仅在首次启动时（用户表为空）创建默认 admin 用户
// Only creates default admin user when user table is empty on first boot
// Requirements: 2.1, 2.2
func InitDefaultAdminUserWithDB(database *gorm.DB) error {
	if database == nil {
		return errors.New("数据库连接未初始化 / database connection is nil")
	}

	// 获取认证配置 / Get authentication configuration
	authConfig := config.GetAuthConfig()

	// 检查是否已存在用户 / Check if any user exists
	var count int64
	if err := database.Model(&auth.User{}).Count(&count).Error; err != nil {
		return err
	}

	// 如果已有用户，跳过初始化 / If users already exist, skip initialization
	if count > 0 {
		log.Println("[Database] 用户表已有数据，跳过默认管理员初始化")
		return nil
	}

	// 检查是否已存在 admin 用户（双重检查）/ Check if admin user already exists (double check)
	var existingUser auth.User
	err := database.Where("username = ?", authConfig.DefaultAdminUsername).First(&existingUser).Error
	if err == nil {
		log.Printf("[Database] 管理员用户 '%s' 已存在，跳过创建\n", authConfig.DefaultAdminUsername)
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	// 创建默认管理员用户 / Create default admin user
	adminUser := &auth.User{
		Username: authConfig.DefaultAdminUsername,
		Nickname: "系统管理员",
		IsActive: true,
		IsAdmin:  true,
	}

	// 设置密码（使用 bcrypt 哈希）/ Set password (hashed using bcrypt)
	if err := adminUser.SetPassword(authConfig.DefaultAdminPassword, authConfig.BcryptCost); err != nil {
		return err
	}

	// 保存到数据库 / Save to database
	if err := adminUser.Create(database); err != nil {
		return err
	}

	log.Printf("[Database] 成功创建默认管理员用户: %s\n", authConfig.DefaultAdminUsername)
	return nil
}

// initDefaultAdminUser 初始化默认管理员用户（使用全局数据库）
// initDefaultAdminUser initializes the default admin user using the global database
func initDefaultAdminUser() error {
	return InitDefaultAdminUserWithDB(db.GetDB(context.Background()))
}

// createStoredProceduresWithDB 创建存储过程（使用指定数据库）
// createStoredProceduresWithDB creates stored procedures using the specified database
func createStoredProceduresWithDB(database *gorm.DB) error {
	if database == nil {
		return errors.New("database instance is nil")
	}

	// 读取SQL文件（支持相对路径与项目根路径定位）
	// Read SQL file (supports relative path and project root location)
	sqlFile := "support-files/sql/create_dashboard_proc.sql"
	if _, err := os.Stat(sqlFile); os.IsNotExist(err) {
		// 如果相对路径不存在则尝试寻找，若不存在直接跳过
		// If relative path does not exist, skip safely
		return nil
	}
	content, err := os.ReadFile(sqlFile)
	if err != nil {
		return err
	}

	// 处理SQL脚本，替换DELIMITER并分割成单独的语句
	// Process SQL script, replace DELIMITER and split into individual statements
	sqlContent := string(content)
	// 移除DELIMITER声明行 / Remove DELIMITER declarations
	sqlContent = strings.Replace(sqlContent, "DELIMITER $$", "", -1)
	sqlContent = strings.Replace(sqlContent, "DELIMITER ;", "", -1)
	sqlContent = strings.Replace(sqlContent, "$$", ";", -1)

	// 执行SQL语句 / Execute SQL statement
	return database.Exec(sqlContent).Error
}

// createStoredProcedures 创建存储过程（使用全局数据库）
// createStoredProcedures creates stored procedures using the global database
func createStoredProcedures() error {
	return createStoredProceduresWithDB(db.GetDB(context.Background()))
}
