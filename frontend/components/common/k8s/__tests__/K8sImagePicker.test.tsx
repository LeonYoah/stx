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

// K8sImagePicker 组件单元测试：覆盖默认 docker.io 预填、350ms 防抖探测、
// 智能粘贴自动吸附（已登记仓库 vs Route A 自定义镜像）、升级预填与单版本输入契约。
// Unit tests for K8sImagePicker: default docker.io hydration, 350ms debounced tag
// probing, smart-paste auto-snap (registered vs custom Route A), upgrade initialImage
// prefill, and single unified engine-version field contract.

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {K8sImagePicker} from '../K8sImagePicker';

const registryMocks = vi.hoisted(() => ({
  listContainerRegistries: vi.fn(),
  listRegistryTags: vi.fn(),
}));

vi.mock('@/lib/services/installer/registry.service', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/services/installer/registry.service')>();
  return {
    ...actual,
    listContainerRegistries: registryMocks.listContainerRegistries,
    listRegistryTags: registryMocks.listRegistryTags,
  };
});

const REGISTRIES = [
  {id: 1, host: 'docker.io', default_repo: 'apache/seatunnel', is_system: true, probe_status: 'public'},
  {
    id: 2,
    host: 'harbor.corp.local',
    default_repo: 'data/seatunnel',
    is_system: false,
    has_auth: true,
    probe_status: 'ok',
  },
];

describe('K8sImagePicker', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    registryMocks.listContainerRegistries.mockResolvedValue(REGISTRIES);
    registryMocks.listRegistryTags.mockResolvedValue(['3.1.1', '3.1.0']);
  });

  it('defaults to docker.io, fetches tags after debounce, and emits {image, seatunnelVersion} only', async () => {
    const onChange = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <K8sImagePicker value='' version='' onChange={onChange} />
      </NextIntlClientProvider>,
    );

    const repoInput = await screen.findByTestId('k8s-image-picker-repo-input');
    expect(repoInput).toHaveValue('apache/seatunnel');
    expect(screen.getByTestId('k8s-version-input')).toBeInTheDocument();

    // 等 350ms 防抖后触发 listRegistryTags / Wait for 350ms debounce to fetch tags
    await waitFor(
      () => {
        expect(registryMocks.listRegistryTags).toHaveBeenCalledWith(1, 'apache/seatunnel');
      },
      {timeout: 1500},
    );

    // 切回手动 tag 输入并输入版本，验证 onChange 不含 pullPolicy/pullSecrets
    // Switch to manual tag input and check emitted payload omits pullPolicy/pullSecrets
    fireEvent.click(await screen.findByTestId('k8s-image-picker-tag-toggle'));
    fireEvent.change(screen.getByTestId('k8s-image-picker-tag-input'), {target: {value: '3.2.0'}});

    const lastCall = onChange.mock.calls.at(-1)?.[0];
    expect(lastCall).toEqual({
      image: 'apache/seatunnel:3.2.0',
      seatunnelVersion: '3.2.0',
    });
    expect(lastCall).not.toHaveProperty('pullPolicy');
    expect(lastCall).not.toHaveProperty('pullSecrets');
  });

  it('snaps pasted registered image to registry mode and unregistered image to custom mode', async () => {
    const onChange = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <K8sImagePicker value='' onChange={onChange} />
      </NextIntlClientProvider>,
    );

    const pasteInput = await screen.findByTestId('k8s-image-picker-paste');
    fireEvent.change(pasteInput, {target: {value: 'harbor.corp.local/data/seatunnel:3.1.1'}});

    expect(screen.getByTestId('k8s-image-picker-paste-notice')).toHaveTextContent('harbor.corp.local');
    expect(onChange).toHaveBeenLastCalledWith({
      image: 'harbor.corp.local/data/seatunnel:3.1.1',
      seatunnelVersion: '3.1.1',
    });

    // 粘贴未登记仓库镜像 -> 自动切换自定义模式（Route A）
    // Paste unregistered image -> snaps to custom mode (Route A)
    fireEvent.change(pasteInput, {target: {value: 'ghcr.io/custom/seatunnel:3.2.0'}});
    expect(await screen.findByTestId('k8s-image-picker-custom-input')).toHaveValue('ghcr.io/custom/seatunnel:3.2.0');
    expect(onChange).toHaveBeenLastCalledWith({
      image: 'ghcr.io/custom/seatunnel:3.2.0',
      seatunnelVersion: '3.2.0',
    });
  });

  it('prefills registry and repo from initialImage while leaving tag clear for upgrades', async () => {
    const onChange = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <K8sImagePicker
          value=''
          initialImage='apache/seatunnel:3.0.0'
          onChange={onChange}
        />
      </NextIntlClientProvider>,
    );

    const repoInput = await screen.findByTestId('k8s-image-picker-repo-input');
    expect(repoInput).toHaveValue('apache/seatunnel');
    expect(screen.getByTestId('k8s-image-picker-tag-input')).toHaveValue('');
  });

  it('warns when custom tag has no deterministic semver', async () => {
    const onChange = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <K8sImagePicker value='' onChange={onChange} />
      </NextIntlClientProvider>,
    );

    fireEvent.click(await screen.findByTestId('k8s-image-picker-mode-custom'));
    fireEvent.change(screen.getByTestId('k8s-image-picker-custom-input'), {
      target: {value: 'ghcr.io/custom/seatunnel:latest'},
    });

    expect(screen.getByTestId('k8s-image-picker-version-warning')).toBeInTheDocument();
    fireEvent.change(screen.getByTestId('k8s-version-input'), {target: {value: '3.1.0'}});
    expect(screen.queryByTestId('k8s-image-picker-version-warning')).not.toBeInTheDocument();
  });

  it('prefills configured default_repo and probes tags when selecting a private registry', async () => {
    const onChange = vi.fn();
    registryMocks.listContainerRegistries.mockResolvedValue([REGISTRIES[1]]);
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <K8sImagePicker value='' version='' onChange={onChange} />
      </NextIntlClientProvider>,
    );

    const repoInput = await screen.findByTestId('k8s-image-picker-repo-input');
    expect(repoInput).toHaveValue('data/seatunnel');

    await waitFor(
      () => {
        expect(registryMocks.listRegistryTags).toHaveBeenCalledWith(2, 'data/seatunnel');
      },
      {timeout: 1500},
    );
  });
});
