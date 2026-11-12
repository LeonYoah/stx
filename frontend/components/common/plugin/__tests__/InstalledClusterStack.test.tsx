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

/**
 * InstalledClusterStack interaction and reduced-motion tests.
 * 已安装集群堆叠的交互与减少动态效果测试。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {InstalledClusterStack} from '../InstalledClusterStack';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('motion/react', async () => {
  const ReactMod = await import('react');
  const reduceFlag = {current: false};
  const Motion = ReactMod.forwardRef(function MotionStub(
    props: React.HTMLAttributes<HTMLElement> & {
      animate?: Record<string, unknown>;
      children?: React.ReactNode;
      type?: string;
      initial?: unknown;
      transition?: unknown;
      variants?: unknown;
      layout?: unknown;
    },
    ref: React.Ref<HTMLElement>,
  ) {
    const {
      children,
      animate,
      type,
      className,
      style,
      onMouseEnter,
      onFocus,
      onBlur,
      ...rest
    } = props;
    // Drop Motion-only props so they are not forwarded to the DOM.
    // 去掉 Motion 专用属性，避免转发到 DOM。
    const dom = {...rest} as Record<string, unknown>;
    delete dom.initial;
    delete dom.transition;
    delete dom.variants;
    delete dom.layout;

    const Tag = type === 'button' ? 'button' : 'span';
    return ReactMod.createElement(
      Tag,
      {
        ...dom,
        ref,
        type: Tag === 'button' ? 'button' : undefined,
        className,
        style,
        onMouseEnter,
        onFocus,
        onBlur,
        'data-animate-scale':
          animate && 'scale' in animate ? String(animate.scale) : undefined,
        'data-animate-ml':
          animate && 'marginLeft' in animate
            ? String(animate.marginLeft)
            : undefined,
      },
      children,
    );
  });

  return {
    __esModule: true,
    motion: {
      button: Motion,
      span: Motion,
      div: Motion,
    },
    useReducedMotion: () => reduceFlag.current,
    __setReducedMotionForTest: (value: boolean) => {
      reduceFlag.current = value;
    },
  };
});

vi.mock('@/components/ui/tooltip', () => ({
  TooltipProvider: ({children}: {children: React.ReactNode}) => <>{children}</>,
  Tooltip: ({children}: {children: React.ReactNode}) => <>{children}</>,
  TooltipTrigger: ({children}: {children: React.ReactNode}) => <>{children}</>,
  TooltipContent: ({children}: {children: React.ReactNode}) => (
    <div data-testid='tooltip-content'>{children}</div>
  ),
}));

vi.mock('@/components/ui/avatar', () => ({
  Avatar: ({children, className}: {children: React.ReactNode; className?: string}) => (
    <div className={className}>{children}</div>
  ),
  AvatarFallback: ({children}: {children: React.ReactNode}) => (
    <span>{children}</span>
  ),
}));

afterEach(() => {
  cleanup();
});

beforeEach(async () => {
  const motionMock = await import('motion/react');
  (
    motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
  ).__setReducedMotionForTest(false);
});

const manyClusters = [
  {id: 1, name: 'alpha'},
  {id: 2, name: 'beta'},
  {id: 3, name: 'gamma'},
  {id: 4, name: 'delta'},
  {id: 5, name: 'epsilon'},
  {id: 6, name: 'zeta'},
];

describe('InstalledClusterStack', () => {
  it('shows notInstalled when there are no clusters', () => {
    render(<InstalledClusterStack clusters={[]} />);
    expect(screen.getByTestId('plugin-cluster-stack-empty')).toHaveTextContent(
      'plugin.notInstalled',
    );
  });

  it('renders a compact stack with readable install count and overflow', () => {
    render(<InstalledClusterStack clusters={manyClusters} maxVisible={4} />);

    expect(screen.getByTestId('plugin-cluster-stack')).toBeInTheDocument();
    expect(screen.getByTestId('plugin-cluster-stack-item-1')).toBeInTheDocument();
    expect(screen.getByTestId('plugin-cluster-stack-overflow')).toHaveTextContent(
      '+2',
    );
    expect(screen.getByTestId('plugin-cluster-stack-count')).toHaveTextContent(
      '6',
    );
    // 全名在 sr-only 中可读；视觉仍是缩写芯片，不引入用户头像。
    // Full names stay readable via sr-only; visuals stay as initials chips, no user photos.
    expect(screen.getByTestId('plugin-cluster-stack-item-1')).toHaveTextContent(
      'alpha',
    );
    expect(screen.queryByRole('img')).not.toBeInTheDocument();
  });

  it('enlarges the hovered item and pushes neighbors when motion is allowed', () => {
    render(<InstalledClusterStack clusters={manyClusters.slice(0, 3)} />);

    const first = screen.getByTestId('plugin-cluster-stack-item-1');
    const second = screen.getByTestId('plugin-cluster-stack-item-2');

    fireEvent.mouseEnter(second);

    expect(second).toHaveAttribute('data-animate-scale', '1.18');
    expect(second).toHaveAttribute('data-animate-ml', '6');
    expect(first).toHaveAttribute('data-animate-ml', '0');
  });

  it('uses instant spacing switches without scale under reduced motion', async () => {
    const motionMock = await import('motion/react');
    (
      motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
    ).__setReducedMotionForTest(true);

    render(<InstalledClusterStack clusters={manyClusters.slice(0, 3)} />);

    const root = screen.getByTestId('plugin-cluster-stack');
    expect(root).toHaveAttribute('data-reduce-motion', 'true');

    const second = screen.getByTestId('plugin-cluster-stack-item-2');
    fireEvent.mouseEnter(second);

    expect(second).toHaveAttribute('data-animate-scale', '1');
    expect(second).toHaveAttribute('data-animate-ml', '6');
  });
});
