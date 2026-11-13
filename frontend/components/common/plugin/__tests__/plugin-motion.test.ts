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
 * Unit tests for plugin marketplace motion helpers and reduced-motion contracts.
 * 插件市场动效辅助与减少动态效果契约的单元测试。
 */

import {describe, expect, it} from 'vitest';
import {
  buildPluginCardHover,
  buildPluginGridVariants,
  buildPluginPageVariants,
  CLUSTER_STACK_COLLAPSED_OVERLAP,
  CLUSTER_STACK_EXPANDED_GAP,
  CLUSTER_STACK_HOVER_SCALE,
  getClusterStackInitials,
  resolveClusterStackOffset,
} from '../plugin-motion';

describe('plugin-motion', () => {
  describe('buildPluginGridVariants', () => {
    it('uses opacity-only entrance under reduced motion', () => {
      const {item, container} = buildPluginGridVariants(true);
      const hidden = item.hidden as {opacity?: number; y?: number; scale?: number};
      const visible = item.visible as {
        opacity?: number;
        transition?: {duration?: number};
      };
      const visibleContainer = container.visible as {
        transition?: {staggerChildren?: number};
      };

      expect(hidden).toEqual({opacity: 0});
      expect(hidden.y).toBeUndefined();
      expect(hidden.scale).toBeUndefined();
      expect(visible.opacity).toBe(1);
      expect(visible.transition?.duration).toBeLessThanOrEqual(0.15);
      expect(visibleContainer.transition?.staggerChildren).toBe(0);
    });

    it('keeps modest stagger and lift when motion is allowed', () => {
      const {item, container} = buildPluginGridVariants(false);
      const hidden = item.hidden as {opacity?: number; y?: number; scale?: number};
      const visibleContainer = container.visible as {
        transition?: {staggerChildren?: number};
      };

      expect(hidden.opacity).toBe(0);
      expect(hidden.y).toBeGreaterThan(0);
      expect(hidden.scale).toBeLessThan(1);
      expect(visibleContainer.transition?.staggerChildren).toBeGreaterThan(0);
    });
  });

  describe('buildPluginCardHover', () => {
    it('disables hover lift under reduced motion', () => {
      expect(buildPluginCardHover(true)).toBeUndefined();
    });

    it('returns a small vertical lift when motion is allowed', () => {
      const hover = buildPluginCardHover(false);
      expect(hover).toBeDefined();
      expect(hover?.y).toBeLessThan(0);
      expect(Math.abs(hover?.y ?? 0)).toBeLessThanOrEqual(4);
    });
  });

  describe('buildPluginPageVariants', () => {
    it('drops layout travel under reduced motion', () => {
      const {item} = buildPluginPageVariants(true);
      const hidden = item.hidden as {opacity?: number; y?: number};
      expect(hidden).toEqual({opacity: 0});
      expect(hidden.y).toBeUndefined();
    });
  });

  describe('resolveClusterStackOffset', () => {
    it('overlaps neighbors when collapsed', () => {
      const offset = resolveClusterStackOffset(2, null, false);
      expect(offset.marginLeft).toBe(CLUSTER_STACK_COLLAPSED_OVERLAP);
      expect(offset.scale).toBe(1);
    });

    it('pushes neighbors and enlarges the hovered item', () => {
      const hovered = resolveClusterStackOffset(1, 1, false);
      const neighbor = resolveClusterStackOffset(2, 1, false);

      expect(hovered.marginLeft).toBe(CLUSTER_STACK_EXPANDED_GAP);
      expect(hovered.scale).toBe(CLUSTER_STACK_HOVER_SCALE);
      expect(hovered.zIndex).toBeGreaterThan(neighbor.zIndex);
      expect(neighbor.marginLeft).toBe(CLUSTER_STACK_EXPANDED_GAP);
      expect(neighbor.scale).toBe(1);
    });

    it('skips scale under reduced motion while still switching spacing', () => {
      const hovered = resolveClusterStackOffset(1, 1, true);
      expect(hovered.marginLeft).toBe(CLUSTER_STACK_EXPANDED_GAP);
      expect(hovered.scale).toBe(1);
    });
  });

  describe('getClusterStackInitials', () => {
    it('uses the first CJK character', () => {
      expect(getClusterStackInitials('生产集群')).toBe('生');
    });

    it('uses two Latin initials from multi-word names', () => {
      expect(getClusterStackInitials('prod-east')).toBe('PE');
      expect(getClusterStackInitials('Staging Cluster')).toBe('SC');
    });

    it('falls back to a question mark for empty names', () => {
      expect(getClusterStackInitials('   ')).toBe('?');
    });
  });
});
