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
 * Shared responsive sizes for marketplace plugin tiles and package Apple cards.
 * 插件市场瓦片与安装包 Apple 大卡的共享响应式尺寸。
 *
 * Goals / 目标：
 * - Fill the available content width/height without large empty gutters.
 * - 宽屏吃满内容区，避免大片留白。
 * - Stay dense enough that taller cards do not look hollow.
 * - 加高后仍靠字号层级与内边距填满，避免空洞。
 */

/** Plugin FlipCard / skeleton height band. / 插件翻转卡与骨架高度带。 */
export const MARKETPLACE_PLUGIN_CARD_HEIGHT_CLASS =
  'h-[15.25rem] min-[480px]:h-[15.75rem] md:h-[16.25rem] xl:h-[16.75rem]';

/**
 * Plugin card grid: auto-fill with larger min tracks so wide screens grow
 * fewer, wider cards instead of tiny sparse columns.
 * 插件卡网格：更大最小轨宽的 auto-fill，宽屏变成更少但更宽的卡。
 */
export const MARKETPLACE_PLUGIN_CARD_GRID_CLASS =
  'grid gap-4 grid-cols-1 min-[420px]:grid-cols-2 md:grid-cols-[repeat(auto-fill,minmax(18.5rem,1fr))] xl:grid-cols-[repeat(auto-fill,minmax(20rem,1fr))] 2xl:grid-cols-[repeat(auto-fill,minmax(21.5rem,1fr))]';

/**
 * Package Apple carousel widths — 1440 约一屏 3 张；390 近一屏一张。
 * Package Apple carousel widths — ~3 cards at 1440; nearly one at 390.
 */
export const PACKAGE_APPLE_CARD_WIDTH_CLASS = [
  // 390：视口定宽（勿用 %，shrink-0 父级会导致 100% 塌缩）。
  // ~390: viewport width (% collapses under shrink-0 parents).
  'w-[calc(100vw-2.75rem)] max-w-[24rem]',
  'sm:w-[22rem] sm:max-w-none',
  'md:w-96',
  'lg:w-[25rem]',
  // 1440：稍收窄，约 3 张可见（416px×3 + gap ≈ 内容区）。
  // 1440: slightly narrower so ~3 cards fit (416px×3 + gaps ≈ pane).
  'xl:w-[26rem]',
].join(' ');

/** Rich meta (downloaded / many fields). / 已下载或字段较多时的高度。 */
export const PACKAGE_APPLE_CARD_HEIGHT_RICH_CLASS =
  'h-[22rem] sm:h-[23rem] md:h-[24rem] xl:h-[24.5rem]';

/** Compact meta (online-only / few fields). / 仅在线或字段少时压缩高度。 */
export const PACKAGE_APPLE_CARD_HEIGHT_COMPACT_CLASS =
  'h-[19.5rem] sm:h-[20rem] md:h-[21rem] xl:h-[21.5rem]';

/** @deprecated Prefer width + rich/compact height. / 兼容旧引用：宽 + 丰富高度。 */
export const PACKAGE_APPLE_CARD_SIZE_CLASS = [
  PACKAGE_APPLE_CARD_WIDTH_CLASS,
  PACKAGE_APPLE_CARD_HEIGHT_RICH_CLASS,
].join(' ');
