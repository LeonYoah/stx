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

import {readFileSync} from 'node:fs';
import path from 'node:path';
import {describe, expect, it} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';

const fontsDir = path.resolve(__dirname, '../../../../public/fonts');

// 开屏欢迎语子集字体与文案一致性校验
// Guards that the splash greeting subset font matches the copy
describe('splash greeting font', () => {
  it('was generated from the current zh greeting', () => {
    const manifest = JSON.parse(
      readFileSync(path.join(fontsDir, 'stx-splash-greeting.json'), 'utf8'),
    ) as {text: string; weight: number};

    // 不一致时运行 scripts/fonts/build-splash-greeting-font.sh 重新生成。
    // On mismatch, re-run scripts/fonts/build-splash-greeting-font.sh.
    expect(manifest.text).toBe(zh.splash.welcomeBack);
    expect(manifest.weight).toBe(300);
  });

  it('ships a tiny woff2 with its OFL license', () => {
    const font = readFileSync(path.join(fontsDir, 'stx-splash-greeting.woff2'));
    expect(font.subarray(0, 4).toString('latin1')).toBe('wOF2');
    expect(font.byteLength).toBeLessThan(16 * 1024);
    expect(
      readFileSync(path.join(fontsDir, 'stx-splash-greeting.OFL.txt'), 'utf8'),
    ).toContain('SIL OPEN FONT LICENSE');
  });
});
