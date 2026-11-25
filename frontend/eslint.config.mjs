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

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const compat = new FlatCompat({
  baseDirectory: __dirname,
});

const eslintConfig = [
  // 基础配置
  ...compat.extends('next/core-web-vitals', 'next/typescript', 'google'),
  // prettier 配置必须放在最后，用于关闭所有与 prettier 冲突的规则
  ...compat.extends('prettier'),
  {
    rules: {
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
      ],
    },
  },
];

export default eslintConfig;
