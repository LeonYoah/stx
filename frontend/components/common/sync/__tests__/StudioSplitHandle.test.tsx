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

import React from 'react';
import {describe, expect, it, vi} from 'vitest';
import {fireEvent, render, screen} from '@testing-library/react';
import {StudioSplitHandle} from '../StudioSplitHandle';

describe('StudioSplitHandle', () => {
  it('moves the value with arrow keys and clamps at the ends', () => {
    const onChange = vi.fn();
    render(
      <StudioSplitHandle
        orientation='vertical'
        label='Resize explorer'
        value={240}
        min={180}
        max={420}
        onChange={onChange}
      />,
    );

    const handle = screen.getByRole('separator', {name: 'Resize explorer'});
    fireEvent.keyDown(handle, {key: 'ArrowRight'});
    expect(onChange).toHaveBeenCalledWith(256);
    fireEvent.keyDown(handle, {key: 'Home'});
    expect(onChange).toHaveBeenCalledWith(180);
    fireEvent.keyDown(handle, {key: 'End'});
    expect(onChange).toHaveBeenCalledWith(420);
  });

  it('inverts keyboard steps for the right sidebar handle', () => {
    const onChange = vi.fn();
    render(
      <StudioSplitHandle
        orientation='vertical'
        invert
        label='Resize sidebar'
        value={360}
        min={280}
        max={560}
        onChange={onChange}
      />,
    );

    fireEvent.keyDown(screen.getByRole('separator', {name: 'Resize sidebar'}), {
      key: 'ArrowRight',
    });
    expect(onChange).toHaveBeenCalledWith(344);
  });
});
