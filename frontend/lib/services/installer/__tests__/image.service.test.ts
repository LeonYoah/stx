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
import {resolveImageSeatunnelVersion} from '../image.service';

describe('resolveImageSeatunnelVersion', () => {
  it('registered image returns its recorded version', () => {
    expect(
      resolveImageSeatunnelVersion('corp/seatunnel:custom', [
        {image: 'corp/seatunnel:custom', seatunnel_version: '2.3.13'},
      ]),
    ).toBe('2.3.13');
  });
  it('extracts x.y.z tag', () => {
    expect(resolveImageSeatunnelVersion('apache/seatunnel:2.3.13')).toBe('2.3.13');
    expect(resolveImageSeatunnelVersion('apache/seatunnel:2.3.13-rc1')).toBe('2.3.13-rc1');
  });
  it('embedded version for seatunnel images without clean tag', () => {
    expect(resolveImageSeatunnelVersion('corp/seatunnel-2.3.13@sha256:abc')).toBe('2.3.13');
  });
  it('unrecognizable returns empty', () => {
    expect(resolveImageSeatunnelVersion('corp/seatunnel:latest')).toBe('');
    expect(resolveImageSeatunnelVersion('')).toBe('');
  });
});
