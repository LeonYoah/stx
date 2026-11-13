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
 * Plugin marketplace Motion helpers (reduced-motion aware).
 * Re-exports shared marketplace helpers and keeps cluster-stack specifics here.
 * 插件市场 Motion 辅助：复用共享 marketplace helpers，集群堆叠逻辑留在本文件。
 */

export {
  MARKETPLACE_MOTION_INSTANT as PLUGIN_MOTION_INSTANT,
  MARKETPLACE_ENTRANCE_TRANSITION as PLUGIN_CARD_ENTRANCE_TRANSITION,
  buildMarketplaceGridVariants as buildPluginGridVariants,
  buildMarketplaceCardHover as buildPluginCardHover,
  buildMarketplacePageVariants as buildPluginPageVariants,
} from '@/lib/animations/marketplace-motion';

/**
 * Cluster stack overlap / expand metrics.
 * 集群堆叠重叠与展开度量。
 */
export const CLUSTER_STACK_COLLAPSED_OVERLAP = -10;
export const CLUSTER_STACK_EXPANDED_GAP = 6;
export const CLUSTER_STACK_HOVER_SCALE = 1.18;

/**
 * Resolve horizontal offset for a stack item under hover/push.
 * 计算堆叠项在悬停推让下的水平偏移。
 */
export function resolveClusterStackOffset(
  index: number,
  hoveredIndex: number | null,
  reduceMotion: boolean,
): {marginLeft: number; scale: number; zIndex: number} {
  const isHovered = hoveredIndex === index;
  const expanded = hoveredIndex !== null;
  const marginLeft =
    index === 0
      ? 0
      : expanded
        ? CLUSTER_STACK_EXPANDED_GAP
        : CLUSTER_STACK_COLLAPSED_OVERLAP;

  return {
    marginLeft,
    // 减少动效时不做放大，只做即时间距切换。
    // Under reduced motion skip scale; only switch spacing instantly.
    scale: !reduceMotion && isHovered ? CLUSTER_STACK_HOVER_SCALE : 1,
    zIndex: isHovered ? 40 : index + 1,
  };
}

/**
 * Derive short initials for a cluster chip (not a user avatar).
 * 从集群名称派生短缩写（不是用户头像）。
 */
export function getClusterStackInitials(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) {
    return '?';
  }

  // CJK: first character is enough for a compact chip.
  // 中日韩：取首字即可作为紧凑标识。
  if (/[\u4e00-\u9fff]/.test(trimmed[0] ?? '')) {
    return trimmed[0]!.toUpperCase();
  }

  const parts = trimmed.split(/[\s_-]+/).filter(Boolean);
  if (parts.length >= 2) {
    return `${parts[0]![0] ?? ''}${parts[1]![0] ?? ''}`.toUpperCase();
  }

  return trimmed.slice(0, 2).toUpperCase();
}
