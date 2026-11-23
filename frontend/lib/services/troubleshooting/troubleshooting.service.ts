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

// 预置排障经验库统一在后端数据库迁移时播种并由 API 提供，前端不硬编码静态方案
// Preset troubleshooting memories are seeded during backend DB migration and served via API; frontend does not hardcode static entries
const PRESET_MEMORIES: TroubleshootingMemoryEntry[] = [];

// 强特征最小长度：低于该长度的片段（如 "oom"、"npe"）容易误撞，不作强特征参与匹配
// Minimum strong-feature length; shorter fragments (e.g. "oom", "npe") are too prone to false hits
const MIN_STRONG_FEATURE_LENGTH = 6;

// 高置信包含匹配的最小长度：双方均需达到该长度才允许标题/指纹互相包含命中
// Minimum length for high-confidence containment between title/fingerprint pairs
const MIN_HIGH_CONFIDENCE_LENGTH = 8;

// 命中门槛：强特征得分达到该值才算命中；tag 只在此之上追加排序分，绝不独立命中
// Match threshold: an entry is a match only when its strong-feature score reaches this;
// tag hits only add ordering bonus on top, never match alone
const MIN_MATCH_SCORE = 50;

// 过于宽泛的通用词永远不算强特征，避免 checkpoint/storage/worker 等词造成荒谬误命中
// Overly generic words never count as strong features, preventing absurd hits on words
// like checkpoint/storage/worker
const GENERIC_MATCH_WORDS = new Set([
  'exception',
  'error',
  'failed',
  'failure',
  'timeout',
  'storage',
  'checkpoint',
  'worker',
  'master',
  'server',
  'connection',
  'unknown',
  'fatal',
  'warning',
]);


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
      throw new Error(
        '解决方案内容不能为空，须包含具体处理措施与步骤',
      );
    }

    const now = new Date().toISOString();
    const id = entry.id || `mem-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
    const newEntry: TroubleshootingMemoryEntry = {
      ...entry,
      id,
      solution: trimmedSolution,
      title: entry.title.trim() || '未命名方案',
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
   * 根据指纹与关键字匹配相关排障经验（强特征命中制：指纹/异常类/高置信包含命中才可入选，
   * tag 仅在已命中条目之间追加排序分，宁缺毋滥）
   * Match relevant troubleshooting memories (strong-feature gating: only fingerprint,
   * exception-class or high-confidence containment hits qualify; tags only add ordering
   * bonus on already-matched entries so weak matches are dropped)
   */
  public findMatchingMemories(
    query: TroubleshootingMemoryQuery,
  ): TroubleshootingMemoryEntry[] {
    const all = this.getMemories();
    const {fingerprint, exception_class, title, target_type} = query;

    const normalizedFingerprint = fingerprint?.trim().toLowerCase();
    const normalizedException = exception_class?.trim().toLowerCase();
    const normalizedTitle = title?.trim().toLowerCase();

    // 强特征判定：非空、足够长且非通用词，防止空串/宽泛词把 includes 打成恒真
    // Strong-feature gate: non-empty, long enough and non-generic, so empty or broad
    // fragments can never turn String.includes into an always-true hit
    const isStrongFeature = (value?: string): value is string =>
      Boolean(value) &&
      value!.length >= MIN_STRONG_FEATURE_LENGTH &&
      !GENERIC_MATCH_WORDS.has(value!);

    const scored = all.map((entry) => {
      const entryFp = (entry.fingerprint || '').trim().toLowerCase();
      const entryTitle = (entry.title || '').toLowerCase();
      const entrySummary = (entry.error_summary || '').toLowerCase();
      const entryTags = (entry.tags || []).map((t) => t.toLowerCase());

      // 类型匹配过滤
      // Target type filtering
      if (target_type && entry.target_type !== target_type) {
        return {entry, score: 0};
      }

      let score = 0;

      // 强特征 1（+100）：完整指纹精确命中（覆盖用户沉淀的 SHA256 指纹）
      // Strong 1 (+100): exact fingerprint hit (covers user-deposited SHA256 fingerprints)
      if (normalizedFingerprint && entryFp === normalizedFingerprint) {
        score += 100;
      }

      // 强特征 2（+80）：特征签名/异常类命中——条目指纹作为完整特征短语出现在
      // 查询侧任一字段，或查询异常类完整出现在条目指纹/标题/摘要中
      // Strong 2 (+80): feature/exception-class hit — the entry fingerprint appears as a
      // whole signature inside any query field, or the query exception class appears
      // verbatim inside the entry fingerprint/title/summary
      if (
        isStrongFeature(entryFp) &&
        ((normalizedFingerprint && normalizedFingerprint.includes(entryFp)) ||
          (normalizedException && normalizedException.includes(entryFp)) ||
          (normalizedTitle && normalizedTitle.includes(entryFp)))
      ) {
        score += 80;
      }
      // 条目已带强指纹时指纹即签名，异常类名不允许单独命中——否则像
      // UnsupportedOperationException 这种通用异常会把「getSubject 专属」的
      // 预置经验贴到所有同类报错上。
      // When an entry carries a strong fingerprint, that fingerprint is the signature:
      // an exception class alone must not match — otherwise a generic class like
      // UnsupportedOperationException would attach the getSubject-specific preset to
      // every unrelated error of the same class.
      const exceptionClassHit =
        isStrongFeature(normalizedException) &&
        (entryFp.includes(normalizedException) ||
          entryTitle.includes(normalizedException) ||
          entrySummary.includes(normalizedException));
      if (exceptionClassHit && !isStrongFeature(entryFp)) {
        score += 80;
      }

      // 强特征 3（+50）：高置信标题/指纹包含（双方 >= 8 字符且非通用词）
      // Strong 3 (+50): high-confidence title/fingerprint containment (both sides >= 8
      // chars and non-generic)
      if (
        normalizedTitle &&
        normalizedTitle.length >= MIN_HIGH_CONFIDENCE_LENGTH &&
        !GENERIC_MATCH_WORDS.has(normalizedTitle) &&
        (entryTitle.includes(normalizedTitle) || entryFp.includes(normalizedTitle))
      ) {
        score += 50;
      }
      if (
        normalizedFingerprint &&
        normalizedFingerprint.length >= MIN_HIGH_CONFIDENCE_LENGTH &&
        !GENERIC_MATCH_WORDS.has(normalizedFingerprint) &&
        (entryFp.includes(normalizedFingerprint) ||
          entryTitle.includes(normalizedFingerprint))
      ) {
        score += 50;
      }

      // 弱特征：tag 仅在强特征已达标的条目上追加排序分，绝不独立命中
      // Weak signal: tags only add ordering bonus when the strong-feature threshold is
      // already met; they can never produce a match on their own
      if (score >= MIN_MATCH_SCORE) {
        for (const tag of entryTags) {
          if (
            tag &&
            ((normalizedTitle && normalizedTitle.includes(tag)) ||
              (normalizedFingerprint && normalizedFingerprint.includes(tag)) ||
              (normalizedException && normalizedException.includes(tag)))
          ) {
            score += 5;
          }
        }
      }

      return {entry, score};
    });

    // 仅保留达到强特征门槛的记录，并按得分与更新时间倒序排序
    // Keep only entries reaching the strong-feature threshold, sorted by score and recency
    return scored
      .filter((item) => item.score >= MIN_MATCH_SCORE)
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
