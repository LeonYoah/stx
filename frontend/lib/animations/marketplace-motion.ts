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
 * Shared marketplace Motion helpers (plugin + packages), reduced-motion aware.
 * 插件市场与安装包管理共用的 Motion 辅助（兼容减少动态效果）。
 */

import type {Transition, Variants} from 'motion/react';

/** Instant transition used under reduced motion. / 减少动效时的瞬时过渡。 */
export const MARKETPLACE_MOTION_INSTANT: Transition = {duration: 0.01};

/** Card / row entrance ease. / 卡片与表格行入场缓动。 */
export const MARKETPLACE_ENTRANCE_TRANSITION: Transition = {
  duration: 0.26,
  ease: [0.22, 1, 0.36, 1],
};

/**
 * Staggered grid variants for card walls. Reduced motion → opacity-only fade.
 * 卡片墙交错入场；减少动效时仅淡入。
 */
export function buildMarketplaceGridVariants(reduceMotion: boolean): {
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
        transition: MARKETPLACE_ENTRANCE_TRANSITION,
      },
    },
  };
}

/**
 * Hover lift for cards. Reduced motion disables transform lift.
 * 卡片悬停抬起；减少动效时关闭位移。
 */
export function buildMarketplaceCardHover(reduceMotion: boolean) {
  if (reduceMotion) {
    return undefined;
  }
  return {
    y: -3,
    transition: {duration: 0.18, ease: 'easeOut' as const},
  };
}

/**
 * Page shell fade/slide variants.
 * 页面外壳淡入/位移 variants。
 */
export function buildMarketplacePageVariants(reduceMotion: boolean): {
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
 * Table row stagger variants. No layout scale; reduced motion → opacity only.
 * 表格行交错入场；不做布局缩放；减少动效时仅淡入。
 */
export function buildMarketplaceTableRowVariants(reduceMotion: boolean): {
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
          transition: {duration: 0.1, ease: 'linear'},
        },
      },
    };
  }

  return {
    container: {
      hidden: {},
      visible: {
        transition: {
          staggerChildren: 0.02,
          delayChildren: 0.01,
        },
      },
    },
    item: {
      hidden: {opacity: 0, y: 4},
      visible: {
        opacity: 1,
        y: 0,
        transition: {duration: 0.22, ease: [0.22, 1, 0.36, 1]},
      },
    },
  };
}

/**
 * Row hover feedback without layout push/scale. Reduced motion → none.
 * 行悬停反馈（无布局推让/缩放）；减少动效时关闭。
 */
export function buildMarketplaceTableRowHover(reduceMotion: boolean) {
  if (reduceMotion) {
    return undefined;
  }
  return {
    backgroundColor: 'color-mix(in oklab, var(--muted) 45%, transparent)',
    transition: {duration: 0.16, ease: 'easeOut' as const},
  };
}
