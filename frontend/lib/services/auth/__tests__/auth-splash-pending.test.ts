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
import {
  peekPostLoginSplashPending,
  POST_LOGIN_SPLASH_PENDING_KEY,
} from '@/lib/splash/post-login-splash-session';

vi.mock('../../core/api-client', () => ({
  default: {
    get: vi.fn(),
    post: vi.fn(async () => ({data: {data: {username: 'admin'}}})),
    put: vi.fn(),
  },
}));

describe('AuthService splash pending arming', () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.resetModules();
    // @ts-expect-error jsdom location replace for redirect assertions
    delete window.location;
    // @ts-expect-error jsdom location stub
    window.location = {href: ''};
  });

  it('marks splash pending before redirecting on credential login', async () => {
    const {AuthService} = await import('../auth.service');
    await AuthService.login({username: 'admin', password: 'x'}, '/clusters/1');
    expect(peekPostLoginSplashPending()).toBe(true);
    expect(sessionStorage.getItem(POST_LOGIN_SPLASH_PENDING_KEY)).toBe('1');
    expect(window.location.href).toBe('/clusters/1');
  });

  it('clears splash pending on logout', async () => {
    sessionStorage.setItem(POST_LOGIN_SPLASH_PENDING_KEY, '1');
    const {AuthService} = await import('../auth.service');
    await AuthService.logout('/login');
    expect(peekPostLoginSplashPending()).toBe(false);
    expect(window.location.href).toContain('/login');
  });
});
