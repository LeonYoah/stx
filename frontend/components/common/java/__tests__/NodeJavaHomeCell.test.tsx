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

import {describe, it, expect} from 'vitest';
import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {NodeJavaHomeCell} from '../NodeJavaHomeCell';

const messages = {
  javaHome: {
    badgeAuto: 'Auto detect',
    versionLoading: 'Fetching version…',
    versionUnavailable: 'Version unavailable',
    effectiveVersionValue: 'Java {version}',
    titlePathOnly: '{path}',
    titleWithVersion: '{path} · Java {version}',
  },
};

function wrap(ui: React.ReactElement) {
  return (
    <NextIntlClientProvider locale='en' messages={messages}>
      {ui}
    </NextIntlClientProvider>
  );
}

describe('NodeJavaHomeCell', () => {
  it('shows configured path and version when fetch succeeds (AC7)', () => {
    render(
      wrap(
        <NodeJavaHomeCell
          configuredJavaHome='/opt/jdk11'
          runtime={{status: 'ok', version: 11, version_string: '11.0.2'}}
        />,
      ),
    );
    expect(screen.getByText('/opt/jdk11')).toBeInTheDocument();
    expect(screen.getByText('Java 11.0.2')).toBeInTheDocument();
    expect(screen.getByTitle('/opt/jdk11 · Java 11.0.2')).toBeInTheDocument();
  });

  it('shows auto label and version-unavailable when fetch fails (AC7)', () => {
    render(
      wrap(
        <NodeJavaHomeCell configuredJavaHome='' runtime={{status: 'error'}} />,
      ),
    );
    expect(screen.getByText('Auto detect')).toBeInTheDocument();
    expect(screen.getByText('Version unavailable')).toBeInTheDocument();
  });

  it('keeps path visible while version is loading (AC7)', () => {
    render(
      wrap(
        <NodeJavaHomeCell
          configuredJavaHome='/usr/lib/jvm/java-8'
          runtime={{status: 'loading'}}
        />,
      ),
    );
    expect(screen.getByText('/usr/lib/jvm/java-8')).toBeInTheDocument();
    expect(screen.getByText('Fetching version…')).toBeInTheDocument();
  });
});
