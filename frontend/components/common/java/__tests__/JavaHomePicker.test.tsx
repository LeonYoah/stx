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

import {useState} from 'react';
import {describe, it, expect, vi, beforeEach} from 'vitest';
import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {JavaHomePicker, type JavaHomePickerValue} from '../JavaHomePicker';

const listJavaRuntimes = vi.fn();

vi.mock('@/lib/services', () => ({
  default: {
    installer: {
      listJavaRuntimes: (...args: unknown[]) => listJavaRuntimes(...args),
    },
  },
}));

const messages = {
  javaHome: {
    label: 'JDK path',
    help: 'help text',
    placeholder: 'Select JDK',
    optionAuto: 'Auto detect',
    optionDetected: '{path} (Java {version})',
    optionCustom: 'Custom path…',
    customPlaceholder: '/usr/lib/jvm/java-11',
    versionUnknown: '?',
    loadFailed: 'Could not load the JDK list',
    badgeCustom: 'Custom',
    badgeDetected: 'Detected',
    badgeAuto: 'Auto',
  },
  common: {
    helpLabel: 'Help',
    helpLabelFor: '{label} help',
  },
};

function wrap(ui: React.ReactElement) {
  return (
    <NextIntlClientProvider locale='en' messages={messages}>
      {ui}
    </NextIntlClientProvider>
  );
}

describe('JavaHomePicker', () => {
  beforeEach(() => {
    listJavaRuntimes.mockReset();
    listJavaRuntimes.mockResolvedValue([
      {
        java_home: '/usr/lib/jvm/java-11',
        version: 11,
        version_string: '11.0.2',
      },
      {java_home: '/opt/java/jdk8', version: 8, version_string: '1.8.0'},
    ]);
  });

  it('probes host JDK list on mount and preselects the first concrete JDK (no auto option)', async () => {
    const onChange = vi.fn();
    render(
      wrap(<JavaHomePicker hostId={1} value='' onChange={onChange} id='jdk' />),
    );
    await waitFor(() => expect(listJavaRuntimes).toHaveBeenCalledWith(1));
    expect(screen.getByText('JDK path')).toBeInTheDocument();
    expect(screen.queryByText('Auto')).not.toBeInTheDocument();
    expect(screen.queryByText('Auto detect')).not.toBeInTheDocument();
    expect(onChange).toHaveBeenCalledWith({
      javaHome: '/usr/lib/jvm/java-11',
      isCustom: false,
      isAutoPrefill: true,
    });
    fireEvent.click(screen.getByRole('combobox'));
    expect(screen.queryByText('Auto detect')).not.toBeInTheDocument();
    expect(
      screen.getByText('/usr/lib/jvm/java-11 (Java 11)'),
    ).toBeInTheDocument();
  });

  it('hides the field label when hideLabel is set', async () => {
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={1}
          value='/usr/lib/jvm/java-11'
          onChange={onChange}
          hideLabel
          id='jdk-compact'
        />,
      ),
    );
    await waitFor(() => expect(listJavaRuntimes).toHaveBeenCalledWith(1));
    expect(screen.queryByText('JDK path')).not.toBeInTheDocument();
    expect(await screen.findByText('Detected')).toBeInTheDocument();
  });

  it('keeps placeholder without auto-selecting when autoSelectFirst is false', async () => {
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={1}
          value=''
          onChange={onChange}
          hideLabel
          autoSelectFirst={false}
          id='jdk-upgrade'
        />,
      ),
    );
    await waitFor(() => expect(listJavaRuntimes).toHaveBeenCalledWith(1));
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.queryByText('Auto')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('combobox'));
    expect(screen.queryByText('Auto detect')).not.toBeInTheDocument();
    expect(
      screen.getByText('/usr/lib/jvm/java-11 (Java 11)'),
    ).toBeInTheDocument();
  });

  it('prefills process JDK as custom when not in list (AC1f)', async () => {
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={2}
          value=''
          preferredJavaHome='/proc/special/jdk'
          onChange={onChange}
        />,
      ),
    );
    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith({
        javaHome: '/proc/special/jdk',
        isCustom: true,
        isAutoPrefill: true,
      }),
    );
    expect(await screen.findByText('Custom')).toBeInTheDocument();
  });

  it('does not block when list is empty (AC1b)', async () => {
    listJavaRuntimes.mockResolvedValue([]);
    const onChange = vi.fn();
    render(wrap(<JavaHomePicker hostId={3} value='' onChange={onChange} />));
    await waitFor(() => expect(listJavaRuntimes).toHaveBeenCalled());
    expect(screen.getByRole('combobox')).toBeInTheDocument();
  });

  it('shows a light loadFailed tip when JDK list fetch fails', async () => {
    listJavaRuntimes.mockRejectedValue(new Error('network'));
    const onChange = vi.fn();
    render(wrap(<JavaHomePicker hostId={5} value='' onChange={onChange} />));
    expect(
      await screen.findByText('Could not load the JDK list'),
    ).toBeInTheDocument();
  });

  it('marks a detected list value with Detected badge (AC1b)', async () => {
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={4}
          value='/usr/lib/jvm/java-11'
          onChange={onChange}
        />,
      ),
    );
    await waitFor(() => expect(listJavaRuntimes).toHaveBeenCalled());
    expect(await screen.findByText('Detected')).toBeInTheDocument();
  });

  it('prefers JDK 17 and sorts compatible JDKs ahead of JDK 8/25 for SeaTunnel 3.1.0', async () => {
    listJavaRuntimes.mockResolvedValue([
      {java_home: '/jdk/8', version: 8, version_string: '1.8.0_402'},
      {java_home: '/jdk/11', version: 11, version_string: '11.0.22'},
      {java_home: '/jdk/17', version: 17, version_string: '17.0.10'},
      {java_home: '/jdk/21', version: 21, version_string: '21.0.2'},
      {java_home: '/jdk/25', version: 25, version_string: '25'},
    ]);
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={10}
          value=''
          seatunnelVersion='3.1.0'
          onChange={onChange}
          id='jdk-310'
        />,
      ),
    );
    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith({
        javaHome: '/jdk/17',
        isCustom: false,
        isAutoPrefill: true,
      }),
    );
    fireEvent.click(screen.getByRole('combobox'));
    const optionTexts = screen
      .getAllByRole('option')
      .map((node) => node.textContent);
    expect(optionTexts).toEqual([
      '/jdk/17 (Java 17)',
      '/jdk/11 (Java 11)',
      '/jdk/21 (Java 21)',
      '/jdk/8 (Java 8)',
      '/jdk/25 (Java 25)',
      'Custom path…',
    ]);
  });

  it('falls back to supported warning JDK 21 when only 8, 21, 25 exist for SeaTunnel 3.1.0', async () => {
    listJavaRuntimes.mockResolvedValue([
      {java_home: '/jdk/8', version: 8, version_string: '1.8.0_402'},
      {java_home: '/jdk/21', version: 21, version_string: '21.0.2'},
      {java_home: '/jdk/25', version: 25, version_string: '25'},
    ]);
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={11}
          value=''
          seatunnelVersion='3.1.0'
          onChange={onChange}
        />,
      ),
    );
    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith({
        javaHome: '/jdk/21',
        isCustom: false,
        isAutoPrefill: true,
      }),
    );
  });

  it('does not auto-select incompatible JDK 8 or JDK 25 on SeaTunnel 3.1.0', async () => {
    listJavaRuntimes.mockResolvedValue([
      {java_home: '/jdk/8', version: 8, version_string: '1.8.0_402'},
      {java_home: '/jdk/25', version: 25, version_string: '25'},
    ]);
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={12}
          value=''
          seatunnelVersion='3.1.0'
          onChange={onChange}
        />,
      ),
    );
    await waitFor(() => expect(listJavaRuntimes).toHaveBeenCalledWith(12));
    expect(onChange).not.toHaveBeenCalled();
  });

  it('selects JDK 8 and skips JDK 25 for SeaTunnel 3.0.0', async () => {
    listJavaRuntimes.mockResolvedValue([
      {java_home: '/jdk/8', version: 8, version_string: '1.8.0_402'},
      {java_home: '/jdk/25', version: 25, version_string: '25'},
    ]);
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker
          hostId={13}
          value=''
          seatunnelVersion='3.0.0'
          onChange={onChange}
        />,
      ),
    );
    await waitFor(() =>
      expect(onChange).toHaveBeenCalledWith({
        javaHome: '/jdk/8',
        isCustom: false,
        isAutoPrefill: true,
      }),
    );
  });

  it('upgrades auto-prefilled JDK on version switch from 3.0.0 to 3.1.0 without overwriting manual picks', async () => {
    listJavaRuntimes.mockResolvedValue([
      {java_home: '/jdk/8', version: 8, version_string: '1.8.0_402'},
      {java_home: '/jdk/17', version: 17, version_string: '17.0.10'},
    ]);
    const latestChanges: JavaHomePickerValue[] = [];
    function ControlledPicker({
      hostId,
      seatunnelVersion,
    }: {
      hostId: number;
      seatunnelVersion: string;
    }) {
      const [val, setVal] = useState('');
      return (
        <JavaHomePicker
          hostId={hostId}
          value={val}
          seatunnelVersion={seatunnelVersion}
          onChange={(next) => {
            latestChanges.push(next);
            setVal(next.javaHome);
          }}
        />
      );
    }

    // 场景 A：自动预填后切换版本，自动从 JDK 8 升级为 JDK 17。
    // Case A: an auto-prefilled JDK 8 upgrades to JDK 17 when version switches to 3.1.0.
    const {rerender, unmount} = render(
      wrap(<ControlledPicker hostId={14} seatunnelVersion='3.0.0' />),
    );
    await waitFor(() =>
      expect(latestChanges.at(-1)?.javaHome).toBe('/jdk/8'),
    );
    rerender(wrap(<ControlledPicker hostId={14} seatunnelVersion='3.1.0' />));
    await waitFor(() =>
      expect(latestChanges.at(-1)).toEqual({
        javaHome: '/jdk/17',
        isCustom: false,
        isAutoPrefill: true,
      }),
    );
    unmount();

    // 场景 B：用户显式手动选中 JDK 8 后切换版本，不得覆盖用户选择。
    // Case B: an explicit manual pick stays untouched when version switches to 3.1.0.
    latestChanges.length = 0;
    const manualRender = render(
      wrap(<ControlledPicker hostId={15} seatunnelVersion='3.0.0' />),
    );
    await waitFor(() =>
      expect(latestChanges.at(-1)?.javaHome).toBe('/jdk/8'),
    );
    fireEvent.click(screen.getByRole('combobox'));
    fireEvent.click(await screen.findByRole('option', {name: '/jdk/17 (Java 17)'}));
    fireEvent.click(screen.getByRole('combobox'));
    fireEvent.click(await screen.findByRole('option', {name: '/jdk/8 (Java 8)'}));
    await waitFor(() =>
      expect(latestChanges.at(-1)).toEqual({
        javaHome: '/jdk/8',
        isCustom: false,
      }),
    );
    manualRender.rerender(
      wrap(<ControlledPicker hostId={15} seatunnelVersion='3.1.0' />),
    );
    expect(latestChanges.at(-1)).toEqual({
      javaHome: '/jdk/8',
      isCustom: false,
    });
  });
});
