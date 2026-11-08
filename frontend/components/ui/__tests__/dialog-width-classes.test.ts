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

import {readdirSync, readFileSync, statSync} from 'node:fs';
import path from 'node:path';
import {describe, expect, it} from 'vitest';

const FRONTEND_ROOT = path.resolve(__dirname, '../../..');
const SCAN_DIRS = ['components', 'app'];

function listTsx(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const full = path.join(dir, name);
    if (statSync(full).isDirectory()) {
      return name === '__tests__' || name === 'node_modules'
        ? []
        : listTsx(full);
    }
    return full.endsWith('.tsx') ? [full] : [];
  });
}

// 弹窗底座自带 sm:max-w-lg，调用处不带断点前缀的 max-w-* 在宽屏会被覆盖，
// 导致大弹窗被锁成 512px（安装向导步骤名被挤成「预检/查」）。规范要求写 sm:max-w-*。
// The dialog base ships sm:max-w-lg, so an unprefixed max-w-* at the call site
// is overridden on wide screens and large dialogs get stuck at 512px. The spec
// requires sm:max-w-*.
describe('DialogContent width classes', () => {
  it('never uses an unprefixed max-w-* that sm:max-w-lg would override', () => {
    const offenders: string[] = [];
    const element = /<(DialogContent|AlertDialogContent)\b([^>]*?)>/g;
    const unprefixed = /(?<![\w:!\-[])max-w-[\w[]/;
    for (const dir of SCAN_DIRS) {
      for (const file of listTsx(path.join(FRONTEND_ROOT, dir))) {
        if (file.includes(`${path.sep}components${path.sep}ui${path.sep}`)) {
          continue;
        }
        const source = readFileSync(file, 'utf8');
        for (const match of source.matchAll(element)) {
          const className =
            /className=('[^']*'|"[^"]*"|\{cn\([\s\S]*?\)\})/.exec(
              match[2],
            )?.[1] ?? '';
          if (unprefixed.test(className) && !className.includes('sm:max-w')) {
            const line = source.slice(0, match.index).split('\n').length;
            offenders.push(
              `${path.relative(FRONTEND_ROOT, file)}:${line} ${className}`,
            );
          }
        }
      }
    }
    expect(offenders).toEqual([]);
  });
});
