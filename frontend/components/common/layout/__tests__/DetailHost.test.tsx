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

import {act, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {canFlyDetailName, DetailHost} from '../DetailHost';

function renderHost(
  props: Partial<Parameters<typeof DetailHost>[0]> & {open: boolean},
) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <DetailHost
        hasContent
        onOpenChange={() => {}}
        narrow={false}
        title='详情'
        {...props}
      >
        <div>body</div>
      </DetailHost>
    </NextIntlClientProvider>,
  );
}

function mockMatchMedia(matches: boolean) {
  // JSDOM 没有 matchMedia；matches=false 表示宽屏（走常驻栏）。
  // JSDOM lacks matchMedia; matches=false means wide layout (persistent rail).
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation(() => ({
      matches,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  });
}

describe('DetailHost', () => {
  beforeEach(() => {
    mockMatchMedia(false);
  });

  it('renders the persistent rail with an open data-state once mounted', async () => {
    renderHost({open: true});
    const rail = await screen.findByTestId('detail-rail');
    await waitFor(() => expect(rail).toHaveAttribute('data-state', 'open'));
    expect(rail).toHaveAttribute('data-motion', 'enter');
  });

  it('keeps the rail mounted while the close animation plays, then unmounts', async () => {
    const {rerender} = renderHost({open: true});
    await screen.findByTestId('detail-rail');

    rerender(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <DetailHost
          open={false}
          hasContent
          onOpenChange={() => {}}
          narrow={false}
          title='详情'
        >
          <div>body</div>
        </DetailHost>
      </NextIntlClientProvider>,
    );

    // 关闭动画期间栏仍在 DOM（data-state 翻回 closed）。
    // The rail stays in the DOM during the close animation (state flips back).
    const rail = screen.getByTestId('detail-rail');
    await waitFor(() => expect(rail).toHaveAttribute('data-state', 'closed'));

    // jsdom 不触发 transitionend，走 600ms 卸载兜底。
    // jsdom never fires transitionend, so the 600ms fallback unmounts it.
    await waitFor(
      () =>
        expect(screen.queryByTestId('detail-rail')).not.toBeInTheDocument(),
      {timeout: 2000},
    );
  });

  it('cancels the pending unmount when reopened mid-close', async () => {
    const {rerender} = renderHost({open: true});
    await screen.findByTestId('detail-rail');

    const host = (open: boolean) => (
      <NextIntlClientProvider locale='zh' messages={zh}>
        <DetailHost
          open={open}
          hasContent
          onOpenChange={() => {}}
          narrow={false}
          title='详情'
        >
          <div>body</div>
        </DetailHost>
      </NextIntlClientProvider>
    );

    rerender(host(false));
    expect(screen.getByTestId('detail-rail')).toBeInTheDocument();

    // 收起动画未播完就重新打开，栏不能被卸载。
    // Reopening before the collapse finishes must not unmount the rail.
    rerender(host(true));
    await act(async () => {
      await new Promise((r) => setTimeout(r, 700));
    });
    const rail = screen.getByTestId('detail-rail');
    await waitFor(() => expect(rail).toHaveAttribute('data-state', 'open'));
  });

  it('does not render a persistent rail in narrow mode', () => {
    renderHost({open: true, narrow: true});
    expect(screen.queryByTestId('detail-rail')).not.toBeInTheDocument();
  });

  it('marks the configured header slot as the fly-in target', async () => {
    const {unmount} = renderHost({
      open: true,
      meta: 'meta text',
      nameSlot: 'meta',
    });
    const rail = await screen.findByTestId('detail-rail');
    expect(
      rail.querySelector("[data-detail-name-target='meta']"),
    ).not.toBeNull();
    expect(
      rail.querySelector("[data-detail-name-target='title']"),
    ).toBeNull();
    unmount();
  });
});

describe('canFlyDetailName', () => {
  const row = () => {
    const el = document.createElement('div');
    document.body.appendChild(el);
    return el;
  };
  const base = {
    narrow: false,
    reducedMotion: false,
    nameSlot: 'title' as const,
    sourceText: '告警 A',
    targetText: '告警 A',
  };

  it('flies only when an origin row exists, is connected, and texts match', () => {
    expect(canFlyDetailName({...base, origin: row()})).toBe(true);
    expect(canFlyDetailName({...base, origin: null})).toBe(false);
    expect(canFlyDetailName({...base, origin: undefined})).toBe(false);
    // 脱离文档的来源行不算数（翻页后旧行已卸载）。
    // A detached origin row does not count (paged-away rows are unmounted).
    const detached = document.createElement('div');
    expect(canFlyDetailName({...base, origin: detached})).toBe(false);
  });

  it('skips in narrow mode and under reduced motion', () => {
    expect(canFlyDetailName({...base, origin: row(), narrow: true})).toBe(
      false,
    );
    expect(
      canFlyDetailName({...base, origin: row(), reducedMotion: true}),
    ).toBe(false);
  });

  it('skips without a nameSlot or when source/target text differ', () => {
    expect(
      canFlyDetailName({...base, origin: row(), nameSlot: undefined}),
    ).toBe(false);
    expect(
      canFlyDetailName({...base, origin: row(), targetText: '告警 B'}),
    ).toBe(false);
    // trim 后一致仍算一致（尾部空白、换行不阻断）。
    // Trailing whitespace/newlines still count as a match.
    expect(
      canFlyDetailName({...base, origin: row(), sourceText: ' 告警 A\n'}),
    ).toBe(true);
    // 空文本不飞。Empty text never flies.
    expect(canFlyDetailName({...base, origin: row(), sourceText: '  '})).toBe(
      false,
    );
  });
});
