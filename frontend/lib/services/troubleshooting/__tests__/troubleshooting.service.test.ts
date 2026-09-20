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

  it('应该包含系统预置的典型 SeaTunnel 排障经验', () => {
    // Should contain preset classic SeaTunnel troubleshooting solutions
    const memories = troubleshootingService.getMemories();
    expect(memories.length).toBeGreaterThanOrEqual(4);

    const mysqlCase = memories.find((m) =>
      m.fingerprint.includes('Communications link failure'),
    );
    expect(mysqlCase).toBeDefined();
    expect(mysqlCase?.is_preset).toBe(true);
    expect(mysqlCase?.solution).toContain('autoReconnect=true');

    const slotCase = memories.find((m) =>
      m.fingerprint.includes('SlotNotEnoughException'),
    );
    expect(slotCase).toBeDefined();
    expect(slotCase?.solution).toContain('Slot');
  });

  it('应该能按指纹和异常类名正确检索匹配的排障方案', () => {
    // Should correctly match troubleshooting solutions by fingerprint and exception class
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
    const memory = troubleshootingService.getMemoryById('preset-mysql-link-failure');
    expect(memory).toBeDefined();
    expect(memory?.id).toBe('preset-mysql-link-failure');
    expect(memory?.title).toContain('MySQL 连接断开');
  });
});
