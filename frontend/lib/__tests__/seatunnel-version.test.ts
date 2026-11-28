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
  nextEventChannelEnabled,
  resolveSeatunnelVersion,
  resolveSelectableSeatunnelVersions,
  resolveUpgradeTargetVersion,
} from '@/lib/seatunnel-version';
import type {PackageInfo} from '@/lib/services/installer/types';

function makeLocalPackage(version: string): PackageInfo {
  return {
    version,
    file_name: `apache-seatunnel-${version}-bin.tar.gz`,
    file_size: 1024,
    download_urls: {aliyun: '', apache: '', huaweicloud: ''},
    is_local: true,
    has_source: false,
    source_download_urls: {aliyun: '', apache: '', huaweicloud: ''},
  };
}

describe('resolveSelectableSeatunnelVersions', () => {
  it('merges manually uploaded intranet packages with catalog versions in descending order', () => {
    const versions = resolveSelectableSeatunnelVersions({
      versions: ['2.3.12', '2.3.11'],
      local_packages: [
        makeLocalPackage('2.3.9'),
        makeLocalPackage('3.1.0'),
        makeLocalPackage('3.2.0-custom'),
      ],
    });

    expect(versions).toEqual([
      '3.2.0-custom',
      '3.1.0',
      '2.3.12',
      '2.3.11',
      '2.3.9',
    ]);
  });

  it('deduplicates and preserves extra active versions', () => {
    const versions = resolveSelectableSeatunnelVersions(
      {
        versions: ['3.1.0', '2.3.12'],
        local_packages: [makeLocalPackage('3.1.0')],
      },
      ['3.0.0-internal', '3.1.0', '', undefined],
    );

    expect(versions).toEqual(['3.1.0', '3.0.0-internal', '2.3.12']);
  });

  it('returns an empty array when metadata is missing', () => {
    expect(resolveSelectableSeatunnelVersions(null)).toEqual([]);
  });
});

describe('resolveSeatunnelVersion', () => {
  it('prefers the newest local package over online/fallback catalog versions', () => {
    expect(
      resolveSeatunnelVersion({
        versions: ['3.1.0', '3.0.0', '2.3.13', '2.3.12'],
        recommended_version: '3.1.0',
        local_packages: [
          makeLocalPackage('2.3.12'),
          makeLocalPackage('2.3.13'),
        ],
      }),
    ).toBe('2.3.13');

    expect(
      resolveSeatunnelVersion({
        versions: ['2.3.12', '2.3.11'],
        recommended_version: '2.3.12',
        local_packages: [makeLocalPackage('3.1.0')],
      }),
    ).toBe('3.1.0');
  });

  it('falls back to the newest catalog version when no local packages exist', () => {
    expect(
      resolveSeatunnelVersion({
        versions: ['2.3.12', '3.1.0', '3.0.0'],
        recommended_version: '2.3.12',
        local_packages: [],
      }),
    ).toBe('3.1.0');
  });

  it('returns empty string when metadata has no versions', () => {
    expect(resolveSeatunnelVersion(null)).toBe('');
    expect(resolveSeatunnelVersion({versions: [], local_packages: []})).toBe(
      '',
    );
  });
});

describe('resolveUpgradeTargetVersion', () => {
  it('avoids selecting the cluster current version when a newer catalog version exists', () => {
    expect(
      resolveUpgradeTargetVersion(
        {
          versions: ['3.1.0', '3.0.0', '2.3.13', '2.3.12'],
          local_packages: [makeLocalPackage('2.3.13')],
        },
        '2.3.13',
      ),
    ).toBe('3.1.0');
  });

  it('prefers the newest local package strictly newer than the cluster current version', () => {
    expect(
      resolveUpgradeTargetVersion(
        {
          versions: ['3.1.0', '3.0.0', '2.3.13'],
          local_packages: [
            makeLocalPackage('2.3.13'),
            makeLocalPackage('3.0.0'),
          ],
        },
        '2.3.13',
      ),
    ).toBe('3.0.0');
  });

  it('does not downgrade when the cluster is already on the newest available version', () => {
    expect(
      resolveUpgradeTargetVersion(
        {
          versions: ['3.1.0', '2.3.13'],
          local_packages: [
            makeLocalPackage('3.1.0'),
            makeLocalPackage('2.3.13'),
          ],
        },
        '3.1.0',
      ),
    ).toBe('3.1.0');
  });

  it('keeps single-version fallback compatibility when no newer version is listed', () => {
    expect(
      resolveUpgradeTargetVersion(
        {
          versions: ['2.3.13'],
          local_packages: [makeLocalPackage('2.3.13')],
        },
        '2.3.13',
      ),
    ).toBe('2.3.13');
  });
});

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
