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
import {
  TruncatedText,
  splitPathForMiddleEllipsis,
} from '../TruncatedText';

const messages = {
  common: {
    copy: 'Copy',
    copied: 'Copied',
  },
};

function wrap(ui: React.ReactElement) {
  return (
    <NextIntlClientProvider locale='en' messages={messages}>
      {ui}
    </NextIntlClientProvider>
  );
}

describe('splitPathForMiddleEllipsis', () => {
  it('splits deep JDK paths while preserving distinguishing tail segments', () => {
    expect(
      splitPathForMiddleEllipsis(
        '/Library/Java/JavaVirtualMachines/jdk-26.jdk/Contents/Home',
      ),
    ).toEqual([
      '/Library/Java/JavaVirtualMachines',
      '/jdk-26.jdk/Contents/Home',
    ]);
  });

  it('keeps short 2-segment paths intact without splitting', () => {
    expect(splitPathForMiddleEllipsis('/opt/jdk11')).toBeNull();
    expect(splitPathForMiddleEllipsis('Auto detect')).toBeNull();
  });

  it('splits standard install paths with prefix and tail', () => {
    expect(
      splitPathForMiddleEllipsis('/Users/mac/opt/seatunnel-3.0.0'),
    ).toEqual(['/Users/mac', '/opt/seatunnel-3.0.0']);
  });
});

describe('TruncatedText', () => {
  it('renders fallback when text is empty', () => {
    render(wrap(<TruncatedText text='' fallback='—' />));
    expect(screen.getByText('—')).toBeInTheDocument();
  });

  it('keeps full textContent intact in path mode', () => {
    const full = '/Library/Java/JavaVirtualMachines/jdk-26.jdk/Contents/Home';
    render(
      wrap(
        <TruncatedText
          text={full}
          mode='path'
          mono
          data-testid='jdk-path-trunc'
        />,
      ),
    );
    expect(screen.getByTestId('jdk-path-trunc')).toHaveTextContent(full);
  });
});
