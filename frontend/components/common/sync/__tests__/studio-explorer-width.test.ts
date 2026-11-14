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

describe('studio explorer width contract', () => {
  const css = readFileSync(
    path.join(__dirname, '../studio-chrome.css'),
    'utf8',
  );

  it('stops Radix ScrollArea table layout from clipping tree badges', () => {
    expect(css).toContain(
      "[data-studio-left-sidebar] [data-slot='scroll-area-viewport'] > div",
    );
    expect(css).toContain('display: block !important');
    expect(css).toContain('min-width: 0 !important');
  });
});
