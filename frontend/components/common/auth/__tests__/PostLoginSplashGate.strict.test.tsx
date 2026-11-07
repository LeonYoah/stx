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

import React, {StrictMode} from 'react';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import {act, render, screen} from '@testing-library/react';
import {PostLoginSplashGate} from '../PostLoginSplashGate';
import {
  markPostLoginSplashPending,
  peekPostLoginSplashPending,
} from '@/lib/splash/post-login-splash-session';

vi.mock('next/dynamic', () => ({
  default: () => {
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

describe('PostLoginSplashGate StrictMode / remount semantics (W2 verification)', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('keeps splash active under React StrictMode double effect (consume once, state preserved)', async () => {
    markPostLoginSplashPending();
    render(
      <StrictMode>
        <PostLoginSplashGate />
      </StrictMode>,
    );

    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    // pending 已被第一次 effect 消费；第二次 Strict effect 再 consume 为 false，但不应清掉已激活的 state
    // Pending was consumed by the first effect; the second Strict effect sees false but must not clear active state
    expect(peekPostLoginSplashPending()).toBe(false);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('full unmount+new instance after consume does not replay (no module latch / no refresh replay)', async () => {
    markPostLoginSplashPending();
    const first = render(<PostLoginSplashGate />);
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    first.unmount();

    // 全新实例：session 已消费，不应因「模块变量」误播
    // Fresh instance: session already consumed; must not replay via a module latch
    const second = render(<PostLoginSplashGate />);
    await act(async () => {});
    expect(screen.queryByRole('dialog')).toBeNull();
    second.unmount();
  });
});
