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
import {fireEvent, render, screen} from '@testing-library/react';
import type {ReactNode} from 'react';

vi.mock('../onboarding-checklist.module.css', () => {
  const handler: ProxyHandler<Record<string, string>> = {
    get: (_target, prop) => String(prop),
  };
  return {default: new Proxy({}, handler)};
});

vi.mock('motion/react', () => {
  const passthrough = ({
    children,
    ...rest
  }: {
    children?: ReactNode;
    [key: string]: unknown;
  }) => {
    const safe: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(rest)) {
      if (
        key === 'className' ||
        key === 'id' ||
        key === 'role' ||
        key === 'style' ||
        key.startsWith('data-') ||
        key.startsWith('aria-')
      ) {
        safe[key] = value;
      }
    }
    return <div {...safe}>{children}</div>;
  };
  return {
    AnimatePresence: ({children}: {children?: ReactNode}) => <>{children}</>,
    motion: {
      span: passthrough,
      div: passthrough,
      li: ({children, ...rest}: {children?: ReactNode; [key: string]: unknown}) => {
        const safe: Record<string, unknown> = {};
        for (const [key, value] of Object.entries(rest)) {
          if (
            key === 'className' ||
            key === 'id' ||
            key.startsWith('data-') ||
            key.startsWith('aria-')
          ) {
            safe[key] = value;
          }
        }
        return <li {...safe}>{children}</li>;
      },
      svg: passthrough,
      path: passthrough,
      circle: passthrough,
    },
    useIsPresent: () => true,
    useReducedMotion: () => true,
  };
});

import {OnboardingChecklist} from '../onboarding-checklist';

describe('OnboardingChecklist fully-hide reachability', () => {
  it('renders focusable fully-hide with icon hit target and distinct headerActions', () => {
    const onFullyHide = vi.fn();
    const onCollapse = vi.fn();
    const {container} = render(
      <div style={{width: 360}} data-testid='narrow-shell'>
        <OnboardingChecklist
          title='Install'
          steps={[
            {
              id: 'hostsAgents',
              title: 'Hosts',
              description: 'Connect hosts',
              actionLabel: 'Open',
              externalCompletion: true,
              onAction: async () => undefined,
            },
          ]}
          hideLabel='Collapse'
          showLabel='Expand'
          fullyHideLabel='Hide'
          onCollapseRequest={onCollapse}
          onFullyHideRequest={onFullyHide}
        />
      </div>,
    );

    const hideBtn = screen.getByTestId('stx-onboarding-fully-hide');
    const collapseBtn = screen.getByTestId('stx-onboarding-collapse');
    expect(hideBtn).toBeTruthy();
    expect(collapseBtn).toBeTruthy();
    expect(hideBtn.getAttribute('aria-label')).toBe('Hide');
    // Icon keeps a pointer/keyboard target when narrow CSS hides text label.
    // 窄布局 CSS 隐藏文案时，图标仍保留指针/键盘目标。
    expect(hideBtn.querySelector('svg')).toBeTruthy();

    fireEvent.click(hideBtn);
    expect(onFullyHide).toHaveBeenCalledTimes(1);
    fireEvent.click(collapseBtn);
    expect(onCollapse).toHaveBeenCalledTimes(1);

    const headerActions = container.querySelector('.headerActions');
    expect(headerActions).toBeTruthy();
    // Wrapper must not carry .hide (that class clamps width under @container).
    // 包装节点不得带 .hide（该 class 在 @container 下会钳宽）。
    expect(headerActions?.classList.contains('hide')).toBe(false);
    expect(hideBtn.classList.contains('hide')).toBe(true);
  });
});
