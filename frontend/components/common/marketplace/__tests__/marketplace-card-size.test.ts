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

import {describe, expect, it} from 'vitest';
import {
  MARKETPLACE_PLUGIN_CARD_GRID_CLASS,
  MARKETPLACE_PLUGIN_CARD_HEIGHT_CLASS,
  PACKAGE_APPLE_CARD_HEIGHT_COMPACT_CLASS,
  PACKAGE_APPLE_CARD_HEIGHT_RICH_CLASS,
  PACKAGE_APPLE_CARD_WIDTH_CLASS,
} from '../marketplace-card-size';

describe('marketplace-card-size', () => {
  it('uses auto-fill min tracks so wide screens grow larger plugin cards', () => {
    expect(MARKETPLACE_PLUGIN_CARD_GRID_CLASS).toContain('auto-fill');
    expect(MARKETPLACE_PLUGIN_CARD_GRID_CLASS).toContain('minmax(18.5rem');
    expect(MARKETPLACE_PLUGIN_CARD_GRID_CLASS).toContain('minmax(21.5rem');
  });

  it('raises plugin flip height above the legacy 12.75rem band', () => {
    expect(MARKETPLACE_PLUGIN_CARD_HEIGHT_CLASS).toContain('15.25rem');
    expect(MARKETPLACE_PLUGIN_CARD_HEIGHT_CLASS).not.toContain('12.75rem');
  });

  it('narrows 1440 package cards toward ~3 per screen without a 40rem hero', () => {
    expect(PACKAGE_APPLE_CARD_WIDTH_CLASS).toContain('md:w-96');
    expect(PACKAGE_APPLE_CARD_WIDTH_CLASS).toContain('xl:w-[26rem]');
    expect(PACKAGE_APPLE_CARD_WIDTH_CLASS).toContain('calc(100vw-2.75rem)');
    expect(PACKAGE_APPLE_CARD_HEIGHT_RICH_CLASS).toContain('xl:h-[24.5rem]');
    expect(PACKAGE_APPLE_CARD_HEIGHT_COMPACT_CLASS).toContain('xl:h-[21.5rem]');
    expect(PACKAGE_APPLE_CARD_HEIGHT_RICH_CLASS).not.toContain('40rem');
  });
});
