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

// UI 约定静态门禁（对应 .trellis/spec/frontend/ui-conventions.md）：
// 存量未迁移文件使用棘轮白名单（只减不增），新增与已迁移域一律硬性拦截。
// Static UI-convention guardrails (see .trellis/spec/frontend/ui-conventions.md):
// unmigrated legacy files stay on a shrinking ratchet allowlist; new and migrated
// code is strictly enforced in CI.

import {existsSync, readdirSync, readFileSync, statSync} from 'node:fs';
import path from 'node:path';
import {describe, expect, it} from 'vitest';

const REPO_ROOT = path.resolve(__dirname, '../../../../..');
const FRONTEND_ROOT = path.join(REPO_ROOT, 'frontend');
const SCAN_DIRS = ['components', 'app'];

function listFiles(dir: string, exts: readonly string[]): string[] {
  if (!existsSync(dir)) {
    return [];
  }
  return readdirSync(dir).flatMap((name) => {
    if (name === '__tests__' || name === 'node_modules' || name === '.next') {
      return [];
    }
    const full = path.join(dir, name);
    if (statSync(full).isDirectory()) {
      return listFiles(full, exts);
    }
    return exts.some((ext) => full.endsWith(ext)) ? [full] : [];
  });
}

function frontendRel(absPath: string): string {
  return path.relative(FRONTEND_ROOT, absPath).split(path.sep).join('/');
}

function repoRel(absPath: string): string {
  return path.relative(REPO_ROOT, absPath).split(path.sep).join('/');
}

const SOURCE_FILES = SCAN_DIRS.flatMap((dir) =>
  listFiles(path.join(FRONTEND_ROOT, dir), ['.tsx', '.ts']),
);

// 棘轮校验：禁止新增违规；白名单内文件清理完毕后须同步移出名单。
// Ratchet assertion: reject any new offender and flag stale allowlist entries once cleaned up.
function expectRatchet(offenders: string[], allowlist: readonly string[]) {
  const allowed = new Set(allowlist);
  const found = new Set(offenders);
  const unexpected = offenders.filter((item) => !allowed.has(item));
  const stale = allowlist.filter((item) => !found.has(item));
  expect(unexpected, '新增违规文件（请按 ui-conventions.md 修正）').toEqual([]);
  expect(stale, '以下文件已合规，请从白名单中移除以锁紧门禁').toEqual([]);
}

describe('UI conventions static guardrails', () => {
  it('已弃用的 StatPillsBar 禁止新增引用', () => {
    const LEGACY_STAT_PILLS_ALLOWLIST = [
      'components/common/cluster/ClusterConfigs.tsx',
      'components/common/cluster/ClusterDetail.tsx',
      'components/common/cluster/ClusterDetailSkeleton.tsx',
      'components/common/cluster/ClusterMain.tsx',
      'components/common/installer/PackageMain.tsx',
      'components/common/plugin/PluginMain.tsx',
      'components/common/sync/job-table/JobTableToolbar.tsx',
      'components/common/troubleshooting/TroubleshootingMemoryCenter.tsx',
    ];

    const offenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const rel = frontendRel(file);
      if (
        rel === 'components/common/layout/StatPillsBar.tsx' ||
        rel === 'components/common/layout/index.ts'
      ) {
        continue;
      }
      const source = readFileSync(file, 'utf8');
      if (/\bStatPillsBar\b/.test(source)) {
        offenders.push(rel);
      }
    }

    expectRatchet(offenders.sort(), [...LEGACY_STAT_PILLS_ALLOWLIST].sort());
  });

  it('Arc 弹窗 / TableCard / DetailHost 页面禁止混用 shadcn 同类交互控件', () => {
    const LEGACY_MIXED_ALLOWLIST = [
      'components/common/diagnostics/inspections/InspectionReportDetail.tsx',
      'components/common/host/DiscoverClusterDialog.tsx',
      'components/common/sync/CheckpointPanels.tsx',
      'components/common/sync/CuratedTemplatesManagePanel.tsx',
      'components/common/sync/DataSyncStudio.tsx',
      'components/common/sync/StudioDialogs.tsx',
      'components/common/sync/TaskAlertSection.tsx',
      'components/common/troubleshooting/SaveMemoryDialog.tsx',
    ];

    const offenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const rel = frontendRel(file);
      if (rel.startsWith('components/common/layout/')) {
        continue;
      }
      const source = readFileSync(file, 'utf8');
      const usesArcShell =
        /@\/components\/arc\/dialog\/dialog/.test(source) ||
        /\b(TableCard|DetailHost)\b/.test(source);
      if (!usesArcShell) {
        continue;
      }
      // 豁免 Arc 未封装的 textarea / alert-dialog / badge / tooltip / progress / skeleton。
      // Exempt controls without an Arc equivalent (textarea, alert-dialog, badge, tooltip, etc.).
      if (
        /@\/components\/ui\/(button|input|select|switch|checkbox|toggle-group)\b/.test(
          source,
        )
      ) {
        offenders.push(rel);
      }
    }

    expectRatchet(offenders.sort(), [...LEGACY_MIXED_ALLOWLIST].sort());
  });

  it('全站禁止 window.confirm 与 window.alert', () => {
    const offenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const source = readFileSync(file, 'utf8');
      if (/\bwindow\.(confirm|alert)\s*\(/.test(source)) {
        offenders.push(frontendRel(file));
      }
    }
    expect(offenders).toEqual([]);
  });

  it('时间格式化走 lib/timezone，禁止新增 new Date(...).toLocale*String()', () => {
    const LEGACY_DATE_LOCALE_ALLOWLIST = [
      'components/common/self-upgrade/SystemUpgradeMain.tsx',
    ];

    const offenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const source = readFileSync(file, 'utf8');
      if (/new\s+Date\([^)]*\)\.toLocale(?:Date|Time)?String\s*\(/.test(source)) {
        offenders.push(frontendRel(file));
      }
    }

    expectRatchet(offenders.sort(), [...LEGACY_DATE_LOCALE_ALLOWLIST].sort());
  });

  it('DetailHost 详情体保持单层扁平，禁止嵌套卡片边框与阴影（防框套框）', () => {
    const LEGACY_DETAIL_NEST_ALLOWLIST = [
      'components/common/diagnostics/policies/PolicyFormBody.tsx',
    ];

    const offenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const source = readFileSync(file, 'utf8');
      if (!/layout\.detailScroll/.test(source)) {
        continue;
      }
      if (
        /@\/components\/ui\/card\b/.test(source) ||
        /rounded-(?:lg|xl|md)[^'"`\n]*\bborder\b[^'"`\n]*(?:bg-card|bg-background|shadow-)/.test(
          source,
        ) ||
        /\bshadow-(?:sm|md|lg|xl|2xl)\b/.test(source)
      ) {
        offenders.push(frontendRel(file));
      }
    }

    expectRatchet(offenders.sort(), [...LEGACY_DETAIL_NEST_ALLOWLIST].sort());
  });

  it('列表行尾编辑+删除操作统一走 Arc DropdownMenu，禁止手拼图标按钮组', () => {
    const offenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const source = readFileSync(file, 'utf8');
      if (
        /layout\.row\b/.test(source) &&
        /\bPencil\b/.test(source) &&
        /\bTrash2\b/.test(source) &&
        !/@\/components\/arc\/dropdown-menu\/dropdown-menu/.test(source)
      ) {
        offenders.push(frontendRel(file));
      }
    }
    expect(offenders).toEqual([]);
  });

  it('list-detail.module.css 契约：禁止内联 --cols，且每个 data-mcols 均有样式定义', () => {
    const css = readFileSync(
      path.join(FRONTEND_ROOT, 'components/common/layout/list-detail.module.css'),
      'utf8',
    );
    const inlineOffenders: string[] = [];
    const missingMcols: string[] = [];

    for (const file of SOURCE_FILES) {
      const source = readFileSync(file, 'utf8');
      if (/--cols\s*:/.test(source)) {
        inlineOffenders.push(frontendRel(file));
      }
      for (const match of source.matchAll(/data-mcols=['"]([^'"]+)['"]/g)) {
        const key = match[1];
        if (
          !css.includes(`data-mcols='${key}'`) &&
          !css.includes(`data-mcols="${key}"`)
        ) {
          missingMcols.push(`${frontendRel(file)} -> data-mcols="${key}"`);
        }
      }
    }

    expect(inlineOffenders).toEqual([]);
    expect(missingMcols).toEqual([]);
  });

  it('Arc Select 与 Combobox 必须具备可见 label 或 aria-label（控件不得无名）', () => {
    const offenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const source = readFileSync(file, 'utf8');
      if (!/@\/components\/arc\/(select|combobox)\//.test(source)) {
        continue;
      }
      for (const match of source.matchAll(
        /<(Select|Combobox)\b([^>]*?)(?:\/>|>)/g,
      )) {
        const attrs = match[2];
        if (
          !/\blabel\s*=/.test(attrs) &&
          !/\baria-label\s*=/.test(attrs) &&
          !/\baria-labelledby\s*=/.test(attrs)
        ) {
          const line = source.slice(0, match.index).split('\n').length;
          offenders.push(`${frontendRel(file)}:${line} <${match[1]}>`);
        }
      }
    }
    expect(offenders).toEqual([]);
  });

  it('Arc 组件全部登记在 third_party_mit_files.txt 且禁止引入 @uiarc-pro', () => {
    const mitList = new Set(
      readFileSync(
        path.join(REPO_ROOT, 'licenses/third_party_mit_files.txt'),
        'utf8',
      )
        .split(/\r?\n/)
        .map((line) => line.trim())
        .filter((line) => line && !line.startsWith('#')),
    );

    const arcFiles = listFiles(path.join(FRONTEND_ROOT, 'components/arc'), [
      '.tsx',
      '.ts',
    ]).map(repoRel);
    const unregistered = arcFiles.filter((rel) => !mitList.has(rel));
    expect(unregistered).toEqual([]);

    const proOffenders: string[] = [];
    for (const file of SOURCE_FILES) {
      const source = readFileSync(file, 'utf8');
      if (/@uiarc-pro\b/.test(source)) {
        proOffenders.push(frontendRel(file));
      }
    }
    expect(proOffenders).toEqual([]);
  });
});
