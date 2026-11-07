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

import {beforeEach, describe, expect, it, vi} from 'vitest';
import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {PostLoginSplashGate} from '../PostLoginSplashGate';
import {
  markPostLoginSplashPending,
  peekPostLoginSplashPending,
} from '@/lib/splash/post-login-splash-session';

vi.mock('next/dynamic', () => ({
  default: () => {
    // 简化动态导入：直接渲染轻量替身，避免拉起真实 Canvas 引擎
    // Stub dynamic import with a lightweight stand-in (no real canvas engine)
    return function SplashStub({onDismiss}: {onDismiss: () => void}) {
      return (
        <div role='dialog' aria-label='STX splash animation'>
          <button type='button' onClick={onDismiss}>
            Skip
          </button>
        </div>
      );
    };
  },
}));

describe('PostLoginSplashGate', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('does not mount splash when pending is absent', async () => {
    render(<PostLoginSplashGate />);
    await act(async () => {});
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('consumes pending and shows splash once, then dismiss removes it', async () => {
    markPostLoginSplashPending();
    render(<PostLoginSplashGate />);

    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(peekPostLoginSplashPending()).toBe(false);

    fireEvent.click(screen.getByRole('button', {name: 'Skip'}));
    await waitFor(() => {
      expect(screen.queryByRole('dialog')).toBeNull();
    });
  });

  it('does not remount splash after pending was already consumed', async () => {
    markPostLoginSplashPending();
    const first = render(<PostLoginSplashGate />);
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    first.unmount();

    render(<PostLoginSplashGate />);
    await act(async () => {});
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
