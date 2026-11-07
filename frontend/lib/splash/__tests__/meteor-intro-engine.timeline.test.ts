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
  ASSET_LOAD_TIMEOUT_MS,
  REDUCED_MOTION_HOLD_MS,
  SPLASH_TIMELINE,
} from '../meteor-intro-engine';

describe('SPLASH_TIMELINE (v4 prototype contract)', () => {
  it('matches the approved fourth-edition beats', () => {
    expect(SPLASH_TIMELINE.logoInEnd).toBe(1300);
    expect(SPLASH_TIMELINE.meteorEnd).toBe(2700);
    expect(SPLASH_TIMELINE.shatterEnd).toBe(4300);
    expect(SPLASH_TIMELINE.formEnd).toBe(6800);
    expect(SPLASH_TIMELINE.cleanTextEnd).toBe(7080);
    expect(SPLASH_TIMELINE.welcomeStart).toBe(7200);
    expect(SPLASH_TIMELINE.welcomeEnd).toBe(7800);
    expect(SPLASH_TIMELINE.lockupEnd).toBe(8600);
    expect(SPLASH_TIMELINE.total).toBe(9000);
  });

  it('keeps reduced-motion hold shorter than the full freeze', () => {
    expect(REDUCED_MOTION_HOLD_MS).toBeLessThan(SPLASH_TIMELINE.total);
    expect(REDUCED_MOTION_HOLD_MS).toBeGreaterThan(0);
  });

  it('keeps asset hang timeout finite and shorter than the full splash', () => {
    expect(ASSET_LOAD_TIMEOUT_MS).toBe(2500);
    expect(ASSET_LOAD_TIMEOUT_MS).toBeLessThan(SPLASH_TIMELINE.total);
  });
});
