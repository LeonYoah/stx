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

import {cn} from '@/lib/utils';
import {IconLayoutNavbarCollapse} from '@tabler/icons-react';
import {
  AnimatePresence,
  MotionValue,
  motion,
  useMotionValue,
  useSpring,
  useTransform,
} from 'motion/react';
import Link from 'next/link';
import {usePathname} from 'next/navigation';
import {useRef, useState, memo, useCallback, useEffect} from 'react';

export interface FloatingDockItem {
  title: string;
  icon: React.ReactNode;
  href?: string;
  onClick?: () => void;
  tooltip?: string;
  customComponent?: React.ReactNode;
  external?: boolean;
  isActive?: boolean;
}

export const FloatingDock = memo(
  ({
    items,
    desktopClassName,
    mobileClassName,
    mobileButtonClassName,
  }: {
    items: FloatingDockItem[];
    desktopClassName?: string;
    mobileClassName?: string;
    mobileButtonClassName?: string;
  }) => {
    return (
      <>
        <FloatingDockDesktop items={items} className={desktopClassName} />
        <FloatingDockMobile
          items={items}
          className={mobileClassName}
          buttonClassName={mobileButtonClassName}
        />
      </>
    );
  },
);

FloatingDock.displayName = 'FloatingDock';

const FloatingDockMobile = memo(
  ({
    items,
    className,
    buttonClassName,
  }: {
    items: FloatingDockItem[];
    className?: string;
    buttonClassName?: string;
  }) => {
    const [open, setOpen] = useState(false);
    const pathname = usePathname();

    const toggleOpen = useCallback(() => {
      setOpen((prev) => !prev);
    }, []);

    const isItemActive = useCallback(
      (item: FloatingDockItem) => {
        if (item.isActive !== undefined) {
          return item.isActive;
        }
        if (!item.href || !pathname) {
          return false;
        }
        if (item.href === '/dashboard') {
          return pathname === '/dashboard' || pathname === '/';
        }
        return pathname === item.href || pathname.startsWith(item.href + '/');
      },
      [pathname],
    );

    return (
      <div className={cn('relative block md:hidden', className)}>
        <AnimatePresence>
          {open && (
            <motion.div
              layoutId='nav'
              className='absolute inset-x-0 bottom-full mb-2 flex flex-col gap-1'
            >
              {items.map((item, idx) => {
                if (item.title === 'divider') {
                  return null; // Mobile版本不显示分隔符 / Hide divider in mobile
                }
                const active = isItemActive(item);
                const isExternal = Boolean(
                  item.external ||
                    item.href?.startsWith('https://') ||
                    item.href?.startsWith('http://'),
                );

                return (
                  <motion.div
                    key={item.title}
                    initial={{opacity: 0, y: 10}}
                    animate={{
                      opacity: 1,
                      y: 0,
                    }}
                    exit={{
                      opacity: 0,
                      y: 10,
                      transition: {
                        delay: idx * 0.04,
                      },
                    }}
                    transition={{delay: (items.length - 1 - idx) * 0.04}}
                  >
                    {item.customComponent ? (
                      <div
                        className={cn(
                          'flex h-9 w-9 items-center justify-center rounded-full bg-gray-50 dark:bg-neutral-900',
                          buttonClassName,
                        )}
                      >
                        {item.customComponent}
                      </div>
                    ) : item.href ? (
                      isExternal ? (
                        <a
                          href={item.href}
                          target='_blank'
                          rel='noopener noreferrer'
                          className={cn(
                            'flex h-9 w-9 items-center justify-center rounded-full transition-colors',
                            active
                              ? 'bg-primary/20 text-primary border border-primary/40'
                              : 'bg-gray-50 dark:bg-neutral-900 text-muted-foreground hover:text-foreground',
                            buttonClassName,
                          )}
                        >
                          <div className='flex items-center justify-center'>
                            {item.icon}
                          </div>
                        </a>
                      ) : (
                        <Link
                          href={item.href}
                          prefetch={true}
                          onClick={() => {
                            item.onClick?.();
                            setOpen(false);
                          }}
                          className={cn(
                            'flex h-9 w-9 items-center justify-center rounded-full transition-colors',
                            active
                              ? 'bg-primary/20 text-primary border border-primary/40'
                              : 'bg-gray-50 dark:bg-neutral-900 text-muted-foreground hover:text-foreground',
                            buttonClassName,
                          )}
                        >
                          <div className='flex items-center justify-center'>
                            {item.icon}
                          </div>
                        </Link>
                      )
                    ) : (
                      <button
                        type='button'
                        onClick={() => {
                          item.onClick?.();
                          setOpen(false);
                        }}
                        className={cn(
                          'flex h-9 w-9 items-center justify-center rounded-full transition-colors',
                          buttonClassName,
                        )}
                      >
                        <div className='flex items-center justify-center'>
                          {item.icon}
                        </div>
                      </button>
                    )}
                  </motion.div>
                );
              })}
            </motion.div>
          )}
        </AnimatePresence>
        <button
          type='button'
          onClick={toggleOpen}
          className={cn(
            'flex h-9 w-9 items-center justify-center rounded-full bg-gray-50 dark:bg-neutral-800',
            buttonClassName,
          )}
        >
          <motion.div
            animate={{rotate: open ? 180 : 0}}
            transition={{duration: 0.25, ease: 'easeInOut'}}
          >
            <IconLayoutNavbarCollapse className='h-4 w-4 text-neutral-500 dark:text-neutral-400' />
          </motion.div>
        </button>
      </div>
    );
  },
);

FloatingDockMobile.displayName = 'FloatingDockMobile';

const FloatingDockDesktop = memo(
  ({
    items,
    className,
  }: {
    items: FloatingDockItem[];
    className?: string;
  }) => {
    const mouseX = useMotionValue(Infinity);

    const handleMouseMove = useCallback(
      (e: React.MouseEvent) => {
        mouseX.set(e.pageX);
      },
      [mouseX],
    );

    const handleMouseLeave = useCallback(() => {
      mouseX.set(Infinity);
    }, [mouseX]);

    return (
      <motion.div
        onMouseMove={handleMouseMove}
        onMouseLeave={handleMouseLeave}
        className={cn(
          'mx-auto hidden items-end gap-2 rounded-xl bg-gray-50 px-2 pb-2 md:flex dark:bg-neutral-900',
          className,
        )}
      >
        {(() => {
          const dividerIndex = items.findIndex(
            (item) => item.title === 'divider',
          );
          // 取 divider 之前的所有项目，如果没有 divider 则取全部
          // Take all items before divider, or all items if no divider exists
          const firstRow =
            dividerIndex !== -1 ? items.slice(0, dividerIndex) : items;
          const secondRow =
            dividerIndex !== -1 ? items.slice(dividerIndex + 1) : [];

          return (
            <>
              <div className='flex items-end gap-2'>
                {firstRow.map((item) => (
                  <IconContainer mouseX={mouseX} key={item.title} {...item} />
                ))}
              </div>
              {dividerIndex !== -1 && (
                <div className='flex items-center justify-center px-1 mx-1 self-stretch mt-3'>
                  <div className='w-px h-full bg-border'></div>
                </div>
              )}
              {secondRow.length > 0 && (
                <div className='flex items-end gap-2'>
                  {secondRow.map((item) => (
                    <IconContainer mouseX={mouseX} key={item.title} {...item} />
                  ))}
                </div>
              )}
            </>
          );
        })()}
      </motion.div>
    );
  },
);

FloatingDockDesktop.displayName = 'FloatingDockDesktop';

const IconContainer = memo(
  ({
    mouseX,
    title,
    icon,
    href,
    onClick,
    tooltip,
    customComponent,
    external,
    isActive,
  }: FloatingDockItem & {
    mouseX: MotionValue;
  }) => {
    const ref = useRef<HTMLDivElement>(null);
    const centerRef = useRef<number>(0);
    const pathname = usePathname();

    // 缓存图标中心 X 轴坐标，杜绝 mousemove 过程中的强制布局回流 (Layout Thrashing)
    // Cache icon center X coordinate to prevent layout thrashing on every mousemove event
    const updateCenter = useCallback(() => {
      if (ref.current) {
        const bounds = ref.current.getBoundingClientRect();
        centerRef.current = bounds.x + bounds.width / 2;
      }
    }, []);

    useEffect(() => {
      updateCenter();
      window.addEventListener('resize', updateCenter);
      window.addEventListener('scroll', updateCenter, {passive: true});
      return () => {
        window.removeEventListener('resize', updateCenter);
        window.removeEventListener('scroll', updateCenter);
      };
    }, [updateCenter]);

    // 计算鼠标距离：非悬停时返回 Infinity，悬停时直接内存计算，0 次 DOM 测量
    // Calculate distance: returns Infinity when idle, memory-only calculation when hovered with 0 DOM reflows
    const distance = useTransform(mouseX, (val) => {
      if (val === Infinity) {
        return Infinity;
      }
      if (centerRef.current === 0 && ref.current) {
        const bounds = ref.current.getBoundingClientRect();
        centerRef.current = bounds.x + bounds.width / 2;
      }
      return val - centerRef.current;
    });

    // 统一尺寸与图标尺寸弹簧：由 4 个弹簧减半为 2 个弹簧，物理开销减半
    // Consolidated size and iconSize springs: cut spring instances from 4 to 2, halving physics compute overhead
    const sizeTransform = useTransform(distance, [-150, 0, 150], [40, 68, 40]);
    const iconSizeTransform = useTransform(
      distance,
      [-150, 0, 150],
      [20, 34, 20],
    );

    const size = useSpring(sizeTransform, {
      mass: 0.1,
      stiffness: 160,
      damping: 14,
    });

    const iconSize = useSpring(iconSizeTransform, {
      mass: 0.1,
      stiffness: 160,
      damping: 14,
    });

    const [hovered, setHovered] = useState(false);

    const handleMouseEnter = useCallback(() => {
      updateCenter();
      setHovered(true);
    }, [updateCenter]);

    const handleMouseLeave = useCallback(() => {
      setHovered(false);
    }, []);

    // 自动根据当前路由计算激活状态
    // Automatically determine active state from current pathname
    const isAutoActive = Boolean(
      href &&
        pathname &&
        (href === '/dashboard'
          ? pathname === '/dashboard' || pathname === '/'
          : pathname === href || pathname.startsWith(href + '/')),
    );
    const active = isActive !== undefined ? isActive : isAutoActive;

    const isExternal = Boolean(
      external || href?.startsWith('https://') || href?.startsWith('http://'),
    );

    const content = (
      <motion.div
        ref={ref}
        style={{width: size, height: size}}
        whileTap={{scale: 0.92}}
        onMouseEnter={handleMouseEnter}
        onMouseLeave={handleMouseLeave}
        className={cn(
          'relative flex aspect-square items-center justify-center rounded-full cursor-pointer transition-colors duration-200 select-none',
          active
            ? 'bg-primary/20 text-primary border border-primary/40 shadow-[0_0_12px_rgba(59,130,246,0.28)]'
            : 'bg-muted/70 text-muted-foreground hover:text-foreground hover:bg-muted/90 border border-border/40 dark:bg-neutral-800/80 dark:hover:bg-neutral-700/80 dark:border-neutral-700/40',
        )}
      >
        <AnimatePresence>
          {hovered && (
            <motion.div
              initial={{opacity: 0, y: 10, x: '-50%'}}
              animate={{opacity: 1, y: 0, x: '-50%'}}
              exit={{opacity: 0, y: 2, x: '-50%'}}
              transition={{duration: 0.15}}
              className='pointer-events-none absolute -top-8 left-1/2 w-fit rounded-md border border-border bg-popover/95 px-2 py-0.5 text-xs whitespace-pre text-popover-foreground shadow-md backdrop-blur-sm'
            >
              {tooltip || title}
            </motion.div>
          )}
        </AnimatePresence>
        <motion.div
          style={{width: iconSize, height: iconSize}}
          className='flex items-center justify-center'
        >
          {customComponent || icon}
        </motion.div>

        {/* 激活指示光点：在不同菜单间切换时通过 layoutId 实现丝滑平移过渡 */}
        {/* Active indicator dot: glides seamlessly between menu items via layoutId transition */}
        {active && (
          <motion.span
            layoutId='floating-dock-active-dot'
            className='absolute -bottom-1 h-1 w-1 rounded-full bg-primary shadow-[0_0_6px_var(--primary)]'
            transition={{type: 'spring', stiffness: 380, damping: 26}}
          />
        )}
      </motion.div>
    );

    if (customComponent) {
      return <div className='outline-none'>{content}</div>;
    }

    if (href) {
      if (isExternal) {
        return (
          <a
            href={href}
            target='_blank'
            rel='noopener noreferrer'
            className='outline-none focus-visible:ring-2 focus-visible:ring-primary rounded-full'
          >
            {content}
          </a>
        );
      }

      return (
        <Link
          href={href}
          prefetch={true}
          onClick={onClick}
          className='outline-none focus-visible:ring-2 focus-visible:ring-primary rounded-full'
        >
          {content}
        </Link>
      );
    }

    return (
      <button
        type='button'
        onClick={onClick}
        className='outline-none focus-visible:ring-2 focus-visible:ring-primary rounded-full'
      >
        {content}
      </button>
    );
  },
);

IconContainer.displayName = 'IconContainer';
