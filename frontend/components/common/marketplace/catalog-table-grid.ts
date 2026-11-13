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
 * Shared CSS grid / subgrid tokens for MorphCatalog tables.
 * Parent defines tracks once; header + each row use `grid-cols-subgrid` so edges cannot drift.
 * 表头与行通过 subgrid 共用父级列轨，避免两套独立 grid 错位。
 */

/** Action column fits「下载到服务器」+ optional「…」(~9rem). / 操作列容纳下载按钮与「…」。 */
export const CATALOG_ACTION_COL = '9rem';

/** Selection checkbox column. / 多选勾选列。 */
export const CATALOG_SELECT_COL = '2.5rem';

/** Parent column tracks (no selection). / 父级列轨（无勾选）。 */
export const CATALOG_TABLE_COLUMNS =
  'grid-cols-[minmax(0,2fr)_minmax(0,1.1fr)_minmax(0,1.2fr)_minmax(0,0.8fr)_9rem]';

/** Parent column tracks with selection. / 父级列轨（含勾选）。 */
export const CATALOG_TABLE_COLUMNS_SELECTABLE =
  'grid-cols-[2.5rem_minmax(0,2fr)_minmax(0,1.1fr)_minmax(0,1.2fr)_minmax(0,0.8fr)_9rem]';

/**
 * Row/header strip: subgrid only — column gap lives on the parent so tracks stay shared.
 * 行/表头条仅用 subgrid；列间距放在父级，避免子级自带 gap 拆散列轨。
 */
export const CATALOG_TABLE_GRID =
  'col-span-full grid grid-cols-subgrid items-center';
export const CATALOG_TABLE_GRID_SELECTABLE = CATALOG_TABLE_GRID;

/** Min width so narrow viewports scroll instead of collapsing columns. / 窄屏横向滚动，避免列塌缩。 */
export const CATALOG_TABLE_MIN_WIDTH = 'min-w-[44rem]';
export const CATALOG_TABLE_MIN_WIDTH_SELECTABLE = 'min-w-[48rem]';
