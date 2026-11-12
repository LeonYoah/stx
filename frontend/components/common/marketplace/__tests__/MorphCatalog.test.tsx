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
 * MorphCatalog interaction + empty-menu + loading-dim tests.
 * MorphCatalog 交互、空菜单与加载变淡测试。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {
  hasMarketplaceMenuActions,
  MarketplaceMorphCatalog,
} from '../MorphCatalog';
import type {MarketplaceCatalogItem} from '../types';
import {DropdownMenuItem} from '@/components/ui/dropdown-menu';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) => {
    if (values?.name) {
      return `${key}:${values.name}`;
    }
    if (values?.label) {
      return `${key}:${values.label}:${values.display}`;
    }
    return key;
  },
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
      onClick,
      onKeyDown,
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
        onClick={onClick}
        onKeyDown={onKeyDown}
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

afterEach(() => {
  cleanup();
});

beforeEach(async () => {
  const motionMock = await import('motion/react');
  (
    motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
  ).__setReducedMotionForTest(false);
});

describe('hasMarketplaceMenuActions', () => {
  it('treats empty fragments and null as no menu', () => {
    expect(hasMarketplaceMenuActions(undefined)).toBe(false);
    expect(hasMarketplaceMenuActions(null)).toBe(false);
    expect(hasMarketplaceMenuActions(<></>)).toBe(false);
    expect(hasMarketplaceMenuActions(<>{null}</>)).toBe(false);
    expect(hasMarketplaceMenuActions(<>{false && <span />}</>)).toBe(false);
    expect(
      hasMarketplaceMenuActions(
        <>
          <DropdownMenuItem>Go</DropdownMenuItem>
        </>,
      ),
    ).toBe(true);
  });
});

describe('MarketplaceMorphCatalog', () => {
  it('dims the list container while loading with existing items', () => {
    const items: MarketplaceCatalogItem[] = [
      {id: 'a', title: 'Alpha', subtitle: 'first'},
    ];
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

  it('renders table headers and progress aria-label', () => {
    const items: MarketplaceCatalogItem[] = [
      {
        id: 'a',
        title: 'Alpha',
        subtitle: 'first',
        progress: {label: 'Download', value: 40, display: '40%'},
      },
    ];
    render(<MarketplaceMorphCatalog items={items} viewMode='table' />);
    expect(screen.getByTestId('marketplace-morph-catalog-header')).toHaveTextContent(
      'colName',
    );
    expect(screen.getByRole('progressbar')).toHaveAttribute(
      'aria-label',
      'progressAria:Download:40%',
    );
  });

  it('does not render … menu when menuActions is empty or undefined', () => {
    const items: MarketplaceCatalogItem[] = [
      {
        id: 'empty',
        title: 'Empty',
        subtitle: 'x',
        menuActions: <></>,
      },
      {
        id: 'none',
        title: 'None',
        subtitle: 'y',
        menuActions: undefined,
      },
      {
        id: 'has',
        title: 'Has',
        subtitle: 'z',
        menuActions: <DropdownMenuItem>Act</DropdownMenuItem>,
      },
    ];
    render(<MarketplaceMorphCatalog items={items} viewMode='table' />);
    expect(screen.queryByTestId('marketplace-more-empty')).not.toBeInTheDocument();
    expect(screen.queryByTestId('marketplace-more-none')).not.toBeInTheDocument();
    expect(screen.getByTestId('marketplace-more-has')).toBeInTheDocument();
  });

  it('does not open detail when Enter is pressed on the nested more menu', () => {
    const onOpen = vi.fn();
    const items: MarketplaceCatalogItem[] = [
      {
        id: 'plugin',
        title: 'Kafka',
        subtitle: 'connector',
        onOpen,
        menuActions: <DropdownMenuItem>Docs</DropdownMenuItem>,
      },
    ];
    render(<MarketplaceMorphCatalog items={items} viewMode='table' />);

    const more = screen.getByTestId('marketplace-more-plugin');
    // Nested control Enter must not open detail (stopPropagation + target guard).
    // 内部控件 Enter 不得打开详情（stopPropagation + target 守卫）。
    fireEvent.keyDown(more, {key: 'Enter'});
    more.dispatchEvent(
      new KeyboardEvent('keydown', {key: 'Enter', bubbles: true}),
    );
    expect(onOpen).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('marketplace-open-plugin'));
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it('ignores shell click when the event target is a nested control', () => {
    const onOpen = vi.fn();
    const items: MarketplaceCatalogItem[] = [
      {
        id: 'plugin',
        title: 'Kafka',
        subtitle: 'connector',
        onOpen,
        primaryActions: (
          <button type='button' data-testid='nested-primary'>
            Go
          </button>
        ),
      },
    ];
    render(<MarketplaceMorphCatalog items={items} viewMode='card' />);
    fireEvent.click(screen.getByTestId('nested-primary'));
    expect(onOpen).not.toHaveBeenCalled();
  });
});
