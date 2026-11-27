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

import {dirname} from 'path';
import {fileURLToPath} from 'url';
import {FlatCompat} from '@eslint/eslintrc';
import unusedImports from 'eslint-plugin-unused-imports';

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const compat = new FlatCompat({
  baseDirectory: __dirname,
});

const eslintConfig = [
  // 构建产物与生成物不参与 lint。
  // Build outputs and generated artifacts are not linted.
  {
    ignores: [
      '.next/**',
      'out/**',
      'dist/**',
      'node_modules/**',
      'coverage/**',
      '**/pack-standalone/**',
      'next-env.d.ts',
    ],
  },
  // 基础配置
  ...compat.extends('next/core-web-vitals', 'next/typescript', 'google'),
  // prettier 配置必须放在最后，用于关闭所有与 prettier 冲突的规则
  ...compat.extends('prettier'),
  {
    plugins: {'unused-imports': unusedImports},
    rules: {
      // 死 import 自动清除；未用变量保留人工判断（unused-imports 规则覆盖 base/ts 重复报告）
      'unused-imports/no-unused-imports': 'error',
      'no-unused-vars': 'off',
      '@typescript-eslint/no-unused-vars': ['error', {argsIgnorePattern: '^_', varsIgnorePattern: '^_', ignoreRestSiblings: true}],
      // 关闭 JSDoc 相关规则
      'valid-jsdoc': 'off',
      'require-jsdoc': 'off',
      // curly 规则：要求 if/else 等必须使用大括号（保持代码清晰）
      'linebreak-style': 'off', // 如果您是windows系统，可在开发时禁用行尾换行符检查
      curly: ['error', 'all'],
      // 复制规范硬约束：裸 clipboard 与复制反馈文案一律收口到底层组件，例外用 eslint-disable 注明理由
      // Copy conventions as hard rules: raw clipboard access and copy feedback labels stay inside the base components; exceptions need an eslint-disable with a reason
      'no-restricted-syntax': [
        'error',
        {
          selector: "MemberExpression[object.name='navigator'][property.name='clipboard']",
          message: '禁止裸调 navigator.clipboard：统一走 writeClipboard / Arc CopyButton（use-copy-feedback 除外），否则 HTTP+IP 非安全上下文下复制静默失败。',
        },
        {
          selector: "JSXAttribute[name.name='copiedLabel']",
          message: '禁止业务侧传 copiedLabel：反馈文案由 CopyButton 内部 i18n 统一（common.copied）。确需覆盖须 eslint-disable 并注明理由。',
        },
        {
          selector: "JSXAttribute[name.name='failedLabel']",
          message: '禁止业务侧传 failedLabel：反馈文案由 CopyButton 内部 i18n 统一（common.copyFailed）。确需覆盖须 eslint-disable 并注明理由。',
        },
        // 主机状态文案硬约束：禁止直接拼接 host.statuses.${status}，必须走 getHostStatusLabel(host, t) 按主机类型区分物理机与 K8s/Docker。
        // Host status label guard: forbid raw host.statuses.* interpolation; always use getHostStatusLabel(host, t).
        {
          selector: "CallExpression[callee.name='t'] TemplateLiteral[quasis.0.value.raw='host.statuses.']",
          message: '禁止直接拼接 host.statuses.${status}：请统一走 getHostStatusLabel(host, t) 按主机类型映射状态文案（避免 K8s/Docker pending 误显示「未安装」）。',
        },
        {
          selector: "CallExpression[callee.name='t'] BinaryExpression[operator='+'][left.value='host.statuses.']",
          message: '禁止直接拼接 "host.statuses." + status：请统一走 getHostStatusLabel(host, t) 按主机类型映射状态文案（避免 K8s/Docker pending 误显示「未安装」）。',
        },
        // 禁止对 i18n key 使用 as never 绕过类型检查（须用静态映射或 t.has）。
        // Forbid casting i18n keys with `as never`; use a static map or `t.has`.
        {
          selector: "CallExpression[callee.name='t'][arguments.0.type='TSAsExpression'][arguments.0.typeAnnotation.type='TSNeverKeyword']",
          message: '禁止对 t() 的第一个 key 参数使用 as never：请使用静态键映射或 t.has() 校验。',
        },
        // #178 联动列表硬约束：使用 DetailHost 的页面禁止混用 SortableDataTable，须走 TableCard + list-detail.module.css。
        // #178 linked list-detail guard: pages using DetailHost must not pair it with SortableDataTable; use TableCard + list-detail.module.css.
        {
          selector: "Program:has(ImportSpecifier[imported.name='DetailHost']) ImportDeclaration[source.value=/sortable-data-table/]",
          message: '禁止在使用 DetailHost 的页面中引入 SortableDataTable：请统一使用 TableCard + list-detail.module.css（.rowHead / .tableScroll > .row / .rowActive）以保证 #178 零间距联动与名称飞入。',
        },
        // 全局部署模式硬约束：禁止手写 infra_type !== 'bare_metal' / === 'bare_metal' / === 'kubernetes'，必须走 @/lib/infra helper，避免引入 YARN / Application 时误判。
        // Global deployment mode guard: forbid raw infra_type comparisons; always use @/lib/infra helpers so future YARN/Application shapes are never misclassified as K8s.
        {
          selector: "BinaryExpression[operator=/^(===|!==|==|!=)$/][left.property.name='infra_type'][right.value=/^(bare_metal|kubernetes)$/]",
          message: '禁止直接用 infra_type 与 bare_metal/kubernetes 比较：请统一调用 @/lib/infra（isK8sInfra / isBareMetalInfra / getClusterDeployMode 等），避免引入 YARN 时误判。',
        },
        {
          selector: "BinaryExpression[operator=/^(===|!==|==|!=)$/][right.property.name='infra_type'][left.value=/^(bare_metal|kubernetes)$/]",
          message: '禁止直接用 infra_type 与 bare_metal/kubernetes 比较：请统一调用 @/lib/infra（isK8sInfra / isBareMetalInfra / getClusterDeployMode 等），避免引入 YARN 时误判。',
        },
      ],
    },
  },
  // 测试与 e2e：partial mock 需要宽松 any（源码保持严格）
  // Tests and e2e use partial mocks that legitimately need loose any typing.
  {
    files: ['**/__tests__/**', '**/*.test.ts', '**/*.test.tsx', 'e2e/**'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
    },
  },
];

export default eslintConfig;
