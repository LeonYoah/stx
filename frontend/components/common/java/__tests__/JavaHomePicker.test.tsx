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

import {describe, it, expect, vi, beforeEach} from 'vitest';
import {render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {JavaHomePicker} from '../JavaHomePicker';

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
      {java_home: '/usr/lib/jvm/java-11', version: 11, version_string: '11.0.2'},
      {java_home: '/opt/java/jdk8', version: 8, version_string: '1.8.0'},
    ]);
  });

  it('probes host JDK list on mount (AC1b / AC1b2)', async () => {
    const onChange = vi.fn();
    render(
      wrap(
        <JavaHomePicker hostId={1} value='' onChange={onChange} id='jdk' />,
      ),
    );
    await waitFor(() => expect(listJavaRuntimes).toHaveBeenCalledWith(1));
    expect(screen.getByText('JDK path')).toBeInTheDocument();
    expect(screen.getByText('Auto')).toBeInTheDocument();
    expect(screen.queryByText('hint')).not.toBeInTheDocument();
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
    expect(await screen.findByText('Could not load the JDK list')).toBeInTheDocument();
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
});
