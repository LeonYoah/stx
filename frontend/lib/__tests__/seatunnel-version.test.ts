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
import {nextEventChannelEnabled} from '@/lib/seatunnel-version';

describe('nextEventChannelEnabled', () => {
  it('defaults on for a supported version until the user changes it', () => {
    expect(nextEventChannelEnabled(true, false, false)).toBe(true);
    expect(nextEventChannelEnabled(true, true, false)).toBe(true);
  });

  it('keeps an explicit user choice on supported versions', () => {
    expect(nextEventChannelEnabled(true, false, true)).toBe(false);
    expect(nextEventChannelEnabled(true, true, true)).toBe(true);
  });

  it('turns the switch off when the version cannot use the channel', () => {
    expect(nextEventChannelEnabled(false, true, false)).toBe(false);
    expect(nextEventChannelEnabled(false, true, true)).toBe(false);
  });
});
