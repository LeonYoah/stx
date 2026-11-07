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

import {describe, expect, it, vi, beforeEach} from 'vitest';
import {fireEvent, render, screen} from '@testing-library/react';
import {PostLoginSplash} from '../PostLoginSplash';

const start = vi.fn();
const destroy = vi.fn();
const skip = vi.fn();
const setWelcomeText = vi.fn();

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => {
    const map: Record<string, string> = {
      welcomeBack: '欢迎回来',
      skip: '跳过',
      ariaLabel: 'STX 开屏动画',
      statusAnnouncement: '正在播放',
    };
    return map[key] ?? key;
  },
}));

vi.mock('@/lib/animations/gsap-motion', () => ({
  shouldReduceMotion: () => false,
}));

vi.mock('@/lib/splash/meteor-intro-engine', () => {
  class MeteorIntroEngineMock {
    constructor(opts: {onComplete?: () => void}) {
      // 暴露给测试：点击跳过时走 skip → onComplete
      // Expose for tests: skip path calls onComplete
      (this as {__onComplete?: () => void}).__onComplete = opts.onComplete;
    }
    start = start;
    destroy = destroy;
    setWelcomeText = setWelcomeText;
    skip = () => {
      skip();
      (this as {__onComplete?: () => void}).__onComplete?.();
    };
  }
  return {MeteorIntroEngine: MeteorIntroEngineMock};
});

describe('PostLoginSplash', () => {
  beforeEach(() => {
    start.mockReset();
    destroy.mockReset();
    skip.mockReset();
    setWelcomeText.mockReset();
  });

  it('starts engine, exposes skip, and cleans up on unmount', () => {
    const onDismiss = vi.fn();
    const {unmount} = render(<PostLoginSplash onDismiss={onDismiss} />);

    expect(screen.getByRole('dialog', {name: 'STX 开屏动画'})).toBeInTheDocument();
    expect(start).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole('button', {name: '跳过'}));
    expect(skip).toHaveBeenCalledTimes(1);
    expect(onDismiss).toHaveBeenCalledTimes(1);

    unmount();
    expect(destroy).toHaveBeenCalled();
  });

  it('Escape triggers skip', () => {
    const onDismiss = vi.fn();
    render(<PostLoginSplash onDismiss={onDismiss} />);
    fireEvent.keyDown(window, {key: 'Escape'});
    expect(skip).toHaveBeenCalled();
    expect(onDismiss).toHaveBeenCalled();
  });
});
