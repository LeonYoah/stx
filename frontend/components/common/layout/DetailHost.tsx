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
import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  useSyncExternalStore,
  type ReactNode,
  type RefObject,
} from 'react';
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

/** 名称飞入的头部落点：title=标题，meta=元信息。Fly target slot in the rail header. */
export type DetailNameSlot = 'title' | 'meta';

/**
 * 名称飞入前置判定（纯函数）：没有来源行 / 窄屏 / 减少动态效果 /
 * 未配置 nameSlot / 来源与落点文字不一致 一律不飞。
 * Pure gate for the name fly-in: no origin row, narrow layout, reduced
 * motion, missing nameSlot, or mismatched source/target text all skip it.
 */
export function canFlyDetailName(params: {
  origin: HTMLElement | null | undefined;
  narrow: boolean;
  reducedMotion: boolean;
  nameSlot?: DetailNameSlot;
  sourceText: string;
  targetText: string;
}): boolean {
  const {origin, narrow, reducedMotion, nameSlot, sourceText, targetText} =
    params;
  return Boolean(
    origin &&
      origin.isConnected &&
      !narrow &&
      !reducedMotion &&
      nameSlot &&
      sourceText.trim() &&
      sourceText.trim() === targetText.trim(),
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
  flyOrigin,
  nameSlot,
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
  /** 行点击/回车打开时写入的行元素，用于名称飞入；J/K、↑↓、深链不写入。
      Row element recorded on click/Enter open, for the name fly-in; J/K,
      header arrows and deep links never write it. */
  flyOrigin?: RefObject<HTMLElement | null>;
  /** 行内名称飞入的头部落点。Where the row name lands in the rail header. */
  nameSlot?: DetailNameSlot;
  children: ReactNode;
}) {
  // 常驻栏开合生命周期：打开时先以 closed 态挂载一帧再翻 open（进入过渡从
  // 收起态起跑）；关闭时翻回 closed，宽度过渡结束（600ms 兜底）后才卸载；
  // 关闭途中重新打开取消卸载。
  // Rail open/close lifecycle: on open it mounts one frame as 'closed' then
  // flips to 'open' so the entrance transition has a start state; on close it
  // flips back and stays mounted until the width transition ends (600ms
  // fallback); reopening mid-close cancels the unmount.
  const [present, setPresent] = useState(open);
  const [entered, setEntered] = useState(false);
  const railRef = useRef<HTMLElement | null>(null);

  // 打开帧内同步挂载栏：render 期校正状态，让 open 变 true 的同一个提交里
  // rail 就进 DOM，名称飞入的 useLayoutEffect 才能量到落点。
  // Mount the rail within the opening commit via render-phase state
  // adjustment so the fly-in's useLayoutEffect can measure its target.
  if (open && !present) {
    setPresent(true);
  }

  // 分区入场节奏：打开用 enter，开着时换内容用 switch。
  // Section entrance pacing: 'enter' on open, 'switch' when content changes
  // while staying open.
  const [motion, setMotion] = useState<'enter' | 'switch'>('enter');
  const prevRef = useRef<{open: boolean; title: string; metaKey: string}>({
    open: false,
    title: '',
    metaKey: '',
  });

  const flyRef = useRef<{
    ghost: HTMLElement;
    target: HTMLElement;
    anim: Animation;
  } | null>(null);

  const clearFly = useCallback(() => {
    const f = flyRef.current;
    if (!f) {
      return;
    }
    f.anim.cancel();
    f.ghost.remove();
    f.target.style.opacity = '';
    flyRef.current = null;
  }, []);

  useEffect(() => {
    if (!open) {
      setEntered(false);
      if (!present) {
        return;
      }
      const el = railRef.current;
      if (!el) {
        setPresent(false);
        return;
      }
      let done = false;
      const finish = () => {
        if (!done) {
          done = true;
          setPresent(false);
        }
      };
      const onEnd = (e: TransitionEvent) => {
        if (e.target === el && e.propertyName === 'width') {
          finish();
        }
      };
      const timer = window.setTimeout(finish, 600);
      el.addEventListener('transitionend', onEnd);
      return () => {
        window.clearTimeout(timer);
        el.removeEventListener('transitionend', onEnd);
      };
    }
    // present 已在 render 期校正；这里只负责下一帧翻 open 让进入过渡有起点。
    // `present` is corrected during render; this only flips to 'open' a frame
    // later so the entrance transition has a start state.
    const raf =
      typeof requestAnimationFrame === 'function'
        ? requestAnimationFrame
        : (cb: FrameRequestCallback) =>
            window.setTimeout(cb, 0) as unknown as number;
    const caf =
      typeof cancelAnimationFrame === 'function'
        ? cancelAnimationFrame
        : (id: number) => window.clearTimeout(id);
    let raf2 = 0;
    const raf1 = raf(() => {
      raf2 = raf(() => setEntered(true));
    });
    return () => {
      caf(raf1);
      if (raf2) {
        caf(raf2);
      }
    };
  }, [open, present]);

  useEffect(() => {
    const metaKey =
      typeof meta === 'string' || typeof meta === 'number' ? String(meta) : '';
    if (open) {
      setMotion(
        prevRef.current.open &&
          (prevRef.current.title !== title ||
            prevRef.current.metaKey !== metaKey)
          ? 'switch'
          : 'enter',
      );
    }
    prevRef.current = {open, title, metaKey};
  }, [open, title, meta]);

  // 名称飞入：点击/回车打开或切换时，行内名称的副本从列表飞到头部落点；
  // 落地后换回真实文字。再次选中、关闭或卸载时取消并清理副本。
  // Name fly-in: on click/Enter open or row switch, a clone of the row's name
  // flies to the header slot, then the real text takes over. Reselecting,
  // closing or unmounting cancels and removes the clone.
  useLayoutEffect(() => {
    const rail = railRef.current;
    const origin = flyOrigin?.current;
    if (flyOrigin) {
      flyOrigin.current = null;
    }
    if (!open || !rail || !origin) {
      return clearFly;
    }
    const src = origin.querySelector<HTMLElement>('[data-detail-name]');
    const tgt = nameSlot
      ? rail.querySelector<HTMLElement>(`[data-detail-name-target='${nameSlot}']`)
      : null;
    const reduced =
      window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches ?? false;
    if (
      !canFlyDetailName({
        origin,
        narrow,
        reducedMotion: reduced,
        nameSlot,
        sourceText: src?.textContent ?? '',
        targetText: tgt?.textContent ?? '',
      }) ||
      !src ||
      !tgt
    ) {
      return clearFly;
    }
    const srcRect = src.getBoundingClientRect();
    const tgtRect = tgt.getBoundingClientRect();
    const railRect = rail.getBoundingClientRect();
    const splitRect = rail.parentElement?.getBoundingClientRect() ?? railRect;
    const cs = getComputedStyle(rail);
    const railW = parseFloat(cs.getPropertyValue('--rail-w')) || railRect.width;
    const maxPct = parseFloat(cs.maxWidth) / 100 || 1;
    // 落点取栏的最终位置（分栏右缘 − 栏宽），不受栏正在滑入的位移影响；
    // 纵向不动画，直接用当前 top。偏移部分用矩形差，等价于 offset 链累加。
    // The target uses the rail's final position (split right edge − rail
    // width), unaffected by the slide-in. Vertically nothing animates so the
    // current top is final; the inner offset is a rect delta, equivalent to
    // walking the offsetParent chain.
    const railLeft = splitRect.right - Math.min(railW, maxPct * splitRect.width);
    const dx = railLeft + (tgtRect.left - railRect.left) - srcRect.left;
    const dy = tgtRect.top - srcRect.top;
    const srcFont = parseFloat(getComputedStyle(src).fontSize);
    const scale = parseFloat(getComputedStyle(tgt).fontSize) / srcFont || 1;
    const ghost = src.cloneNode(true) as HTMLElement;
    ghost.removeAttribute('data-detail-name');
    Object.assign(ghost.style, {
      position: 'fixed',
      left: `${srcRect.left}px`,
      top: `${srcRect.top}px`,
      width: `${srcRect.width}px`,
      margin: '0',
      transformOrigin: '0 0',
      pointerEvents: 'none',
      zIndex: '60',
    });
    document.body.appendChild(ghost);
    tgt.style.opacity = '0';
    if (typeof ghost.animate !== 'function') {
      tgt.style.opacity = '';
      ghost.remove();
      return clearFly;
    }
    const anim = ghost.animate(
      [
        {transform: 'translate(0, 0) scale(1)'},
        {transform: `translate(${dx}px, ${dy}px) scale(${scale})`},
      ],
      {duration: 520, easing: 'cubic-bezier(.22,1,.36,1)', fill: 'forwards'},
    );
    anim.onfinish = () => {
      tgt.style.opacity = '';
      ghost.remove();
      flyRef.current = null;
    };
    flyRef.current = {ghost, target: tgt, anim};
    return clearFly;
  }, [open, narrow, title, meta, nameSlot, flyOrigin, clearFly]);

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

  if (!present) {
    return null;
  }

  return (
    <aside
      ref={railRef}
      className={cn(styles.rail, wide && styles.railWide)}
      data-state={entered && open ? 'open' : 'closed'}
      data-motion={motion}
      data-testid='detail-rail'
    >
      <header className={styles.railHead}>
        <div className={styles.railTitles}>
          <div
            className={styles.railTitle}
            title={title}
            data-detail-name-target={nameSlot === 'title' ? 'title' : undefined}
          >
            {title}
          </div>
          {meta ? (
            <div
              className={styles.railMeta}
              data-detail-name-target={nameSlot === 'meta' ? 'meta' : undefined}
            >
              {meta}
            </div>
          ) : null}
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
