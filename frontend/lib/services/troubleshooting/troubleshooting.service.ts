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
      const entrySummary = (entry.error_summary || '').toLowerCase();
      const entryTags = (entry.tags || []).map((t) => t.toLowerCase());

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
