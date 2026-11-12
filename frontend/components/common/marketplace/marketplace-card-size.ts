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
  'h-[15.5rem] min-[480px]:h-[16rem] md:h-[16.75rem] xl:h-[17.5rem]';

/**
 * Plugin card grid: auto-fill with larger min tracks so wide screens grow
 * fewer, wider cards instead of tiny sparse columns.
 * 插件卡网格：更大最小轨宽的 auto-fill，宽屏变成更少但更宽的卡。
 */
export const MARKETPLACE_PLUGIN_CARD_GRID_CLASS =
  'grid gap-4 grid-cols-1 min-[420px]:grid-cols-2 md:grid-cols-[repeat(auto-fill,minmax(19rem,1fr))] xl:grid-cols-[repeat(auto-fill,minmax(21rem,1fr))] 2xl:grid-cols-[repeat(auto-fill,minmax(23rem,1fr))]';

/**
 * Package Apple carousel card — Inspira uses ~w-56/h-80 → md:w-96/h-[40rem].
 * Prefer a wider ladder than a hollow 40rem hero: STX cards are text/meta heavy.
 * 安装包 Apple 轮播卡：对齐 Inspira 大卡思路但偏宽；避免无图 40rem 空洞。
 */
export const PACKAGE_APPLE_CARD_SIZE_CLASS = [
  // 390：用视口宽度显式定宽（勿用 %，shrink-0 父级会导致 100% 塌缩）。
  // ~390: explicit viewport width (% collapses under shrink-0 parents).
  'h-[23rem] w-[calc(100vw-2.75rem)] max-w-[24rem]',
  'sm:h-[24rem] sm:w-[24rem] sm:max-w-none',
  // 768+：宽于 Inspira md:w-96，高度适中由内容区填满。
  // 768+: wider than Inspira md:w-96; moderate height filled by content.
  'md:h-[26rem] md:w-[28rem]',
  'lg:h-[27rem] lg:w-[30rem]',
  // 1440：大卡吃满横向，约 2～3 张可见。
  // 1440: large cards fill horizontally (~2–3 visible).
  'xl:h-[28rem] xl:w-[32rem]',
].join(' ');
