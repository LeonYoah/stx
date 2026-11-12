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
 * MorphCatalog interaction + empty-menu + flip + loading-dim tests.
 * MorphCatalog 交互、空菜单、翻面与加载变淡测试。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
import {
  hasMarketplaceMenuActions,
  MarketplaceMorphCatalog,
  shouldSkipMarketplaceShellOpen,
} from '../MorphCatalog';
import {
  CATALOG_TABLE_COLUMNS,
  CATALOG_TABLE_COLUMNS_SELECTABLE,
  CATALOG_TABLE_GRID,
} from '../catalog-table-grid';
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
    delete next.exit;
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
      style,
      'data-testid': dataTestId,
      'data-reduce-motion': dataReduceMotion,
      'data-view-mode': dataViewMode,
      'data-flipped': dataFlipped,
      ...rest
    } = props as React.HTMLAttributes<HTMLDivElement> & {
      'data-testid'?: string;
      'data-reduce-motion'?: string;
      'data-view-mode'?: string;
      'data-flipped'?: string;
    };
    return (
      <div
        ref={ref}
        className={className}
        style={style}
        data-testid={dataTestId}
        data-reduce-motion={dataReduceMotion}
        data-view-mode={dataViewMode}
        data-flipped={dataFlipped}
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
    AnimatePresence: ({children}: {children: React.ReactNode}) => <>{children}</>,
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
    expect(
      hasMarketplaceMenuActions(
        <>
          <DropdownMenuItem>Go</DropdownMenuItem>
        </>,
      ),
    ).toBe(true);
  });
});

describe('shouldSkipMarketplaceShellOpen', () => {
  it('skips portal targets outside the shell and nested interactive controls', () => {
    const shell = document.createElement('div');
    const icon = document.createElement('div');
    const btn = document.createElement('button');
    shell.append(icon, btn);
    const portal = document.createElement('div');
    document.body.append(shell, portal);

    expect(shouldSkipMarketplaceShellOpen(icon, shell)).toBe(false);
    expect(shouldSkipMarketplaceShellOpen(btn, shell)).toBe(true);
    expect(shouldSkipMarketplaceShellOpen(portal, shell)).toBe(true);
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
  });

  it('does not open detail from blank card areas; only title or view-detail', () => {
    const onOpen = vi.fn();
    const items: MarketplaceCatalogItem[] = [
      {
        id: 'plugin',
        title: 'Kafka',
        subtitle: 'v2.3.12',
        onOpen,
        icon: <span>K</span>,
        badges: [{label: 'Source'}],
        fields: [{label: 'Deps', value: 'none'}],
      },
    ];
    render(<MarketplaceMorphCatalog items={items} viewMode='card' />);

    const card = screen.getByTestId('marketplace-card-plugin');
    expect(card.className).not.toMatch(/cursor-pointer/);

    fireEvent.click(screen.getByTestId('marketplace-icon-plugin'));
    fireEvent.click(screen.getByTestId('marketplace-subtitle-plugin'));
    fireEvent.click(screen.getByTestId('marketplace-badge-plugin'));
    fireEvent.click(card);
    expect(onOpen).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('marketplace-open-plugin'));
    expect(onOpen).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByTestId('marketplace-detail-plugin'));
    expect(onOpen).toHaveBeenCalledTimes(2);
  });

  it('flips card via explicit button and keeps primary action on the front', () => {
    const onPrimary = vi.fn();
    const items: MarketplaceCatalogItem[] = [
      {
        id: 'plugin',
        title: 'Kafka',
        subtitle: 'v2.3.12',
        description: 'Full connector blurb',
        fields: [{label: 'Deps', value: 'jdbc'}],
        primaryActions: (
          <button type='button' data-testid='nested-primary' onClick={onPrimary}>
            Download
          </button>
        ),
      },
    ];
    render(<MarketplaceMorphCatalog items={items} viewMode='card' />);

    expect(screen.getByTestId('marketplace-card-front-plugin')).toBeInTheDocument();
    expect(screen.getByTestId('nested-primary')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('nested-primary'));
    expect(onPrimary).toHaveBeenCalledTimes(1);

    const flip = screen.getByTestId('marketplace-flip-plugin-front');
    fireEvent.click(flip);
    expect(screen.getByTestId('marketplace-card-plugin')).toHaveAttribute(
      'data-flipped',
      'true',
    );
    expect(screen.getByTestId('marketplace-description-plugin')).toHaveTextContent(
      'Full connector blurb',
    );
    expect(screen.getByText('jdbc')).toBeInTheDocument();
  });

  it('does not open detail from … menu, menu Enter, or primary action buttons', () => {
    const onOpen = vi.fn();
    const items: MarketplaceCatalogItem[] = [
      {
        id: 'plugin',
        title: 'Kafka',
        subtitle: 'connector',
        onOpen,
        menuActions: <DropdownMenuItem>Docs</DropdownMenuItem>,
        primaryActions: (
          <button type='button' data-testid='nested-primary'>
            Go
          </button>
        ),
      },
    ];
    render(<MarketplaceMorphCatalog items={items} viewMode='table' />);

    fireEvent.click(screen.getByTestId('nested-primary'));
    fireEvent.click(screen.getByTestId('marketplace-more-plugin'));
    fireEvent.keyDown(screen.getByTestId('marketplace-more-plugin'), {
      key: 'Enter',
    });
    screen.getByTestId('marketplace-more-plugin').dispatchEvent(
      new KeyboardEvent('keydown', {key: 'Enter', bubbles: true}),
    );
    expect(onOpen).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('marketplace-open-plugin'));
    expect(onOpen).toHaveBeenCalledTimes(1);
  });

  it('does not render … menu when menuActions is empty or undefined', () => {
    const items: MarketplaceCatalogItem[] = [
      {id: 'empty', title: 'Empty', subtitle: 'x', menuActions: <></>},
      {id: 'none', title: 'None', subtitle: 'y', menuActions: undefined},
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

  it('wraps title text for truncate and keeps min-w-0 on the open button', () => {
    render(
      <MarketplaceMorphCatalog
        items={[
          {
            id: 'long',
            title: 'VeryLongPluginNameThatShouldTruncate',
            subtitle: 'x',
            onOpen: () => undefined,
          },
        ]}
        viewMode='table'
      />,
    );
    const open = screen.getByTestId('marketplace-open-long');
    expect(open.className).toMatch(/min-w-0/);
    expect(open.className).toMatch(/max-w-full/);
    expect(open.querySelector('span.truncate')).not.toBeNull();
  });

  it('shares subgrid strips under one parent column track (no auto column)', () => {
    render(
      <MarketplaceMorphCatalog
        items={[{id: 'a', title: 'A', subtitle: 'v1'}]}
        viewMode='table'
      />,
    );
    const table = screen.getByTestId('marketplace-morph-catalog-table');
    const header = screen.getByTestId('marketplace-morph-catalog-header');
    const row = screen.getByTestId('marketplace-row-a');
    expect(table.className).toContain(CATALOG_TABLE_COLUMNS);
    expect(header.className).toContain(CATALOG_TABLE_GRID);
    expect(row.className).toContain(CATALOG_TABLE_GRID);
    expect(header.className.includes('_auto]')).toBe(false);
    expect(row.className.includes('_auto]')).toBe(false);
  });

  it('uses selectable parent columns when selection is provided', () => {
    const selectedIds = new Set<string>();
    render(
      <MarketplaceMorphCatalog
        items={[{id: 'a', title: 'A', subtitle: 'v1'}]}
        viewMode='table'
        selection={{
          selectedIds,
          onToggle: () => undefined,
          onToggleAll: () => undefined,
        }}
      />,
    );
    const table = screen.getByTestId('marketplace-morph-catalog-table');
    expect(table.className).toContain(CATALOG_TABLE_COLUMNS_SELECTABLE);
    expect(
      screen.getByRole('checkbox', {name: 'selectAll'}),
    ).toBeInTheDocument();
  });

  it('hides flip control under reduced motion', async () => {
    const motionMock = await import('motion/react');
    (
      motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
    ).__setReducedMotionForTest(true);

    render(
      <MarketplaceMorphCatalog
        items={[
          {
            id: 'plugin',
            title: 'Kafka',
            subtitle: 'v1',
            description: 'blurb',
            fields: [{label: 'Deps', value: 'x'}],
          },
        ]}
        viewMode='card'
      />,
    );
    expect(
      screen.queryByTestId('marketplace-flip-plugin-front'),
    ).not.toBeInTheDocument();
    expect(screen.getByTestId('marketplace-card-front-plugin')).toBeInTheDocument();
  });

  it('keeps cards visible after switching table → card', () => {
    const items: MarketplaceCatalogItem[] = [
      {id: 'a', title: 'Alpha', subtitle: 'v1', description: 'd', fields: [{label: 'x', value: 'y'}]},
    ];
    const {rerender} = render(
      <MarketplaceMorphCatalog items={items} viewMode='table' />,
    );
    expect(screen.getByTestId('marketplace-row-a')).toBeInTheDocument();
    rerender(<MarketplaceMorphCatalog items={items} viewMode='card' />);
    const card = screen.getByTestId('marketplace-card-a');
    expect(card).toBeInTheDocument();
    expect(screen.getByTestId('marketplace-card-front-a')).toBeInTheDocument();
    // Motion mock strips animate; ensure the card shell is in the document after remount.
    // Motion mock 会剥掉 animate；这里确认重挂后卡片外壳仍在。
    expect(card.getAttribute('data-flipped')).toBe('false');
  });
});
