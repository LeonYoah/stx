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

import type {TroubleshootingMemoryEntry} from './types';

/**
 * 官方经典排障预置类型内容接口
 * Content contract for preset classic troubleshooting solutions
 */
export interface PresetLocalizationContent {
  title: string;
  root_cause: string;
  solution: string;
  actions_taken: string[];
  preventive_tips: string;
  author: string;
}

/**
 * 官方经典方案内容定义，跟随全局语言切分（zh / en）
 * Preset solutions partitioned by global language (zh / en)
 */
export const PRESET_LOCALIZATIONS: Record<
  'zh' | 'en',
  Record<string, PresetLocalizationContent>
> = {
  zh: {
    mysql_connection_failure: {
      title: 'MySQL 连接中断 (Communications link failure) 恢复方案',
      root_cause:
        'MySQL 服务端 wait_timeout 或 interactive_timeout 超时导致空闲连接被关闭，或者网络瞬时抖动导致长连接中断。',
      solution:
        '1. 在 SeaTunnel JDBC 连接串中增加参数：autoReconnect=true&failOverReadOnly=false&maxReconnects=5&connectTimeout=30000&socketTimeout=60000\n2. 检查 MySQL 端的 wait_timeout 与 interactive_timeout 参数，建议调整至 28800 秒以上。\n3. 在作业配置的 source/sink 中增加心跳测试探针参数 connection-check-timeout-sec: 30。',
      actions_taken: [
        '在 JDBC URL 末尾追加 autoReconnect=true',
        '调大 MySQL 服务端 wait_timeout 至 28800',
      ],
      preventive_tips:
        '对于大批量流式同步作业，请务必开启 JDBC 连接池的心跳保活（testWhileIdle）以防长空闲被切断。',
      author: 'STX预置经验库',
    },
    slot_not_enough: {
      title: 'Worker 节点 Slot 耗尽 (SlotNotEnoughException) 恢复方案',
      root_cause:
        '提交的任务并行度总和超过了当前集群全部处于 ACTIVE 状态的 Worker 节点空闲槽位总和。',
      solution:
        '1. 快速应急：在「集群管理 -> 节点列表」中水平新增部署 1~2 台 Worker 节点。\n2. 如果资源足够，可以建议开启动态 slot 机制。\n3. 检查是否有僵死作业未释放资源：在运行作业列表中排查并取消异常悬挂的任务。',
      actions_taken: [
        '扩容集群新增 Worker 节点',
        '将作业并行度从 8 调整为 4',
      ],
      preventive_tips:
        '日常运维建议保留集群 20%~30% 的 Slot 资源余量，避免多个调度任务并发堆叠导致资源挤兑。',
      author: 'STX预置经验库',
    },
    checkpoint_timeout: {
      title: '流作业 Checkpoint 超时失败排障方案',
      root_cause:
        '1. 下游 Sink 端写入过慢或目标存储负载过高，导致数据反压阻塞、Barrier 迟迟无法对齐完成；\n2. checkpoint.timeout 超时时间设置过短或 checkpoint.interval 过于频繁；\n3. 底层外部持久化存储（如 HDFS/S3/OSS/IMAP 存储）高延迟或网络波动。',
      solution:
        '1. 调大 Checkpoint 超时时间：在 seatunnel.yaml 中调大 checkpoint 超时时长：checkpoint.timeout: 120000（从默认 30s 提高至 120s），并可适当增大 checkpoint.interval 减轻高频打点负载。\n2. 重点排查下游 Sink 写入瓶颈：检查 Sink 端的数据库/目标存储压力；调整写入参数（如调小 batch.size、增大 buffer/flush 间隔、优化目标表索引与写入并发），提升写入速度消除反压，确保 Barrier 快速对齐。\n3. 排查底层外部持久化存储网络延时与 IOPS 瓶颈；若为离线 Batch 批处理作业，建议关闭外部存储避免外部 I/O 损耗。',
      actions_taken: [
        '在 seatunnel.yaml 或 seatunnel-env.sh 中调大 checkpoint.timeout: 120000',
        '排查下游 Sink 数据库负载并优化写入批次与并发参数加速 Barrier 对齐',
        '调大 checkpoint.interval 降低频次',
      ],
      preventive_tips:
        '运行长时间流作业（Streaming）时建议保持稳定吞吐并监控下游写入延时；批处理作业（Batch）建议关闭外部存储以避免外部 I/O 损耗。',
      author: 'STX预置经验库',
    },
    high_cpu_usage: {
      title: '节点 CPU 持续过高告警排查与处理',
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
      author: 'STX预置经验库',
    },
  },
  en: {
    mysql_connection_failure: {
      title: 'MySQL Connection Closed (Communications link failure) Recovery',
      root_cause:
        'MySQL server wait_timeout or interactive_timeout expired causing idle connections to be closed, or transient network fluctuation interrupted the persistent link.',
      solution:
        '1. Append timeout and auto-reconnect parameters to JDBC URL: autoReconnect=true&failOverReadOnly=false&maxReconnects=5&connectTimeout=30000&socketTimeout=60000\n2. Inspect MySQL server wait_timeout and interactive_timeout settings, recommended to be at least 28800 seconds.\n3. Add connection heartbeat check probe in job source/sink configuration: connection-check-timeout-sec: 30.',
      actions_taken: [
        'Append autoReconnect=true to JDBC URL',
        'Increase MySQL server wait_timeout to 28800',
      ],
      preventive_tips:
        'For high-throughput streaming sync jobs, enable JDBC connection pool heartbeat keepalive (testWhileIdle) to avoid idle drops.',
      author: 'STX Preset Knowledge Base',
    },
    slot_not_enough: {
      title: 'Worker Slot Exhaustion (SlotNotEnoughException) Recovery',
      root_cause:
        'The total parallelism of submitted jobs exceeds the sum of available idle slots on all ACTIVE worker nodes in the cluster.',
      solution:
        '1. Emergency scaling: Add 1-2 new Worker nodes under "Cluster Management -> Node List".\n2. Consider enabling dynamic-slot allocation if cluster resource headroom is adequate.\n3. Cancel hanging or zombie tasks in Running Jobs list to release occupied slots.',
      actions_taken: [
        'Scale out cluster with new Worker nodes',
        'Reduce job parallelism from 8 to 4',
      ],
      preventive_tips:
        'Reserve 20%-30% slot headroom in daily operations to prevent concurrent scheduling peaks from exhausting slot capacity.',
      author: 'STX Preset Knowledge Base',
    },
    checkpoint_timeout: {
      title: 'Streaming Checkpoint Timeout Exception Recovery',
      root_cause:
        '1. Slow sink ingestion or high target storage pressure causing backpressure and barrier alignment blockage;\n2. checkpoint.timeout configured too short or checkpoint.interval too frequent;\n3. High latency or IOPS bottleneck on underlying persistent storage (e.g. HDFS/S3/OSS/IMAP).',
      solution:
        '1. Increase Checkpoint Timeout: In seatunnel.yaml, increase checkpoint timeout to checkpoint.timeout: 120000 (from 30s to 120s), and consider enlarging checkpoint.interval to reduce checkpointing frequency.\n2. Troubleshoot Sink Ingestion Bottleneck: Inspect downstream database/storage load; tune sink batch parameters (e.g., lower batch.size, optimize buffer flush interval, tune indexing and write concurrency) to eliminate backpressure and accelerate barrier alignment.\n3. Verify Storage Latency and IOPS: Check network and I/O bottlenecks of underlying persistent storage; for offline batch jobs, disable external storage to avoid unnecessary I/O overhead.',
      actions_taken: [
        'Increase checkpoint.timeout: 120000 in seatunnel.yaml',
        'Tune downstream sink write parameters to accelerate barrier alignment',
        'Enlarge checkpoint.interval to reduce checkpoint overhead',
      ],
      preventive_tips:
        'Maintain steady throughput and monitor downstream sink write latency for streaming jobs; disable external storage for batch jobs to eliminate I/O overhead.',
      author: 'STX Preset Knowledge Base',
    },
    high_cpu_usage: {
      title: 'Node High CPU Usage Sustained Alert Remediation',
      root_cause:
        'Heavy transformation tasks (e.g. complex regex, JSON parsing) co-located on a single node, or frequent JVM FullGC consuming CPU.',
      solution:
        '1. Log in to the host and run jstack <pid> to check for active garbage collection threads or tight compute loops.\n2. Verify if multiple heavy tasks are running concurrently; stagger schedules across non-peak periods.\n3. If compute demand is permanently elevated, expand cluster capacity and leverage dynamic-slot distribution.',
      actions_taken: [
        'Run top -H -p <pid> to identify high-CPU threads',
        'Stagger scheduled tasks to avoid concurrency peaks',
      ],
      preventive_tips:
        'Filter out unused fields at source connectors to avoid redundant deserialization and memory overhead.',
      author: 'STX Preset Knowledge Base',
    },
  },
};

/**
 * 归一化提取预置类型标识
 * Normalize and resolve preset key from entry
 */
export function resolvePresetKey(
  entry?: TroubleshootingMemoryEntry | null,
): string | null {
  if (!entry) {
    return null;
  }
  if (entry.preset_key) {
    return entry.preset_key;
  }
  if (!entry.is_preset) {
    return null;
  }
  const fp = (entry.fingerprint || '').trim().toLowerCase();
  if (fp.includes('communications link failure')) {
    return 'mysql_connection_failure';
  }
  if (fp.includes('slotnotenoughexception')) {
    return 'slot_not_enough';
  }
  if (fp.includes('checkpointtimeoutexception')) {
    return 'checkpoint_timeout';
  }
  if (fp.includes('highcpuusage')) {
    return 'high_cpu_usage';
  }
  return null;
}

/**
 * 根据当前语言环境获取排障方案内容（仅对官方经典预置经验按全局语言切分，用户自定义经验保持原样）
 * Resolve localized memory entry based on current locale (only adapts presets; user entries remain as-is)
 */
export function getLocalizedMemory(
  entry: TroubleshootingMemoryEntry | null | undefined,
  locale: string = 'zh',
): TroubleshootingMemoryEntry | null {
  if (!entry) {
    return null;
  }

  // 用户自定义经验不考虑语言切换，直接原样呈现
  // User custom entries are preserved as-is without language adaptation
  const key = resolvePresetKey(entry);
  if (!key) {
    return entry;
  }

  const lang = String(locale).toLowerCase().startsWith('en') ? 'en' : 'zh';
  const localized = PRESET_LOCALIZATIONS[lang]?.[key];
  if (!localized) {
    return entry;
  }

  return {
    ...entry,
    title: localized.title,
    root_cause: localized.root_cause,
    solution: localized.solution,
    actions_taken: localized.actions_taken,
    preventive_tips: localized.preventive_tips,
    author: localized.author,
  };
}

/**
 * 判断经验条目是否匹配搜索关键字（官方经典预置方案支持在双语两版词汇中跨语言模糊命中）
 * Check whether a memory entry matches search keyword (presets check both Chinese and English representations)
 */
export function matchesMemoryKeyword(
  entry: TroubleshootingMemoryEntry,
  rawKeyword: string,
): boolean {
  const kw = rawKeyword.trim().toLowerCase();
  if (!kw) {
    return true;
  }

  // 1. 基础字段匹配
  // 1. Check primary entry fields
  const baseFields = [
    entry.title,
    entry.fingerprint,
    entry.preset_key,
    entry.error_summary,
    entry.root_cause,
    entry.solution,
    entry.preventive_tips,
    entry.author,
    ...(entry.tags || []),
    ...(entry.actions_taken || []),
  ];

  for (const field of baseFields) {
    if (field && String(field).toLowerCase().includes(kw)) {
      return true;
    }
  }

  // 2. 针对官方预置经典方案，跨语言检查全局双语词库
  // 2. For preset solutions, check bilingual dictionary across languages
  const key = resolvePresetKey(entry);
  if (key) {
    for (const lang of ['zh', 'en'] as const) {
      const loc = PRESET_LOCALIZATIONS[lang]?.[key];
      if (loc) {
        const localizedFields = [
          loc.title,
          loc.root_cause,
          loc.solution,
          loc.preventive_tips,
          loc.author,
          ...(loc.actions_taken || []),
        ];
        for (const f of localizedFields) {
          if (f && f.toLowerCase().includes(kw)) {
            return true;
          }
        }
      }
    }
  }

  return false;
}
