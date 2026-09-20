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

import {describe, it, expect, beforeEach} from 'vitest';
import {troubleshootingService} from '../troubleshooting.service';

describe('TroubleshootingService', () => {
  beforeEach(() => {
    // 清空本地存储
    // Clear localStorage before each test
    localStorage.clear();
  });

  it('初始状态下本地经验为空，支持通过 saveMemory 写入并检索', () => {
    // Local cache initially empty, supports saving and retrieving via saveMemory
    const memories = troubleshootingService.getMemories();
    expect(memories.length).toBe(0);

    const saved = troubleshootingService.saveMemory({
      target_type: 'error',
      fingerprint: 'Communications link failure',
      title: 'MySQL 连接断开 Communications link failure 恢复方案',
      error_summary: 'The last packet successfully received from the server was 30,000 milliseconds ago.',
      solution: '在 SeaTunnel JDBC 连接串中增加参数：autoReconnect=true',
      tags: ['mysql', 'jdbc'],
      author: 'STX预置经验库',
    });

    const refreshed = troubleshootingService.getMemories();
    expect(refreshed.length).toBe(1);
    expect(refreshed[0].id).toBe(saved.id);
    expect(refreshed[0].solution).toContain('autoReconnect=true');
  });

  it('应该能按指纹和异常类名正确检索匹配的排障方案', () => {
    // Should correctly match troubleshooting solutions by fingerprint and exception class
    troubleshootingService.saveMemory({
      target_type: 'error',
      fingerprint: 'Communications link failure',
      title: 'MySQL 连接断开恢复方案',
      solution: '1. 检查 MySQL 连接池心跳与超时',
      tags: ['mysql'],
    });
    troubleshootingService.saveMemory({
      target_type: 'error',
      fingerprint: 'SlotNotEnoughException',
      title: 'Worker 节点 Slot 耗尽恢复方案',
      solution: '1. 扩容集群或调小作业并行度',
      tags: ['slot'],
    });

    const matches = troubleshootingService.findMatchingMemories({
      fingerprint: 'Communications link failure',
      target_type: 'error',
    });
    expect(matches.length).toBeGreaterThan(0);
    expect(matches[0].fingerprint).toContain('Communications link failure');

    const fuzzyMatches = troubleshootingService.findMatchingMemories({
      exception_class: 'org.apache.seatunnel.engine.common.exception.SeaTunnelEngineException: SlotNotEnoughException',
      target_type: 'error',
    });
    expect(fuzzyMatches.length).toBeGreaterThan(0);
    expect(fuzzyMatches[0].title).toContain('Slot');
  });

  it('保存排障经验时必须要求填写解决方案，否则抛出异常', () => {
    // Must require a verified solution when saving troubleshooting memory, throws otherwise
    expect(() => {
      troubleshootingService.saveMemory({
        target_type: 'error',
        fingerprint: 'CustomError',
        title: '测试错误方案',
        error_summary: '出现异常',
        solution: '   ', // 空白解决方案 / Empty whitespace solution
        tags: ['test'],
        author: '测试员',
      });
    }).toThrow('解决方案内容不能为空');
  });

  it('应该能成功保存并更新用户沉淀的排障经验', () => {
    // Should successfully save and update user-contributed troubleshooting memory
    const saved = troubleshootingService.saveMemory({
      target_type: 'error',
      fingerprint: 'CustomTestException',
      title: '自定义测试异常恢复方案',
      error_summary: 'Custom test exception occurred',
      root_cause: '测试环境配置不匹配',
      solution: '1. 修改配置文件；2. 重启服务。',
      preventive_tips: '提交前自检配置',
      tags: ['test', 'custom'],
      author: '张三',
    });

    expect(saved.id).toBeDefined();
    expect(saved.is_preset).toBe(false);
    expect(saved.solution).toBe('1. 修改配置文件；2. 重启服务。');

    // 再次检索应当精确命中刚保存的方案
    // Matching query should now hit this newly saved entry with top priority
    const matched = troubleshootingService.findMatchingMemories({
      fingerprint: 'CustomTestException',
      target_type: 'error',
    });
    expect(matched.length).toBeGreaterThan(0);
    expect(matched[0].id).toBe(saved.id);
    expect(matched[0].author).toBe('张三');

    // 更新该经验
    // Update the memory
    const updated = troubleshootingService.saveMemory({
      id: saved.id,
      target_type: 'error',
      fingerprint: 'CustomTestException',
      title: '更新后的测试方案标题',
      error_summary: 'Custom test exception occurred',
      solution: '1. 修改配置并清理缓存；2. 重启服务并验证。',
      tags: ['test', 'custom', 'v2'],
      author: '李四',
    });
    expect(updated.id).toBe(saved.id);
    expect(updated.title).toBe('更新后的测试方案标题');
    expect(updated.author).toBe('李四');

    // 删除经验
    // Delete the memory
    const deleted = troubleshootingService.deleteMemory(saved.id);
    expect(deleted).toBe(true);
    const postDeleteMatches = troubleshootingService.findMatchingMemories({
      fingerprint: 'CustomTestException',
      target_type: 'error',
    });
    expect(postDeleteMatches.length).toBe(0);
  });

  it('应该支持根据 ID 精确获取经验详情', () => {
    // Should support fetching memory detail by ID
    const saved = troubleshootingService.saveMemory({
      target_type: 'error',
      fingerprint: 'Communications link failure',
      title: 'MySQL 连接断开恢复方案',
      solution: '增加 autoReconnect 参数',
    });
    const memory = troubleshootingService.getMemoryById(saved.id);
    expect(memory).toBeDefined();
    expect(memory?.id).toBe(saved.id);
    expect(memory?.title).toContain('MySQL 连接断开');
  });

  it('官方经典方案支持按全局语言切分自适应，用户自定义经验保持原样', async () => {
    // Official presets adapt according to global language; user custom entries remain as-is
    const {getLocalizedMemory, matchesMemoryKeyword} = await import('../localize');

    const presetEntry = {
      id: 'preset-1',
      target_type: 'error' as const,
      fingerprint: 'Communications link failure',
      preset_key: 'mysql_connection_failure',
      title: 'MySQL 连接断开恢复方案',
      error_summary: 'Connection lost',
      solution: '增加 autoReconnect 参数',
      tags: ['mysql'],
      author: 'STX预置经验库',
      is_preset: true,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    // 中文环境下展示中文版
    // In Chinese locale, display Chinese version
    const localizedZh = getLocalizedMemory(presetEntry, 'zh');
    expect(localizedZh?.title).toContain('MySQL 连接中断');
    expect(localizedZh?.author).toBe('STX预置经验库');

    // 英文环境下跟随全局语言切换为英文版
    // In English locale, switch to English version along with global language
    const localizedEn = getLocalizedMemory(presetEntry, 'en');
    expect(localizedEn?.title).toContain('MySQL Connection Closed');
    expect(localizedEn?.author).toBe('stx Official Knowledge Base');
    expect(localizedEn?.solution).toContain('autoReconnect=true');

    // 用户自定义经验：中英文环境均保持用户填写的原样
    // User custom entries: remain as-is in both Chinese and English
    const customEntry = {
      id: 'custom-1',
      target_type: 'error' as const,
      fingerprint: 'MyCustomError',
      title: '团队自定义同步问题解决手册',
      error_summary: 'My error',
      solution: '步骤一：重启；步骤二：回滚配置。',
      tags: ['custom'],
      author: '张三',
      is_preset: false,
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    };

    const customZh = getLocalizedMemory(customEntry, 'zh');
    expect(customZh?.title).toBe('团队自定义同步问题解决手册');

    const customEn = getLocalizedMemory(customEntry, 'en');
    expect(customEn?.title).toBe('团队自定义同步问题解决手册');
    expect(customEn?.solution).toBe('步骤一：重启；步骤二：回滚配置。');

    // 关键字搜索：官方方案支持中英双向命中
    // Keyword search: presets match in both Chinese and English
    expect(matchesMemoryKeyword(presetEntry, '连接中断')).toBe(true);
    expect(matchesMemoryKeyword(presetEntry, 'reconnect')).toBe(true);
    expect(matchesMemoryKeyword(presetEntry, 'timeout')).toBe(true);

    // 关键字搜索：自定义方案精准匹配
    // Keyword search: custom matches user content
    expect(matchesMemoryKeyword(customEntry, '回滚')).toBe(true);
    expect(matchesMemoryKeyword(customEntry, 'non_existing')).toBe(false);
  });
});
