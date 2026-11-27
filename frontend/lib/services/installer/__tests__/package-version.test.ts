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
  extractSeatunnelVersionFromPackageFileName,
  isRecommendedSeatunnelVersionFormat,
} from '../package-version';

describe('package version helpers', () => {
  it('extracts official and internal versions from SeaTunnel package names', () => {
    expect(
      extractSeatunnelVersionFromPackageFileName(
        'apache-seatunnel-2.3.13-bin.tar.gz',
      ),
    ).toBe('2.3.13');
    expect(
      extractSeatunnelVersionFromPackageFileName(
        'apache-seatunnel-3.1.0-hotfix.1-bin.tar.gz',
      ),
    ).toBe('3.1.0-hotfix.1');
    expect(
      extractSeatunnelVersionFromPackageFileName(
        'seatunnel-3.0.0-SNAPSHOT-bin.tar.gz',
      ),
    ).toBe('3.0.0-SNAPSHOT');
  });

  it('returns empty for names without a usable version', () => {
    expect(
      extractSeatunnelVersionFromPackageFileName('seatunnel-bin.tar.gz'),
    ).toBe('');
    expect(
      extractSeatunnelVersionFromPackageFileName('backup-2026.tar.gz'),
    ).toBe('');
  });

  it('checks the version format used for capability judgement', () => {
    expect(isRecommendedSeatunnelVersionFormat('3.1.0')).toBe(true);
    expect(isRecommendedSeatunnelVersionFormat('3.1.0-hotfix.1')).toBe(true);
    expect(isRecommendedSeatunnelVersionFormat('3.1')).toBe(false);
    expect(isRecommendedSeatunnelVersionFormat('internal-build')).toBe(false);
  });
});
