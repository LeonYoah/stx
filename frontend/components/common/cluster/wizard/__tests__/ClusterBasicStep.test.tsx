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

/**
 * ClusterBasicStep：验证内网手动上传的本地安装包版本可选，且不再渲染冗余的推荐版本徽标。
 * ClusterBasicStep: verifies manually uploaded offline versions are selectable and no recommended badge renders.
 */

import React from 'react';
import {afterEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {ClusterBasicStep} from '../ClusterBasicStep';
import {defaultClusterDeployConfig} from '../types';
import type {AvailableVersions, PackageInfo} from '@/lib/services/installer/types';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('@/components/arc/select/select', () => ({
  Select: ({
    label,
    value,
    displayValue,
    onValueChange,
    options,
    'data-testid': dataTestId,
  }: {
    label?: string;
    value?: string;
    displayValue?: string;
    onValueChange?: (next: string) => void;
    options: Array<{value: string; label: string}>;
    'data-testid'?: string;
  }) => (
    <label>
      <span>{label}</span>
      <span data-testid='select-display-value'>{displayValue ?? value}</span>
      <select
        data-testid={dataTestId}
        value={value}
        onChange={(event) => onValueChange?.(event.target.value)}
      >
        {options.map((option) => (
          <option key={option.value} value={option.value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  ),
}));

afterEach(() => {
  cleanup();
});

const local310: PackageInfo = {
  version: '3.1.0',
  file_name: 'apache-seatunnel-3.1.0-bin.tar.gz',
  file_size: 256 * 1024 * 1024,
  download_urls: {aliyun: '', apache: '', huaweicloud: ''},
  is_local: true,
  has_source: false,
  source_download_urls: {aliyun: '', apache: '', huaweicloud: ''},
};

describe('ClusterBasicStep version selection', () => {
  it('exposes manually uploaded local versions missing from fallback catalog and omits recommended chip', () => {
    const updateConfig = vi.fn();
    const packages: AvailableVersions = {
      versions: ['2.3.12', '2.3.11'],
      recommended_version: '2.3.12',
      local_packages: [local310],
      version_capabilities: {},
    };

    render(
      <ClusterBasicStep
        config={{...defaultClusterDeployConfig, version: '3.1.0'}}
        updateConfig={updateConfig}
        packages={packages}
        localPackages={[local310]}
        packagesLoading={false}
        defaultVersion='3.1.0'
      />,
    );

    const select = screen.getByTestId(
      'cluster-wizard-version-select',
    ) as HTMLSelectElement;
    const values = Array.from(select.options).map((opt) => opt.value);
    const labels = Array.from(select.options).map((opt) => opt.textContent);

    expect(values).toEqual(['3.1.0', '2.3.12', '2.3.11']);
    expect(labels[0]).toBe('3.1.0 · cluster.wizard.localReady');
    expect(screen.getByTestId('select-display-value')).toHaveTextContent(
      '3.1.0',
    );
    expect(screen.getByText('cluster.wizard.localReady')).toBeInTheDocument();
    expect(screen.queryByText('installer.recommended')).not.toBeInTheDocument();

    fireEvent.change(select, {target: {value: '2.3.12'}});
    expect(updateConfig).toHaveBeenCalledWith({version: '2.3.12'});
  });
});
