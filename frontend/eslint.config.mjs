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
