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

package diagnostics

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TroubleshootingRepository 为排障经验记忆库提供持久化仓储操作
// TroubleshootingRepository provides database persistence for troubleshooting memory entries
type TroubleshootingRepository struct {
	db *gorm.DB
}

// NewTroubleshootingRepository 创建排障经验仓储实例
// NewTroubleshootingRepository creates a new TroubleshootingRepository instance
func NewTroubleshootingRepository(db *gorm.DB) *TroubleshootingRepository {
	return &TroubleshootingRepository{db: db}
}

// Create 新增一条排障经验记录
// Create inserts a new troubleshooting memory entry
func (r *TroubleshootingRepository) Create(ctx context.Context, memory *TroubleshootingMemory) error {
	if r.db == nil {
		return errors.New("database connection is nil / 数据库连接未初始化")
	}
	return r.db.WithContext(ctx).Create(memory).Error
}

// Update 更新指定的排障经验记录
// Update modifies an existing troubleshooting memory entry by ID
func (r *TroubleshootingRepository) Update(ctx context.Context, id uint, memory *TroubleshootingMemory) error {
	if r.db == nil {
		return errors.New("database connection is nil / 数据库连接未初始化")
	}
	return r.db.WithContext(ctx).Model(&TroubleshootingMemory{}).Where("id = ?", id).Updates(memory).Error
}

// Delete 删除指定的排障经验记录
// Delete removes a troubleshooting memory entry by ID
func (r *TroubleshootingRepository) Delete(ctx context.Context, id uint) error {
	if r.db == nil {
		return errors.New("database connection is nil / 数据库连接未初始化")
	}
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&TroubleshootingMemory{}).Error
}

// GetByID 根据 ID 查询单条排障经验
// GetByID retrieves a single troubleshooting memory entry by ID
func (r *TroubleshootingRepository) GetByID(ctx context.Context, id uint) (*TroubleshootingMemory, error) {
	if r.db == nil {
		return nil, errors.New("database connection is nil / 数据库连接未初始化")
	}
	var memory TroubleshootingMemory
	err := r.db.WithContext(ctx).First(&memory, id).Error
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

// List 按照筛选条件检索排障经验列表
// List queries troubleshooting memory entries based on filter criteria
func (r *TroubleshootingRepository) List(ctx context.Context, query *TroubleshootingMemoryQuery) ([]*TroubleshootingMemory, int64, error) {
	if r.db == nil {
		return nil, 0, errors.New("database connection is nil / 数据库连接未初始化")
	}

	dbQuery := r.db.WithContext(ctx).Model(&TroubleshootingMemory{})

	if query != nil {
		if targetType := strings.TrimSpace(query.TargetType); targetType != "" && targetType != "all" {
			dbQuery = dbQuery.Where("target_type = ?", targetType)
		}
		if fp := strings.TrimSpace(query.Fingerprint); fp != "" {
			dbQuery = dbQuery.Where("fingerprint LIKE ? OR title LIKE ?", "%"+fp+"%", "%"+fp+"%")
		}
		if query.ClusterID != nil && *query.ClusterID > 0 {
			dbQuery = dbQuery.Where("cluster_id = ? OR cluster_id = 0", *query.ClusterID)
		}
		if kw := strings.TrimSpace(query.Keyword); kw != "" {
			like := "%" + kw + "%"
			dbQuery = dbQuery.Where("title LIKE ? OR error_summary LIKE ? OR tags LIKE ? OR solution LIKE ?", like, like, like, like)
		}
	}

	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 排序：用户沉淀经验优先（is_preset ASC），最近更新优先（updated_at DESC）
	// Ordering: user memories first (is_preset ASC), recently updated first (updated_at DESC)
	dbQuery = dbQuery.Order("is_preset ASC, updated_at DESC, id DESC")

	pageSize := 50
	if query != nil && query.PageSize > 0 {
		pageSize = query.PageSize
	}
	page := 1
	if query != nil && query.Page > 0 {
		page = query.Page
	}
	offset := (page - 1) * pageSize
	dbQuery = dbQuery.Offset(offset).Limit(pageSize)

	var items []*TroubleshootingMemory
	if err := dbQuery.Find(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// SeedPresetMemories 在数据库排障表为空时自动播种经典官方方案
// SeedPresetMemories populates initial preset troubleshooting cases if database table is empty
func (r *TroubleshootingRepository) SeedPresetMemories(ctx context.Context) error {
	if r.db == nil {
		return nil
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&TroubleshootingMemory{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now()
	presets := []TroubleshootingMemory{
		{
			TargetType:   "error",
			Fingerprint:  "Communications link failure",
			Title:        "MySQL 连接断开 Communications link failure 恢复方案",
			ErrorSummary: "The last packet successfully received from the server was 30,000 milliseconds ago. The last packet sent successfully to the server was 30,000 milliseconds ago.",
			RootCause:    "MySQL 服务端 wait_timeout/interactive_timeout 超时导致空闲连接被主动切断，或网络波动中断了长连接。",
			Solution:     "1. 在 SeaTunnel JDBC 连接串中增加参数：autoReconnect=true&failOverReadOnly=false&maxReconnects=5&connectTimeout=30000&socketTimeout=60000\n2. 检查 MySQL 端的 wait_timeout 与 interactive_timeout 参数，建议调整至 28800 秒以上。\n3. 在作业配置的 source/sink 中增加心跳测试探针参数 connection-check-timeout-sec: 30。",
			ActionsTaken: `["在 JDBC URL 末尾追加 autoReconnect=true","调大 MySQL 服务端 wait_timeout 至 28800"]`,
			PreventiveTips: "对于大批量流式同步作业，请务必开启 JDBC 连接池的心跳保活（testWhileIdle）以防长空闲被切断。",
			Tags:         `["mysql","jdbc","timeout","network"]`,
			Author:       "SeaTunnelX 预置经验库",
			IsPreset:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			TargetType:   "error",
			Fingerprint:  "SlotNotEnoughException",
			Title:        "Worker 节点 Slot 耗尽 (SlotNotEnoughException) 恢复方案",
			ErrorSummary: "org.apache.seatunnel.engine.common.exception.SeaTunnelEngineException: No enough slots for task, required: 4, available: 0",
			RootCause:    "提交的任务并行度总和超过了当前集群全部处于 ACTIVE 状态的 Worker 节点空闲槽位总和。",
			Solution:     "1. 快速应急：在「集群管理 -> 节点列表」中水平新增部署 1~2 台 Worker 节点。\n2. 临时恢复：如果无法立即扩容，修改作业配置 seatunnel.job.parallelism 降低作业并行度，使其与现有 Slot 数量适配。\n3. 检查是否有僵死作业未释放资源：在运行作业列表中排查并取消异常悬挂的任务。",
			ActionsTaken: `["扩容集群新增 Worker 节点","将作业并行度从 8 调整为 4"]`,
			PreventiveTips: "日常运维建议保留集群 20%~30% 的 Slot 资源余量，避免多个调度任务并发堆叠导致资源挤兑。",
			Tags:         `["slot","resource","worker","parallelism"]`,
			Author:       "SeaTunnelX 预置经验库",
			IsPreset:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			TargetType:   "error",
			Fingerprint:  "OutOfMemoryError: Java heap space",
			Title:        "JVM 堆内存溢出 (Java heap space) 恢复方案",
			ErrorSummary: "java.lang.OutOfMemoryError: Java heap space during transform batch buffer execution",
			RootCause:    "SeaTunnel 节点 JVM -Xmx 堆上限过低，或批处理/微批切片过大导致内存缓冲队列积压打满。",
			Solution:     "1. 调大 JVM 堆大小：编辑节点 conf/seatunnel-env.sh，将 JVM_ARGS 中的 -Xms/-Xmx 从默认 2G 提升至 8G（视物理机内存大小而定）。\n2. 限制读写批量批次大小：在 Source 与 Sink 的配置中调小 batch.size 或 fetch.size（如从 50000 降至 5000）。\n3. 重启 Worker 进程使新的 JVM 参数生效。",
			ActionsTaken: `["修改 seatunnel-env.sh: export ST_JVM_ARGS=\"-Xms8g -Xmx8g\"","调小作业 batch.size: 5000"]`,
			PreventiveTips: "定期在监控中心关注各节点的 Heap 内存利用率，配置 >85% 告警以便提早拦截。",
			Tags:         `["jvm","oom","heap","memory"]`,
			Author:       "SeaTunnelX 预置经验库",
			IsPreset:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			TargetType:   "error",
			Fingerprint:  "CheckpointTimeoutException",
			Title:        "流作业 Checkpoint 超时失败排障方案",
			ErrorSummary: "org.apache.seatunnel.engine.server.checkpoint.CheckpointTimeoutException: Checkpoint 124 expired before completing",
			RootCause:    "外部持久化存储（如 IMAP 外部存储或 HDFS/S3）写入过慢，或下游 Sink 端反压严重阻塞了 Barrier 对齐。",
			Solution:     "1. 检查底层外部存储网络延时与 IOPS 瓶颈，如果是纯内存模式则考虑调大 checkpoint.timeout。\n2. 在 seatunnel.yaml 中调大 checkpoint 超时时长：checkpoint.timeout: 120000（从 30s 提高至 120s）。\n3. 检查并调大 checkpoint 间隔时间：checkpoint.interval: 30000，减轻高频打点的 IO 压力。",
			ActionsTaken: `["在 seatunnel.yaml 增加 checkpoint.timeout: 120000","调大 checkpoint.interval 降低频次"]`,
			PreventiveTips: "若为纯离线 Batch 批处理任务，强烈建议关闭外部存储 IMAP，消除外部 I/O 损耗并加快作业执行。",
			Tags:         `["checkpoint","timeout","imap","storage"]`,
			Author:       "SeaTunnelX 预置经验库",
			IsPreset:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			TargetType:   "alert",
			Fingerprint:  "HighCpuUsage",
			Title:        "节点 CPU 持续过高告警排查与处理",
			ErrorSummary: "Node CPU usage exceeds 90% for more than 5 minutes",
			RootCause:    "单节点运行过多重型数据转换（如复杂的正则、JSON 解析）任务，或 JVM 频繁 FullGC 消耗 CPU。",
			Solution:     "1. 登录该主机运行 jstack <pid> 查看是否存在频繁垃圾回收线程或密集死循环代码。\n2. 检查节点上是否有多个大作业并发调度，对任务执行分时错峰。\n3. 如果是常态化计算量大，进行集群扩容并将任务通过 dynamic-slot 机制分散。",
			ActionsTaken: `["执行 top -H -p <pid> 定位耗 CPU 线程","调整定时调度任务执行时间错开高峰"]`,
			PreventiveTips: "大吞吐转换作业建议在 Source 侧前置过滤无关字段，减少无效数据在内存流转与序列化计算。",
			Tags:         `["alert","cpu","fullgc","performance"]`,
			Author:       "SeaTunnelX 预置经验库",
			IsPreset:     true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}

	return r.db.WithContext(ctx).Create(&presets).Error
}
