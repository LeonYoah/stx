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
 * MorphCatalog: loading dim on container, empty/error states, reduced-motion morph off.
 * MorphCatalog：加载变淡在容器、空/错态、减少动效关闭形变。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen} from '@testing-library/react';
import {MarketplaceMorphCatalog} from '../MorphCatalog';
import type {MarketplaceCatalogItem} from '../types';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('motion/react', async () => {
  const ReactMod = await import('react');
  const reduceFlag = {current: false};

  function strip(props: Record<string, unknown>) {
    const next = {...props};
    delete next.variants;
    delete next.initial;
    delete next.animate;
    delete next.whileHover;
    delete next.transition;
    delete next.layout;
    delete next.layoutId;
    return next;
  }

  const MotionDiv = ReactMod.forwardRef(function MotionDiv(
    props: React.HTMLAttributes<HTMLDivElement> & Record<string, unknown>,
    ref: React.Ref<HTMLDivElement>,
  ) {
    const {
      children,
      className,
      'data-testid': dataTestId,
      'data-reduce-motion': dataReduceMotion,
      'data-view-mode': dataViewMode,
      ...rest
    } = props as React.HTMLAttributes<HTMLDivElement> & {
      'data-testid'?: string;
      'data-reduce-motion'?: string;
      'data-view-mode'?: string;
    };
    return (
      <div
        ref={ref}
        className={className}
        data-testid={dataTestId}
        data-reduce-motion={dataReduceMotion}
        data-view-mode={dataViewMode}
        {...strip(rest as Record<string, unknown>)}
      >
        {children}
      </div>
    );
  });

  return {
    __esModule: true,
    LayoutGroup: ({children}: {children: React.ReactNode}) => <>{children}</>,
    motion: {div: MotionDiv},
    useReducedMotion: () => reduceFlag.current,
    __setReducedMotionForTest: (value: boolean) => {
      reduceFlag.current = value;
    },
  };
});

const items: MarketplaceCatalogItem[] = [
  {
    id: 'a',
    title: 'Alpha',
    subtitle: 'first',
    status: {label: 'ready', tone: 'ok'},
  },
];

afterEach(() => {
  cleanup();
});

beforeEach(async () => {
  const motionMock = await import('motion/react');
  (
    motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
  ).__setReducedMotionForTest(false);
});

describe('MarketplaceMorphCatalog', () => {
  it('dims the list container while loading with existing items', () => {
    const {rerender} = render(
      <MarketplaceMorphCatalog items={items} viewMode='table' loading={false} />,
    );
    expect(screen.getByTestId('marketplace-morph-catalog').className).not.toMatch(
      /opacity-60/,
    );

    rerender(
      <MarketplaceMorphCatalog items={items} viewMode='table' loading />,
    );
    expect(screen.getByTestId('marketplace-morph-catalog').className).toMatch(
      /opacity-60/,
    );
    expect(screen.getByTestId('marketplace-row-a').className).not.toMatch(
      /opacity-60/,
    );
  });

  it('renders empty and error states for both views', () => {
    const {rerender} = render(
      <MarketplaceMorphCatalog items={[]} viewMode='table' empty='none-here' />,
    );
    expect(screen.getByTestId('marketplace-morph-catalog-empty')).toHaveTextContent(
      'none-here',
    );

    rerender(
      <MarketplaceMorphCatalog
        items={[]}
        viewMode='card'
        error='boom'
      />,
    );
    expect(screen.getByTestId('marketplace-morph-catalog-error')).toHaveTextContent(
      'boom',
    );
  });

  it('marks reduced-motion on the catalog shell', async () => {
    const motionMock = await import('motion/react');
    (
      motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
    ).__setReducedMotionForTest(true);

    render(<MarketplaceMorphCatalog items={items} viewMode='card' />);
    expect(screen.getByTestId('marketplace-morph-catalog')).toHaveAttribute(
      'data-reduce-motion',
      'true',
    );
    expect(screen.getByTestId('marketplace-card-a')).toBeInTheDocument();
  });
});
