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

import {beforeEach, describe, expect, it} from 'vitest';
import {
  clearPostLoginSplashSession,
  consumePostLoginSplashPending,
  markPostLoginSplashPending,
  peekPostLoginSplashPending,
  POST_LOGIN_SPLASH_PENDING_KEY,
} from '../post-login-splash-session';

describe('post-login-splash-session', () => {
  beforeEach(() => {
    sessionStorage.clear();
  });

  it('marks and peeks pending without consuming', () => {
    expect(peekPostLoginSplashPending()).toBe(false);
    markPostLoginSplashPending();
    expect(sessionStorage.getItem(POST_LOGIN_SPLASH_PENDING_KEY)).toBe('1');
    expect(peekPostLoginSplashPending()).toBe(true);
    expect(peekPostLoginSplashPending()).toBe(true);
  });

  it('consumes pending only once so refresh/nav will not replay', () => {
    markPostLoginSplashPending();
    expect(consumePostLoginSplashPending()).toBe(true);
    expect(consumePostLoginSplashPending()).toBe(false);
    expect(peekPostLoginSplashPending()).toBe(false);
  });

  it('clears pending on logout-style cleanup', () => {
    markPostLoginSplashPending();
    clearPostLoginSplashSession();
    expect(peekPostLoginSplashPending()).toBe(false);
    expect(consumePostLoginSplashPending()).toBe(false);
  });
});
