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
 * 插件市场 Motion 动效辅助（兼容减少动态效果偏好）。
 */

import type {Transition, Variants} from 'motion/react';

/** Instant transition used under reduced motion. / 减少动效时的瞬时过渡。 */
export const PLUGIN_MOTION_INSTANT: Transition = {duration: 0.01};

/** Card entrance spring-like ease. / 卡片入场缓动。 */
export const PLUGIN_CARD_ENTRANCE_TRANSITION: Transition = {
  duration: 0.26,
  ease: [0.22, 1, 0.36, 1],
};

/**
 * Build staggered grid variants. Reduced motion keeps opacity-only fades.
 * 构建网格交错入场 variants；减少动效时仅保留轻微淡入。
 */
export function buildPluginGridVariants(reduceMotion: boolean): {
  container: Variants;
  item: Variants;
} {
  if (reduceMotion) {
    return {
      container: {
        hidden: {},
        visible: {
          transition: {staggerChildren: 0, delayChildren: 0},
        },
      },
      item: {
        hidden: {opacity: 0},
        visible: {
          opacity: 1,
          transition: {duration: 0.12, ease: 'linear'},
        },
      },
    };
  }

  return {
    container: {
      hidden: {},
      visible: {
        transition: {
          staggerChildren: 0.04,
          delayChildren: 0.02,
        },
      },
    },
    item: {
      hidden: {opacity: 0, y: 8, scale: 0.98},
      visible: {
        opacity: 1,
        y: 0,
        scale: 1,
        transition: PLUGIN_CARD_ENTRANCE_TRANSITION,
      },
    },
  };
}

/**
 * Hover feedback for marketplace cards. Reduced motion disables lift.
 * 市场卡片悬停反馈；减少动效时关闭抬起位移。
 */
export function buildPluginCardHover(reduceMotion: boolean) {
  if (reduceMotion) {
    return undefined;
  }
  return {
    y: -3,
    transition: {duration: 0.18, ease: 'easeOut' as const},
  };
}

/**
 * Page-level fade/slide variants for PluginMain shell.
 * PluginMain 外壳页级淡入/位移 variants。
 */
export function buildPluginPageVariants(reduceMotion: boolean): {
  container: Variants;
  item: Variants;
} {
  if (reduceMotion) {
    return {
      container: {
        hidden: {opacity: 0},
        visible: {
          opacity: 1,
          transition: {duration: 0.15, staggerChildren: 0},
        },
      },
      item: {
        hidden: {opacity: 0},
        visible: {opacity: 1, transition: {duration: 0.12}},
      },
    };
  }

  return {
    container: {
      hidden: {opacity: 0},
      visible: {
        opacity: 1,
        transition: {
          duration: 0.45,
          staggerChildren: 0.08,
        },
      },
    },
    item: {
      hidden: {opacity: 0, y: 16},
      visible: {
        opacity: 1,
        y: 0,
        transition: {duration: 0.45},
      },
    },
  };
}

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
