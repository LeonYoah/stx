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

import {BaseService} from '../core/base.service';
import type {
  TroubleshootingMemoryEntry,
  TroubleshootingMemoryQuery,
} from './types';

// 本地存储键名
// LocalStorage key for persisted troubleshooting memories
const STORAGE_KEY = 'stx_troubleshooting_memories';

// 系统预置典型 SeaTunnel 排障经验库
// Preset classic troubleshooting solutions for common SeaTunnel errors and alerts
const PRESET_MEMORIES: TroubleshootingMemoryEntry[] = [
  {
    id: 'preset-mysql-link-failure',
    target_type: 'error',
    fingerprint: 'Communications link failure',
    title: 'MySQL 连接断开 Communications link failure 恢复方案',
    error_summary:
      'The last packet successfully received from the server was 30,000 milliseconds ago. The last packet sent successfully to the server was 30,000 milliseconds ago.',
    root_cause:
      'MySQL 服务端 wait_timeout/interactive_timeout 超时导致空闲连接被主动切断，或网络波动中断了长连接。',
    solution:
      '1. 在 SeaTunnel JDBC 连接串中增加参数：autoReconnect=true&failOverReadOnly=false&maxReconnects=5&connectTimeout=30000&socketTimeout=60000\n2. 检查 MySQL 端的 wait_timeout 与 interactive_timeout 参数，建议调整至 28800 秒以上。\n3. 在作业配置的 source/sink 中增加心跳测试探针参数 connection-check-timeout-sec: 30。',
    actions_taken: [
      '在 JDBC URL 末尾追加 autoReconnect=true',
      '调大 MySQL 服务端 wait_timeout 至 28800',
    ],
    preventive_tips:
      '对于大批量流式同步作业，请务必开启 JDBC 连接池的心跳保活（testWhileIdle）以防长空闲被切断。',
    tags: ['mysql', 'jdbc', 'timeout', 'network'],
    author: 'SeaTunnelX 预置经验库',
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    is_preset: true,
  },
  {
    id: 'preset-slot-exhaustion',
    target_type: 'error',
    fingerprint: 'SlotNotEnoughException',
    title: 'Worker 节点 Slot 耗尽 (SlotNotEnoughException) 恢复方案',
    error_summary:
      'org.apache.seatunnel.engine.common.exception.SeaTunnelEngineException: No enough slots for task, required: 4, available: 0',
    root_cause:
      '提交的任务并行度总和超过了当前集群全部处于 ACTIVE 状态的 Worker 节点空闲槽位总和。',
    solution:
      '1. 快速应急：在「集群管理 -> 节点列表」中水平新增部署 1~2 台 Worker 节点。\n2. 临时恢复：如果无法立即扩容，修改作业配置 seatunnel.job.parallelism 降低作业并行度，使其与现有 Slot 数量适配。\n3. 检查是否有僵死作业未释放资源：在运行作业列表中排查并取消异常悬挂的任务。',
    actions_taken: [
      '扩容集群新增 Worker 节点',
      '将作业并行度从 8 调整为 4',
    ],
    preventive_tips:
      '日常运维建议保留集群 20%~30% 的 Slot 资源余量，避免多个调度任务并发堆叠导致资源挤兑。',
    tags: ['slot', 'resource', 'worker', 'parallelism'],
    author: 'SeaTunnelX 预置经验库',
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    is_preset: true,
  },
  {
    id: 'preset-jvm-heap-oom',
    target_type: 'error',
    fingerprint: 'OutOfMemoryError: Java heap space',
    title: 'JVM 堆内存溢出 (Java heap space) 恢复方案',
    error_summary:
      'java.lang.OutOfMemoryError: Java heap space during transform batch buffer execution',
    root_cause:
      'SeaTunnel 节点 JVM -Xmx 堆上限过低，或批处理/微批切片过大导致内存缓冲队列积压打满。',
    solution:
      '1. 调大 JVM 堆大小：编辑节点 conf/seatunnel-env.sh，将 JVM_ARGS 中的 -Xms/-Xmx 从默认 2G 提升至 8G（视物理机内存大小而定）。\n2. 限制读写批量批次大小：在 Source 与 Sink 的配置中调小 batch.size 或 fetch.size（如从 50000 降至 5000）。\n3. 重启 Worker 进程使新的 JVM 参数生效。',
    actions_taken: [
      '修改 seatunnel-env.sh: export ST_JVM_ARGS="-Xms8g -Xmx8g"',
      '调小作业 batch.size: 5000',
    ],
    preventive_tips:
      '定期在监控中心关注各节点的 Heap 内存利用率，配置 >85% 告警以便提早拦截。',
    tags: ['jvm', 'oom', 'heap', 'memory'],
    author: 'SeaTunnelX 预置经验库',
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    is_preset: true,
  },
  {
    id: 'preset-checkpoint-timeout',
    target_type: 'error',
    fingerprint: 'CheckpointTimeoutException',
    title: '流作业 Checkpoint 超时失败排障方案',
    error_summary:
      'org.apache.seatunnel.engine.server.checkpoint.CheckpointTimeoutException: Checkpoint 124 expired before completing',
    root_cause:
      '外部持久化存储（如 IMAP 外部存储或 HDFS/S3）写入过慢，或下游 Sink 端反压严重阻塞了 Barrier 对齐。',
    solution:
      '1. 检查底层外部存储网络延时与 IOPS 瓶颈，如果是纯内存模式则考虑调大 checkpoint.timeout。\n2. 在 seatunnel.yaml 中调大 checkpoint 超时时长：checkpoint.timeout: 120000（从 30s 提高至 120s）。\n3. 检查并调大 checkpoint 间隔时间：checkpoint.interval: 30000，减轻高频打点的 IO 压力。',
    actions_taken: [
      '在 seatunnel.yaml 增加 checkpoint.timeout: 120000',
      '调大 checkpoint.interval 降低频次',
    ],
    preventive_tips:
      '若为纯离线 Batch 批处理任务，强烈建议关闭外部存储 IMAP，消除外部 I/O 损耗并加快作业执行。',
    tags: ['checkpoint', 'timeout', 'imap', 'storage'],
    author: 'SeaTunnelX 预置经验库',
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    is_preset: true,
  },
  {
    id: 'preset-high-cpu-alert',
    target_type: 'alert',
    fingerprint: 'HighCpuUsage',
    title: '节点 CPU 持续过高告警排查与处理',
    error_summary:
      'Node CPU usage exceeds 90% for more than 5 minutes',
    root_cause:
      '单节点运行过多重型数据转换（如复杂的正则、JSON 解析）任务，或 JVM 频繁 FullGC 消耗 CPU。',
    solution:
      '1. 登录该主机运行 jstack <pid> 查看是否存在频繁垃圾回收线程或密集死循环代码。\n2. 检查节点上是否有多个大作业并发调度，对任务执行分时错峰。\n3. 如果是常态化计算量大，进行集群扩容并将任务通过 dynamic-slot 机制分散。',
    actions_taken: [
      '执行 top -H -p <pid> 定位耗 CPU 线程',
      '调整定时调度任务执行时间错开高峰',
    ],
    preventive_tips:
      '大吞吐转换作业建议在 Source 侧前置过滤无关字段，减少无效数据在内存流转与序列化计算。',
    tags: ['alert', 'cpu', 'fullgc', 'performance'],
    author: 'SeaTunnelX 预置经验库',
    created_at: '2026-01-01T00:00:00.000Z',
    updated_at: '2026-01-01T00:00:00.000Z',
    is_preset: true,
  },
];

/**
 * 排障经验记忆库服务类
 * Troubleshooting memory bank service
 */
export class TroubleshootingService extends BaseService {
  protected static readonly basePath = '/diagnostics/troubleshooting-memories';

  /**
   * 异步从后端 API 读取排障经验列表（失败时优雅回退本地与预置库）
   * Asynchronously fetch troubleshooting memories from backend API (gracefully falls back to local and presets)
   */
  public async fetchRemoteMemories(
    query?: TroubleshootingMemoryQuery,
  ): Promise<TroubleshootingMemoryEntry[]> {
    try {
      const response = await TroubleshootingService.get<{total: number; items: TroubleshootingMemoryEntry[]}>(
        '',
        query as Record<string, unknown>,
      );
      if (response && Array.isArray(response.items) && response.items.length > 0) {
        // 同步更新本地缓存以供离线兜底
        // Sync to local cache for offline fallback
        if (typeof window !== 'undefined') {
          try {
            localStorage.setItem(STORAGE_KEY, JSON.stringify(response.items));
          } catch {
            // Ignore storage write errors
          }
        }
        return response.items;
      }
      return this.getMemories();
    } catch {
      return this.getMemories();
    }
  }

  /**
   * 异步保存排障经验至后端数据库与本地缓存
   * Asynchronously persist troubleshooting memory to backend database and local cache
   */
  public async saveRemoteMemory(
    entry: Omit<TroubleshootingMemoryEntry, 'id' | 'created_at' | 'updated_at'> & {
      id?: string;
    },
  ): Promise<TroubleshootingMemoryEntry> {
    const localSaved = this.saveMemory(entry);
    try {
      if (entry.id && !entry.id.startsWith('mem-') && !entry.id.startsWith('preset-')) {
        await TroubleshootingService.put(`/${entry.id}`, entry);
      } else {
        await TroubleshootingService.post('', entry);
      }
    } catch (err) {
      console.warn('Failed to sync troubleshooting memory to backend, kept in local cache:', err);
    }
    return localSaved;
  }

  /**
   * 异步删除排障经验
   * Asynchronously delete troubleshooting memory from backend and local cache
   */
  public async deleteRemoteMemory(id: string): Promise<boolean> {
    this.deleteMemory(id);
    try {
      await TroubleshootingService.delete(`/${id}`);
      return true;
    } catch (err) {
      console.warn('Failed to delete troubleshooting memory from backend:', err);
      return true;
    }
  }

  /**
   * 读取全部存储的排障经验（合并预置库与本地用户沉淀）
   * Read all stored troubleshooting memories (merges presets and local memories)
   */
  public getMemories(): TroubleshootingMemoryEntry[] {
    if (typeof window === 'undefined') {
      return [...PRESET_MEMORIES];
    }
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) {
        return [...PRESET_MEMORIES];
      }
      const localEntries: TroubleshootingMemoryEntry[] = JSON.parse(raw);
      // 预置方案排在后面，用户沉淀的经验置顶
      // User-created memories placed first, followed by preset classic cases
      const localIds = new Set(localEntries.map((e) => e.id));
      const remainingPresets = PRESET_MEMORIES.filter((p) => !localIds.has(p.id));
      return [...localEntries, ...remainingPresets];
    } catch {
      return [...PRESET_MEMORIES];
    }
  }

  /**
   * 保存或更新排障经验
   * Save or update troubleshooting memory entry
   */
  public saveMemory(
    entry: Omit<TroubleshootingMemoryEntry, 'id' | 'created_at' | 'updated_at'> & {
      id?: string;
    },
  ): TroubleshootingMemoryEntry {
    const trimmedSolution = entry.solution?.trim();
    if (!trimmedSolution) {
      throw new Error('解决方案内容不能为空，排障经验必须包含具体的处理措施与步骤');
    }

    const now = new Date().toISOString();
    const id = entry.id || `mem-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const newEntry: TroubleshootingMemoryEntry = {
      ...entry,
      id,
      solution: trimmedSolution,
      title: entry.title.trim() || '未命名排障方案',
      error_summary: entry.error_summary?.trim() || '',
      root_cause: entry.root_cause?.trim(),
      preventive_tips: entry.preventive_tips?.trim(),
      author: entry.author?.trim() || '运维工程师',
      tags: entry.tags || [],
      created_at: now,
      updated_at: now,
      is_preset: false,
    };

    if (typeof window !== 'undefined') {
      try {
        const raw = localStorage.getItem(STORAGE_KEY);
        const list: TroubleshootingMemoryEntry[] = raw ? JSON.parse(raw) : [];
        const index = list.findIndex((item) => item.id === id);
        if (index >= 0) {
          newEntry.created_at = list[index].created_at;
          list[index] = newEntry;
        } else {
          list.unshift(newEntry);
        }
        localStorage.setItem(STORAGE_KEY, JSON.stringify(list));
      } catch (err) {
        console.error('Failed to persist troubleshooting memory to localStorage:', err);
      }
    }

    return newEntry;
  }

  /**
   * 删除排障经验
   * Delete troubleshooting memory entry
   */
  public deleteMemory(id: string): boolean {
    if (typeof window === 'undefined') {
      return false;
    }
    try {
      const raw = localStorage.getItem(STORAGE_KEY);
      if (!raw) {
        return false;
      }
      const list: TroubleshootingMemoryEntry[] = JSON.parse(raw);
      const filtered = list.filter((item) => item.id !== id);
      localStorage.setItem(STORAGE_KEY, JSON.stringify(filtered));
      return true;
    } catch {
      return false;
    }
  }

  /**
   * 根据指纹与关键字匹配相关排障经验
   * Match relevant troubleshooting memories based on fingerprint and keywords
   */
  public findMatchingMemories(
    query: TroubleshootingMemoryQuery,
  ): TroubleshootingMemoryEntry[] {
    const all = this.getMemories();
    const {fingerprint, exception_class, title, target_type} = query;

    const normalizedFingerprint = fingerprint?.trim().toLowerCase();
    const normalizedException = exception_class?.trim().toLowerCase();
    const normalizedTitle = title?.trim().toLowerCase();

    // 计算匹配相关度打分
    // Calculate match relevance score
    const scored = all.map((entry) => {
      let score = 0;
      const entryFp = entry.fingerprint.toLowerCase();
      const entryTitle = entry.title.toLowerCase();
      const entrySummary = entry.error_summary.toLowerCase();
      const entryTags = entry.tags.map((t) => t.toLowerCase());

      // 类型匹配过滤
      // Target type filtering
      if (target_type && entry.target_type !== target_type) {
        return {entry, score: 0};
      }

      // 1. 精确或包含指纹匹配 (最高权重)
      // 1. Exact or substring fingerprint match (highest weight)
      if (normalizedFingerprint) {
        if (entryFp === normalizedFingerprint) {
          score += 100;
        } else if (
          normalizedFingerprint.includes(entryFp) ||
          entryFp.includes(normalizedFingerprint)
        ) {
          score += 60;
        }
      }

      // 2. 异常类名匹配
      // 2. Exception class match
      if (normalizedException) {
        if (entryFp.includes(normalizedException) || entrySummary.includes(normalizedException)) {
          score += 40;
        }
        if (entryTags.some((tag) => normalizedException.includes(tag))) {
          score += 20;
        }
      }

      // 3. 标题或文本匹配
      // 3. Title or text keyword match
      if (normalizedTitle) {
        if (entryTitle.includes(normalizedTitle) || entryFp.includes(normalizedTitle)) {
          score += 30;
        }
        if (entryTags.some((tag) => normalizedTitle.includes(tag))) {
          score += 15;
        }
      }

      return {entry, score};
    });

    // 过滤出有匹配度（score > 0）的记录，并按得分与更新时间倒序排序
    // Filter out matched records (score > 0) and sort by score & updated time descending
    return scored
      .filter((item) => item.score > 0)
      .sort((a, b) => {
        if (b.score !== a.score) {
          return b.score - a.score;
        }
        return new Date(b.entry.updated_at).getTime() - new Date(a.entry.updated_at).getTime();
      })
      .map((item) => item.entry);
  }

  /**
   * 获取单条排障经验详情
   * Get single troubleshooting memory by ID
   */
  public getMemoryById(id: string): TroubleshootingMemoryEntry | null {
    const all = this.getMemories();
    return all.find((item) => item.id === id) || null;
  }
}

// 导出单例实例
// Export singleton instance
export const troubleshootingService = new TroubleshootingService();
