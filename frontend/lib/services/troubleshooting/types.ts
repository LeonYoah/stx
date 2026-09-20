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

/**
 * 排障经验记忆库目标分类：错误日志还是监控告警
 * Target category of troubleshooting memory: error log or monitoring alert
 */
export type TroubleshootingTargetType = 'error' | 'alert';

/**
 * 排障经验记忆库核心条目实体
 * Core entry entity of the troubleshooting memory bank
 */
export interface TroubleshootingMemoryEntry {
  /**
   * 唯一标识 ID
   * Unique identifier ID
   */
  id: string;

  /**
   * 关联目标类型（错误组或告警项）
   * Associated target category (error group or alert instance)
   */
  target_type: TroubleshootingTargetType;

  /**
   * 故障唯一指纹（用于同类错误/告警自动检索匹配）
   * Fault unique fingerprint (used for auto-matching similar errors/alerts)
   */
  fingerprint: string;

  /**
   * 方案标题
   * Solution title
   */
  title: string;

  /**
   * 异常/故障现象摘要
   * Exception or symptom summary
   */
  error_summary: string;

  /**
   * 根因分析说明（选填）
   * Root cause analysis (optional)
   */
  root_cause?: string;

  /**
   * 真实解决方案与排障执行动作（必填核心内容）
   * Verified solution and remediation actions (mandatory core content)
   */
  solution: string;

  /**
   * 具体执行的命令或参数变更列表（选填）
   * Action items or configuration adjustments executed (optional)
   */
  actions_taken?: string[];

  /**
   * 防范优化建议（选填）
   * Preventive tips and best practices (optional)
   */
  preventive_tips?: string;

  /**
   * 所属集群标识（选填）
   * Associated cluster ID (optional)
   */
  cluster_id?: number | string;

  /**
   * 所属集群名称（选填）
   * Associated cluster name (optional)
   */
  cluster_name?: string;

  /**
   * 检索分类标签（如 mysql, timeout, slot, oom 等）
   * Categorical tags for search and filtering
   */
  tags: string[];

  /**
   * 沉淀人 / 记录者
   * Author / troubleshooter who recorded the solution
   */
  author: string;

  /**
   * 创建时间（ISO 格式）
   * Creation timestamp (ISO string)
   */
  created_at: string;

  /**
   * 最后更新时间（ISO 格式）
   * Last updated timestamp (ISO string)
   */
  updated_at: string;

  /**
   * 是否为系统内置经典排障经验
   * Whether this entry is a preset classic solution
   */
  is_preset?: boolean;
}

/**
 * 排障经验检索查询条件
 * Query parameters for matching troubleshooting memories
 */
export interface TroubleshootingMemoryQuery {
  /**
   * 精确匹配指纹
   * Exact fingerprint to match
   */
  fingerprint?: string;

  /**
   * 异常类名（模糊回退匹配）
   * Exception class name for fallback fuzzy match
   */
  exception_class?: string;

  /**
   * 标题或关键字（模糊回退匹配）
   * Title or keyword for fallback fuzzy match
   */
  title?: string;

  /**
   * 目标类型过滤
   * Target category filter
   */
  target_type?: TroubleshootingTargetType;

  /**
   * 集群过滤
   * Cluster filter
   */
  cluster_id?: number | string;
}
