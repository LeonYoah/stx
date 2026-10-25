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

import {describe, expect, it, vi} from 'vitest';
import {renderHook} from '@testing-library/react';
import {useRefreshOnSettle} from '../useRefreshOnSettle';

describe('useRefreshOnSettle', () => {
  it('refreshes exactly once when the task leaves the active state', () => {
    const refresh = vi.fn();
    const {rerender} = renderHook(
      ({active}) => useRefreshOnSettle(active, refresh),
      {
        initialProps: {active: true},
      },
    );
    expect(refresh).not.toHaveBeenCalled();

    rerender({active: true});
    expect(refresh).not.toHaveBeenCalled();

    rerender({active: false});
    expect(refresh).toHaveBeenCalledTimes(1);

    rerender({active: false});
    expect(refresh).toHaveBeenCalledTimes(1);
  });

  it('does not refresh for a task that was never active on this page', () => {
    const refresh = vi.fn();
    const {rerender} = renderHook(
      ({active}) => useRefreshOnSettle(active, refresh),
      {
        initialProps: {active: false},
      },
    );
    rerender({active: false});
    expect(refresh).not.toHaveBeenCalled();
  });

  it('uses the latest refresh callback', () => {
    const first = vi.fn();
    const second = vi.fn();
    const {rerender} = renderHook(
      ({active, refresh}) => useRefreshOnSettle(active, refresh),
      {initialProps: {active: true, refresh: first}},
    );
    rerender({active: false, refresh: second});
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
  });
});
