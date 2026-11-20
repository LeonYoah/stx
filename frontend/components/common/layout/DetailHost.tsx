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

'use client';

/**
 * 详情宿主：主区域 ≥1080px 时详情是页面内的常驻右侧栏（与列表表面同顶、
 * 同等边框圆角、不浮起），<1080px 退化为贴边 Arc Drawer（附着侧直角）。
 * 两种形态共用同一头部（标题 / 元信息 / ↑↓ 第 i/n 条 / 关闭）与 J/K 快捷键。
 * 宽度判定用 useSyncExternalStore + matchMedia，SSR 快照固定为窄屏，
 * 首屏两端一致（design.md §9.7 第 6 条）。
 * Detail host: at ≥1080px the detail is a persistent right rail (same top as
 * the list surface, same border/radius, not floating); below that it falls
 * back to a flush Arc Drawer (square on the attached side). Both forms share
 * the same header (title / meta / prev-next index / close) and J/K shortcuts.
 * The width check uses useSyncExternalStore + matchMedia with a fixed narrow
 * server snapshot so the first render matches on server and client.
 */
import {useEffect, useSyncExternalStore, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {ChevronDown, ChevronUp, X} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Drawer, DrawerContent} from '@/components/arc/drawer/drawer';
import {cn} from '@/lib/utils';
import styles from './list-detail.module.css';

const NARROW_QUERY = '(max-width: 1079px)';

function subscribeNarrow(onChange: () => void) {
  const mq = window.matchMedia(NARROW_QUERY);
  mq.addEventListener('change', onChange);
  return () => mq.removeEventListener('change', onChange);
}

/** 主区域是否窄于 1080px。Whether the main area is narrower than 1080px. */
export function useDetailNarrow(): boolean {
  return useSyncExternalStore(
    subscribeNarrow,
    () => window.matchMedia(NARROW_QUERY).matches,
    // SSR 与首帧按窄屏渲染（Drawer 形态），避免两端结构不一致。
    // Render narrow on the server and first frame so both sides agree.
    () => true,
  );
}

export interface DetailNav {
  /** 当前项在可导航列表中的下标（0 起）。Index of the open item. */
  index: number;
  /** 可导航项总数。Total navigable items. */
  total: number;
  onPrev: () => void;
  onNext: () => void;
}

function NavControls({nav}: {nav: DetailNav}) {
  const t = useTranslations('common.detailNav');
  return (
    <span className={styles.detailNav} data-testid='detail-nav'>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        iconOnly
        aria-label={t('prev')}
        disabled={nav.index <= 0}
        onClick={nav.onPrev}
      >
        <ChevronUp className='size-4' aria-hidden />
      </Button>
      <Button
        type='button'
        variant='ghost'
        size='sm'
        iconOnly
        aria-label={t('next')}
        disabled={nav.index >= nav.total - 1}
        onClick={nav.onNext}
      >
        <ChevronDown className='size-4' aria-hidden />
      </Button>
      <span className={styles.navIndex}>
        {nav.index + 1} / {nav.total}
      </span>
    </span>
  );
}

/**
 * children 约定：接收方渲染 detailCol 结构（metaBand / detailScroll / detailFoot），
 * 宽度自适应宿主（rail 或 drawer）。item 为 useDrawerItem 的返回，drawer 关闭动画
 * 期间保留最后一项；rail 形态卸载即消失，行为一致可接受。
 * children contract: render the detailCol structure (metaBand / detailScroll /
 * detailFoot); width adapts to the host. item comes from useDrawerItem so the
 * drawer keeps the last item while sliding out; the rail unmounts immediately.
 */
export function DetailHost({
  open,
  hasContent,
  onOpenChange,
  narrow,
  title,
  meta,
  nav,
  wide,
  drawerWidth,
  children,
}: {
  open: boolean;
  /** 关闭动画期间内容仍需挂载：传 useDrawerItem 的保留项是否非空。Content stays mounted during the close animation: whether the retained useDrawerItem value is non-null. */
  hasContent: boolean;
  /** 关闭请求：宿主不负责 dirty 判定，由调用方在 onOpenChange(false) 里拦截。Close request; dirty interception is the caller's job. */
  onOpenChange: (open: boolean) => void;
  narrow: boolean;
  title: string;
  meta?: ReactNode;
  nav?: DetailNav;
  /** 表单类详情（作业级规则）用更宽的栏。Wider rail for form-like details. */
  wide?: boolean;
  /** drawer 形态宽度。Drawer width in narrow mode. */
  drawerWidth?: string;
  children: ReactNode;
}) {
  // J/K 在列表项间切换；输入控件内不拦截。
  // J/K switch between list items; ignored while typing in a control.
  useEffect(() => {
    if (!open || !nav) {
      return;
    }
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el?.closest('input, textarea, select, [contenteditable="true"]')) {
        return;
      }
      if (e.key === 'j' || e.key === 'J') {
        e.preventDefault();
        nav.onNext();
      } else if (e.key === 'k' || e.key === 'K') {
        e.preventDefault();
        nav.onPrev();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, nav]);

  if (narrow) {
    return (
      <Drawer open={open} onOpenChange={onOpenChange}>
        {hasContent ? (
          <DrawerContent
            side='right'
            title={title}
            description={typeof meta === 'string' ? meta : undefined}
            className={cn(
              // DrawerContent portal 到 body，容器变量传不进去，.arc-compact + 贴边圆角必须重挂。
              // DrawerContent portals to <body>; re-apply .arc-compact and edge radii there.
              'arc-compact',
              styles.drawerPanel,
              drawerWidth ?? 'w-[min(92vw,480px)]',
              '[&>div[class*=body]]:flex-1 [&>div[class*=body]]:p-0!',
            )}
          >
            <div className={styles.detailFill}>
              {nav ? (
                <div className={styles.drawerNav}>
                  <NavControls nav={nav} />
                </div>
              ) : null}
              {children}
            </div>
          </DrawerContent>
        ) : null}
      </Drawer>
    );
  }

  if (!open) {
    return null;
  }

  return (
    <aside
      className={cn(styles.rail, wide && styles.railWide)}
      data-testid='detail-rail'
    >
      <header className={styles.railHead}>
        <div className={styles.railTitles}>
          <div className={styles.railTitle} title={title}>
            {title}
          </div>
          {meta ? <div className={styles.railMeta}>{meta}</div> : null}
        </div>
        {nav ? <NavControls nav={nav} /> : null}
        <Button
          type='button'
          variant='ghost'
          size='sm'
          iconOnly
          aria-label='Close'
          onClick={() => onOpenChange(false)}
        >
          <X className='size-4' aria-hidden />
        </Button>
      </header>
      <div className={styles.detailFill}>{children}</div>
    </aside>
  );
}
