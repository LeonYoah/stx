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

import type {ReactNode} from 'react';

/** Table / card view mode for marketplace catalogs. / 市场目录的表格或卡片视图。 */
export type MarketplaceViewMode = 'table' | 'card';

export type MarketplaceStatusTone =
  | 'ok'
  | 'warn'
  | 'error'
  | 'info'
  | 'muted';

export interface MarketplaceBadge {
  label: string;
  highlight?: boolean;
}

export interface MarketplaceField {
  label: string;
  value: string;
}

export interface MarketplaceProgress {
  label: string;
  /** 0–100 progress value. / 0–100 进度值。 */
  value: number;
  display: string;
}

export interface MarketplaceStatus {
  label: string;
  tone: MarketplaceStatusTone;
}

/**
 * Unified list/card item for plugins and packages.
 * 插件与安装包共用的列表/卡片条目模型。
 */
export interface MarketplaceCatalogItem {
  id: string;
  title: string;
  subtitle: string;
  icon?: ReactNode;
  /** Optional leading control (e.g. checkbox). / 可选首列控件（如复选框）。 */
  leading?: ReactNode;
  badges?: MarketplaceBadge[];
  status?: MarketplaceStatus;
  fields?: MarketplaceField[];
  progress?: MarketplaceProgress | null;
  /** Trailing meta (relative time / size). / 末列元信息（相对时间 / 大小）。 */
  meta?: string;
  /** Full blurb shown on card back only. / 仅卡片背面展示的完整简介。 */
  description?: string;
  /** More-menu actions (DropdownMenuItem nodes). / 「…」菜单内的操作项。 */
  menuActions?: ReactNode;
  /** Visible primary actions (download / retry buttons). / 可见主操作（下载 / 重试按钮）。 */
  primaryActions?: ReactNode;
  /** Extra status-area node (e.g. cluster stack); card back in card view. / 状态区额外节点（如集群堆叠）；卡片视图放背面。 */
  aside?: ReactNode;
  /**
   * Open detail via title control only. Blank card area flips (does not open).
   * 仅标题控件打开详情；卡片空白区域用于翻转，不会打开详情。
   */
  onOpen?: () => void;
}
