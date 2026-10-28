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

import {describe, expect, it} from 'vitest';
import {isValidClusterCode, recommendClusterCode} from '@/lib/cluster-code';

// 集群标识工具函数单测
// Unit tests for cluster code utility functions
describe('cluster-code', () => {
  describe('isValidClusterCode', () => {
    it('validates 2-32 lowercase alphanumeric strings', () => {
      expect(isValidClusterCode('stxcluster01')).toBe(true);
      expect(isValidClusterCode('ab')).toBe(true);
      expect(isValidClusterCode('a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6')).toBe(true);
    });

    it('rejects uppercase, symbols, and invalid length', () => {
      expect(isValidClusterCode('Cluster01')).toBe(false);
      expect(isValidClusterCode('stx-cluster')).toBe(false);
      expect(isValidClusterCode('stx_cluster')).toBe(false);
      expect(isValidClusterCode('a')).toBe(false);
      expect(isValidClusterCode('a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6extra')).toBe(false);
      expect(isValidClusterCode('')).toBe(false);
    });
  });

  describe('recommendClusterCode', () => {
    it('cleanses raw name to alphanumeric lowercase and truncates at 32 chars', () => {
      expect(recommendClusterCode('Production-Zeta_01!')).toBe('productionzeta01');
      expect(recommendClusterCode('实时计算 Zeta 99')).toBe('zeta99');
      expect(recommendClusterCode('')).toBe('');
      expect(recommendClusterCode('a'.repeat(40))).toHaveLength(32);
    });
  });
});
