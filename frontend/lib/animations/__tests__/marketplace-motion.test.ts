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
 * Shared marketplace motion helper contracts (grid, page, table rows).
 * 共享 marketplace 动效辅助契约（网格、页面、表格行）。
 */

import {describe, expect, it} from 'vitest';
import {
  buildMarketplaceCardHover,
  buildMarketplaceGridVariants,
  buildMarketplacePageVariants,
  buildMarketplaceTableRowHover,
  buildMarketplaceTableRowVariants,
} from '../marketplace-motion';

describe('marketplace-motion', () => {
  describe('buildMarketplaceTableRowVariants', () => {
    it('uses opacity-only entrance under reduced motion', () => {
      const {item, container} = buildMarketplaceTableRowVariants(true);
      const hidden = item.hidden as {opacity?: number; y?: number; scale?: number};
      const visibleContainer = container.visible as {
        transition?: {staggerChildren?: number};
      };

      expect(hidden).toEqual({opacity: 0});
      expect(hidden.y).toBeUndefined();
      expect(hidden.scale).toBeUndefined();
      expect(visibleContainer.transition?.staggerChildren).toBe(0);
    });

    it('keeps a modest vertical stagger when motion is allowed', () => {
      const {item, container} = buildMarketplaceTableRowVariants(false);
      const hidden = item.hidden as {opacity?: number; y?: number; scale?: number};
      const visibleContainer = container.visible as {
        transition?: {staggerChildren?: number};
      };

      expect(hidden.opacity).toBe(0);
      expect(hidden.y).toBeGreaterThan(0);
      expect(hidden.scale).toBeUndefined();
      expect(visibleContainer.transition?.staggerChildren).toBeGreaterThan(0);
    });
  });

  describe('buildMarketplaceTableRowHover', () => {
    it('disables hover tint under reduced motion', () => {
      expect(buildMarketplaceTableRowHover(true)).toBeUndefined();
    });

    it('returns a background tint without scale when motion is allowed', () => {
      const hover = buildMarketplaceTableRowHover(false);
      expect(hover).toBeDefined();
      expect(hover?.backgroundColor).toBeTruthy();
      expect(
        (hover as {scale?: number} | undefined)?.scale,
      ).toBeUndefined();
    });
  });

  describe('grid / page aliases stay reduced-motion safe', () => {
    it('grid item drops transform travel under reduced motion', () => {
      const {item} = buildMarketplaceGridVariants(true);
      const hidden = item.hidden as {opacity?: number; y?: number; scale?: number};
      expect(hidden).toEqual({opacity: 0});
      expect(buildMarketplaceCardHover(true)).toBeUndefined();
    });

    it('page item drops layout travel under reduced motion', () => {
      const {item} = buildMarketplacePageVariants(true);
      const hidden = item.hidden as {opacity?: number; y?: number};
      expect(hidden).toEqual({opacity: 0});
      expect(hidden.y).toBeUndefined();
    });
  });
});
