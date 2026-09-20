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
		if lang := strings.TrimSpace(query.Language); lang != "" {
			langNorm := "zh"
			if strings.HasPrefix(strings.ToLower(lang), "en") {
				langNorm = "en"
			}
			// 预置方案按语言严格筛选（跟随全局语言切分），用户自定义方案不受语言限制全量展示
			// Preset memories are strictly filtered by language (following global language), while custom entries are visible across all languages
			dbQuery = dbQuery.Where("(is_preset = ? AND (language = ? OR (language = '' AND ? = 'zh'))) OR (is_preset = ?)", true, langNorm, langNorm, false)
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

// SeedPresetMemories 在数据库中播种或更新经典官方中英双语排障方案（中英文整行记录持久化）
// SeedPresetMemories populates or updates preset classic troubleshooting cases with bilingual full-row support
func (r *TroubleshootingRepository) SeedPresetMemories(ctx context.Context) error {
	if r.db == nil {
		return nil
	}

	now := time.Now()
	presets := []TroubleshootingMemory{
		// 1. MySQL 连接中断 - 中文
		// 1. MySQL Connection Closed - Chinese
		{
			TargetType:     "error",
			Fingerprint:    "Communications link failure",
			PresetKey:      "mysql_connection_failure",
			Language:       "zh",
			Title:          "MySQL 连接断开 Communications link failure 恢复方案",
			ErrorSummary:   "The last packet successfully received from the server was 30,000 milliseconds ago. The last packet sent successfully to the server was 30,000 milliseconds ago.",
			RootCause:      "MySQL 服务端 wait_timeout/interactive_timeout 超时导致空闲连接被主动切断，或网络波动中断了长连接。",
			Solution:       "1. 在 SeaTunnel JDBC 连接串中增加参数：autoReconnect=true&failOverReadOnly=false&maxReconnects=5&connectTimeout=30000&socketTimeout=60000\n2. 检查 MySQL 端的 wait_timeout 与 interactive_timeout 参数，建议调整至 28800 秒以上。\n3. 在作业配置的 source/sink 中增加心跳测试探针参数 connection-check-timeout-sec: 30。",
			ActionsTaken:   `["在 JDBC URL 末尾追加 autoReconnect=true","调大 MySQL 服务端 wait_timeout 至 28800"]`,
			PreventiveTips: "对于大批量流式同步作业，请务必开启 JDBC 连接池的心跳保活（testWhileIdle）以防长空闲被切断。",
			Tags:           `["mysql","jdbc","timeout","network"]`,
			Author:         "STX预置经验库",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		// 1. MySQL Connection Closed - English
		// 1. MySQL 连接中断 - 英文
		{
			TargetType:     "error",
			Fingerprint:    "Communications link failure",
			PresetKey:      "mysql_connection_failure",
			Language:       "en",
			Title:          "MySQL Connection Closed (Communications link failure) Recovery",
			ErrorSummary:   "The last packet successfully received from the server was 30,000 milliseconds ago. The last packet sent successfully to the server was 30,000 milliseconds ago.",
			RootCause:      "MySQL server wait_timeout or interactive_timeout expired causing idle connections to be closed, or transient network fluctuation interrupted the persistent link.",
			Solution:       "1. Append timeout and auto-reconnect parameters to JDBC URL: autoReconnect=true&failOverReadOnly=false&maxReconnects=5&connectTimeout=30000&socketTimeout=60000\n2. Inspect MySQL server wait_timeout and interactive_timeout settings, recommended to be at least 28800 seconds.\n3. Add connection heartbeat check probe in job source/sink configuration: connection-check-timeout-sec: 30.",
			ActionsTaken:   `["Append autoReconnect=true to JDBC URL","Increase MySQL server wait_timeout to 28800"]`,
			PreventiveTips: "For high-throughput streaming sync jobs, enable JDBC connection pool heartbeat keepalive (testWhileIdle) to avoid idle drops.",
			Tags:           `["mysql","jdbc","timeout","network"]`,
			Author:         "STX Preset Knowledge Base",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		// 2. Worker 槽位耗尽 - 中文
		// 2. Worker Slot Exhaustion - Chinese
		{
			TargetType:     "error",
			Fingerprint:    "SlotNotEnoughException",
			PresetKey:      "slot_not_enough",
			Language:       "zh",
			Title:          "Worker 节点 Slot 耗尽 (SlotNotEnoughException) 恢复方案",
			ErrorSummary:   "org.apache.seatunnel.engine.common.exception.SeaTunnelEngineException: No enough slots for task, required: 4, available: 0",
			RootCause:      "提交的任务并行度总和超过了当前集群全部处于 ACTIVE 状态的 Worker 节点空闲槽位总和。",
			Solution:       "1. 快速应急：在「集群管理 -> 节点列表」中水平新增部署 1~2 台 Worker 节点。\n2. 如果资源足够，可以建议开启动态 slot 机制。\n3. 检查是否有僵死作业未释放资源：在运行作业列表中排查并取消异常悬挂的任务。",
			ActionsTaken:   `["扩容集群新增 Worker 节点","将作业并行度从 8 调整为 4"]`,
			PreventiveTips: "日常运维建议保留集群 20%~30% 的 Slot 资源余量，避免多个调度任务并发堆叠导致资源挤兑。",
			Tags:           `["slot","resource","worker","parallelism"]`,
			Author:         "STX预置经验库",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		// 2. Worker Slot Exhaustion - English
		// 2. Worker 槽位耗尽 - 英文
		{
			TargetType:     "error",
			Fingerprint:    "SlotNotEnoughException",
			PresetKey:      "slot_not_enough",
			Language:       "en",
			Title:          "Worker Slot Exhaustion (SlotNotEnoughException) Recovery",
			ErrorSummary:   "org.apache.seatunnel.engine.common.exception.SeaTunnelEngineException: No enough slots for task, required: 4, available: 0",
			RootCause:      "The total parallelism of submitted jobs exceeds the sum of available idle slots on all ACTIVE worker nodes in the cluster.",
			Solution:       "1. Emergency scaling: Add 1-2 new Worker nodes under \"Cluster Management -> Node List\".\n2. Consider enabling dynamic-slot allocation if cluster resource headroom is adequate.\n3. Cancel hanging or zombie tasks in Running Jobs list to release occupied slots.",
			ActionsTaken:   `["Scale out cluster with new Worker nodes","Reduce job parallelism from 8 to 4"]`,
			PreventiveTips: "Reserve 20%-30% slot headroom in daily operations to prevent concurrent scheduling peaks from exhausting slot capacity.",
			Tags:           `["slot","resource","worker","parallelism"]`,
			Author:         "STX Preset Knowledge Base",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		// 3. 流作业 Checkpoint 超时 - 中文
		// 3. Streaming Checkpoint Timeout - Chinese
		{
			TargetType:     "error",
			Fingerprint:    "CheckpointTimeoutException",
			PresetKey:      "checkpoint_timeout",
			Language:       "zh",
			Title:          "流作业 Checkpoint 超时失败排障方案",
			ErrorSummary:   "org.apache.seatunnel.engine.server.checkpoint.CheckpointTimeoutException: Checkpoint 124 expired before completing",
			RootCause:      "1. 下游 Sink 端写入过慢或目标存储负载过高，导致数据反压阻塞、Barrier 迟迟无法对齐完成；\n2. checkpoint.timeout 超时时间设置过短或 checkpoint.interval 过于频繁；\n3. 底层外部持久化存储（如 HDFS/S3/OSS/IMAP 存储）高延迟或网络波动。",
			Solution:       "1. 调大 Checkpoint 超时时间：在 seatunnel.yaml 中调大 checkpoint 超时时长：checkpoint.timeout: 120000（从默认 30s 提高至 120s），并可适当增大 checkpoint.interval 减轻高频打点负载。\n2. 重点排查下游 Sink 写入瓶颈：检查 Sink 端的数据库/目标存储压力；调整写入参数（如调小 batch.size、增大 buffer/flush 间隔、优化目标表索引与写入并发），提升写入速度消除反压，确保 Barrier 快速对齐。\n3. 排查底层外部持久化存储网络延时与 IOPS 瓶颈；若为离线 Batch 批处理作业，建议关闭外部存储避免外部 I/O 损耗。",
			ActionsTaken:   `["在 seatunnel.yaml 或 seatunnel-env.sh 中调大 checkpoint.timeout: 120000","排查下游 Sink 数据库负载并优化写入批次与并发参数加速 Barrier 对齐","调大 checkpoint.interval 降低频次"]`,
			PreventiveTips: "运行长时间流作业（Streaming）时建议保持稳定吞吐并监控下游写入延时；批处理作业（Batch）建议关闭外部存储以避免外部 I/O 损耗。",
			Tags:           `["checkpoint","timeout","sink","barrier","storage"]`,
			Author:         "STX预置经验库",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		// 3. Streaming Checkpoint Timeout - English
		// 3. 流作业 Checkpoint 超时 - 英文
		{
			TargetType:     "error",
			Fingerprint:    "CheckpointTimeoutException",
			PresetKey:      "checkpoint_timeout",
			Language:       "en",
			Title:          "Streaming Checkpoint Timeout Exception Recovery",
			ErrorSummary:   "org.apache.seatunnel.engine.server.checkpoint.CheckpointTimeoutException: Checkpoint 124 expired before completing",
			RootCause:      "1. Slow sink ingestion or high target storage pressure causing backpressure and barrier alignment blockage;\n2. checkpoint.timeout configured too short or checkpoint.interval too frequent;\n3. High latency or IOPS bottleneck on underlying persistent storage (e.g. HDFS/S3/OSS/IMAP).",
			Solution:       "1. Increase Checkpoint Timeout: In seatunnel.yaml, increase checkpoint timeout to checkpoint.timeout: 120000 (from 30s to 120s), and consider enlarging checkpoint.interval to reduce checkpointing frequency.\n2. Troubleshoot Sink Ingestion Bottleneck: Inspect downstream database/storage load; tune sink batch parameters (e.g., lower batch.size, optimize buffer flush interval, tune indexing and write concurrency) to eliminate backpressure and accelerate barrier alignment.\n3. Verify Storage Latency and IOPS: Check network and I/O bottlenecks of underlying persistent storage; for offline batch jobs, disable external storage to avoid unnecessary I/O overhead.",
			ActionsTaken:   `["Increase checkpoint.timeout: 120000 in seatunnel.yaml","Tune downstream sink write parameters to accelerate barrier alignment","Enlarge checkpoint.interval to reduce checkpoint overhead"]`,
			PreventiveTips: "Maintain steady throughput and monitor downstream sink write latency for streaming jobs; disable external storage for batch jobs to eliminate I/O overhead.",
			Tags:           `["checkpoint","timeout","sink","barrier","storage"]`,
			Author:         "STX Preset Knowledge Base",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		// 4. 节点 CPU 持续过高 - 中文
		// 4. High CPU Usage - Chinese
		{
			TargetType:     "alert",
			Fingerprint:    "HighCpuUsage",
			PresetKey:      "high_cpu_usage",
			Language:       "zh",
			Title:          "节点 CPU 持续过高告警排查与处理",
			ErrorSummary:   "Node CPU usage exceeds 90% for more than 5 minutes",
			RootCause:      "单节点运行过多重型数据转换（如复杂的正则、JSON 解析）任务，或 JVM 频繁 FullGC 消耗 CPU。",
			Solution:       "1. 登录该主机运行 jstack <pid> 查看是否存在频繁垃圾回收线程或密集死循环代码。\n2. 检查节点上是否有多个大作业并发调度，对任务执行分时错峰。\n3. 如果是常态化计算量大，进行集群扩容并将任务通过 dynamic-slot 机制分散。",
			ActionsTaken:   `["执行 top -H -p <pid> 定位耗 CPU 线程","调整定时调度任务执行时间错开高峰"]`,
			PreventiveTips: "大吞吐转换作业建议在 Source 侧前置过滤无关字段，减少无效数据在内存流转与序列化计算。",
			Tags:           `["alert","cpu","fullgc","performance"]`,
			Author:         "STX预置经验库",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		// 4. High CPU Usage - English
		// 4. 节点 CPU 持续过高 - 英文
		{
			TargetType:     "alert",
			Fingerprint:    "HighCpuUsage",
			PresetKey:      "high_cpu_usage",
			Language:       "en",
			Title:          "Node High CPU Usage Sustained Alert Remediation",
			ErrorSummary:   "Node CPU usage exceeds 90% for more than 5 minutes",
			RootCause:      "Heavy transformation tasks (e.g. complex regex, JSON parsing) co-located on a single node, or frequent JVM FullGC consuming CPU.",
			Solution:       "1. Log in to the host and run jstack <pid> to check for active garbage collection threads or tight compute loops.\n2. Verify if multiple heavy tasks are running concurrently; stagger schedules across non-peak periods.\n3. If compute demand is permanently elevated, expand cluster capacity and leverage dynamic-slot distribution.",
			ActionsTaken:   `["Run top -H -p <pid> to identify high-CPU threads","Stagger scheduled tasks to avoid concurrency peaks"]`,
			PreventiveTips: "Filter out unused fields at source connectors to avoid redundant deserialization and memory overhead.",
			Tags:           `["alert","cpu","fullgc","performance"]`,
			Author:         "STX Preset Knowledge Base",
			IsPreset:       true,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
	}

	// 逐条插入或更新预置条目（按 fingerprint + language 唯一定位更新）
	// Upsert each preset entry (uniquely identified by fingerprint + language)
	for _, p := range presets {
		var existing TroubleshootingMemory
		err := r.db.WithContext(ctx).Where("is_preset = ? AND fingerprint = ? AND (language = ? OR (language = '' AND ? = 'zh'))", true, p.Fingerprint, p.Language, p.Language).First(&existing).Error
		if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
			if createErr := r.db.WithContext(ctx).Create(&p).Error; createErr != nil {
				return createErr
			}
		} else if err == nil {
			updates := map[string]interface{}{
				"preset_key":      p.PresetKey,
				"language":        p.Language,
				"title":           p.Title,
				"error_summary":   p.ErrorSummary,
				"root_cause":      p.RootCause,
				"solution":        p.Solution,
				"actions_taken":   p.ActionsTaken,
				"preventive_tips": p.PreventiveTips,
				"author":          p.Author,
				"target_type":     p.TargetType,
				"tags":            p.Tags,
				"updated_at":      now,
			}
			if updateErr := r.db.WithContext(ctx).Model(&TroubleshootingMemory{}).Where("id = ?", existing.ID).Updates(updates).Error; updateErr != nil {
				return updateErr
			}
		} else {
			return err
		}
	}

	return nil
}
