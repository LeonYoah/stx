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

import {useState, useEffect, useMemo, memo} from 'react';
import {useTranslations} from 'next-intl';
import {FloatingDock} from '@/components/ui/floating-dock';
import {
  BarChartIcon,
  BellRing,
  User,
  LogOutIcon,
  Globe,
  Check,
  GithubIcon,
  Users,
  Server,
  Layers,
  Terminal,
  ScrollText,
  Package,
  Puzzle,
  BugPlay,
  ScanSearch,
} from 'lucide-react';
import {useThemeUtils} from '@/hooks/use-theme-utils';
import {useAuth} from '@/hooks/use-auth';
import {CountingNumber} from '@/components/animate-ui/text/counting-number';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import Link from 'next/link';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/animate-ui/radix/dialog';
import {Avatar, AvatarFallback, AvatarImage} from '@/components/ui/avatar';
import {TrustLevel} from '@/lib/services/core';
import {useLocale, locales, localeNames, Locale} from '@/lib/i18n';
import services from '@/lib/services';
import {STXMark} from '@/components/icons/logo';

const IconOptions = {
  className: 'h-4 w-4',
} as const;

/**
 * Dock 静态图标：按入口语义选型，避免通用图标误导。
 * Dock static icons: pick by entry semantics to avoid misleading generic glyphs.
 */
const StaticIcons = {
  user: <User {...IconOptions} />,
  users: <Users {...IconOptions} />,
  // 主机：单机节点 / Hosts: machine nodes
  server: <Server {...IconOptions} />,
  // 集群：分层编排的计算集群 / Clusters: layered compute fleet
  layers: <Layers {...IconOptions} />,
  terminal: <Terminal {...IconOptions} />,
  // 审计：滚动式操作记录 / Audit: scroll-like operation trail
  scrollText: <ScrollText {...IconOptions} />,
  // 安装包 / Packages
  package: <Package {...IconOptions} />,
  // 插件市场 / Plugin marketplace
  puzzle: <Puzzle {...IconOptions} />,
  // 工作台：调试作业运行台 / Workbench: debug-and-run studio
  workbench: <BugPlay {...IconOptions} />,
  // 控制台：品牌青鸾标 / Dashboard: brand Qingluan mark
  dashboard: <STXMark className='size-5 object-contain' />,
  // 监控：告警脉搏 / Monitoring: alert pulse
  monitoring: <BellRing {...IconOptions} />,
  // 诊断：问题扫描排查 / Diagnostics: scan and triage
  diagnostics: <ScanSearch {...IconOptions} />,
  divider: <div />,
};

// 个人信息按钮 - 独立组件
const ProfileButton = memo(() => {
  const themeUtils = useThemeUtils();
  const {user, isLoading, logout, checkAuthStatus} = useAuth();
  const {locale, syncLocale} = useLocale();
  const [mounted, setMounted] = useState(false);
  const [emailDraft, setEmailDraft] = useState('');
  const [savingEmail, setSavingEmail] = useState(false);
  const [emailError, setEmailError] = useState<string | null>(null);
  const [emailSaved, setEmailSaved] = useState(false);
  const [languageDraft, setLanguageDraft] = useState<Locale>('zh');
  const [savingLanguage, setSavingLanguage] = useState(false);
  const [languageError, setLanguageError] = useState<string | null>(null);
  const [languageSaved, setLanguageSaved] = useState(false);
  const t = useTranslations('profile');

  useEffect(() => {
    setMounted(true);
  }, []);

  useEffect(() => {
    setEmailDraft(user?.email || '');
    setEmailError(null);
    setEmailSaved(false);
    const nextLanguage =
      user?.language && locales.includes(user.language as Locale)
        ? (user.language as Locale)
        : locale;
    setLanguageDraft(nextLanguage);
    setLanguageError(null);
    setLanguageSaved(false);
  }, [locale, user?.email, user?.id, user?.language]);

  useEffect(() => {
    if (!mounted || !user?.language) {
      return;
    }
    if (!locales.includes(user.language as Locale)) {
      return;
    }
    syncLocale(user.language as Locale);
  }, [mounted, syncLocale, user?.language]);

  const getTrustLevelText = (level: number): string => {
    switch (level) {
      case TrustLevel.NEW_USER:
        return t('trustLevel.newUser');
      case TrustLevel.BASIC_USER:
        return t('trustLevel.basicUser');
      case TrustLevel.USER:
        return t('trustLevel.member');
      case TrustLevel.ACTIVE_USER:
        return t('trustLevel.activeUser');
      case TrustLevel.LEADER:
        return t('trustLevel.leader');
      default:
        return t('unknown');
    }
  };

  const handleLogout = () => {
    logout('/login').catch((error) => {
      console.error('登出失败:', error);
    });
  };

  const handleSaveEmail = async () => {
    if (!user) {
      return;
    }

    const nextEmail = emailDraft.trim();
    if (!nextEmail) {
      setEmailError(t('emailRequired'));
      return;
    }
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(nextEmail)) {
      setEmailError(t('invalidEmail'));
      return;
    }

    try {
      setSavingEmail(true);
      setEmailError(null);
      await services.auth.updateProfile({email: nextEmail});
      await checkAuthStatus(true);
      setEmailSaved(true);
    } catch (error) {
      setEmailError(
        error instanceof Error ? error.message : t('emailSaveFailed'),
      );
      setEmailSaved(false);
    } finally {
      setSavingEmail(false);
    }
  };

  const handleSaveLanguage = async () => {
    if (!user) {
      return;
    }
    const nextLanguage = languageDraft;
    if (!locales.includes(nextLanguage)) {
      setLanguageError(t('languageInvalid'));
      return;
    }

    try {
      setSavingLanguage(true);
      setLanguageError(null);
      await services.auth.updateProfile({language: nextLanguage});
      syncLocale(nextLanguage);
      await checkAuthStatus(true);
      setLanguageSaved(true);
    } catch (error) {
      setLanguageError(
        error instanceof Error ? error.message : t('languageSaveFailed'),
      );
      setLanguageSaved(false);
    } finally {
      setSavingLanguage(false);
    }
  };

  return (
    <Dialog>
      <DialogTrigger asChild>
        <div className='w-full h-full flex items-center justify-center cursor-pointer rounded transition-colors'>
          <User className='h-4 w-4' />
        </div>
      </DialogTrigger>
      <DialogContent className='max-w-md'>
        <DialogHeader>
          <DialogTitle>{t('title')}</DialogTitle>
        </DialogHeader>
        <div className='space-y-4'>
          {!isLoading && user && (
            <>
              {/* 用户信息卡片 */}
              <div className='bg-muted/20 rounded-lg p-4 space-y-4'>
                <div className='flex items-center justify-between gap-3'>
                  <div className='flex items-center gap-3 flex-1 min-w-0'>
                    <Avatar className='h-12 w-12 rounded-full ring-2 ring-background'>
                      <AvatarImage src={user.avatar_url} alt={user.username} />
                      <AvatarFallback className='rounded-lg bg-primary/10 text-primary font-medium'>
                        CN
                      </AvatarFallback>
                    </Avatar>
                    <div className='flex-1 min-w-0'>
                      <div className='font-semibold truncate'>
                        {user.username}
                      </div>
                      {user.nickname && (
                        <div className='text-sm text-muted-foreground truncate'>
                          {user.nickname}
                        </div>
                      )}
                      <div className='text-xs text-muted-foreground mt-1 flex items-center gap-2'>
                        <span>
                          {user.trust_level !== undefined
                            ? getTrustLevelText(user.trust_level)
                            : t('unknown')}
                        </span>
                        <span>•</span>
                        <span>{user.id}</span>
                      </div>
                    </div>
                  </div>
                  <Button
                    onClick={handleLogout}
                    variant='destructive'
                    size='sm'
                    className='shrink-0'
                  >
                    <LogOutIcon className='w-4 h-4 mr-1' />
                    {t('logout')}
                  </Button>
                </div>
              </div>

              {/* 用户分数 */}
              {user.score !== undefined && (
                <div>
                  <h4 className='text-sm font-semibold mb-3 text-muted-foreground'>
                    {t('communityScore')}
                  </h4>
                  <div className='text-lg font-bold text-primary flex items-center gap-2'>
                    <BarChartIcon className='h-4 w-4 text-primary' />
                    <CountingNumber
                      number={user.score || 0}
                      fromNumber={0}
                      inView={true}
                      transition={{stiffness: 200, damping: 25}}
                    />
                  </div>
                </div>
              )}

              <div>
                <h4 className='text-sm font-semibold mb-3 text-muted-foreground'>
                  {t('emailSettings')}
                </h4>
                <div className='space-y-2'>
                  <Label htmlFor='profile-email'>{t('emailLabel')}</Label>
                  <div className='flex items-center gap-2'>
                    <Input
                      id='profile-email'
                      type='email'
                      value={emailDraft}
                      placeholder={t('emailPlaceholder')}
                      onChange={(event) => {
                        setEmailDraft(event.target.value);
                        setEmailError(null);
                        setEmailSaved(false);
                      }}
                      disabled={savingEmail}
                    />
                    <Button
                      size='sm'
                      onClick={handleSaveEmail}
                      disabled={
                        savingEmail ||
                        !user ||
                        emailDraft.trim() === (user.email || '').trim()
                      }
                    >
                      {savingEmail ? t('savingEmail') : t('saveEmail')}
                    </Button>
                  </div>
                  {emailError ? (
                    <p className='text-xs text-destructive'>{emailError}</p>
                  ) : null}
                  {!emailError && emailSaved ? (
                    <p className='text-xs text-emerald-600'>
                      {t('emailSaved')}
                    </p>
                  ) : null}
                </div>
              </div>
            </>
          )}

          {/* 主题设置 */}
          {mounted && (
            <div>
              <h4 className='text-sm font-semibold mb-3 text-muted-foreground'>
                {t('themeSettings')}
              </h4>
              <div className='flex items-center justify-between'>
                <button
                  onClick={themeUtils.toggle}
                  className='flex items-center gap-2 px-3 py-2 rounded-md bg-muted/50 hover:bg-muted/80 transition-colors'
                >
                  {themeUtils.getIcon('h-4 w-4')}
                  <span className='text-sm'>{themeUtils.getAction()}</span>
                </button>
              </div>
            </div>
          )}

          {/* 语言设置 */}
          {mounted && (
            <div>
              <h4 className='text-sm font-semibold mb-3 text-muted-foreground'>
                {t('languageSettings')}
              </h4>
              <div className='flex items-center gap-2'>
                {locales.map((loc) => (
                  <button
                    key={loc}
                    onClick={() => {
                      setLanguageDraft(loc as Locale);
                      setLanguageError(null);
                      setLanguageSaved(false);
                    }}
                    className={`flex items-center gap-2 px-3 py-2 rounded-md transition-colors ${
                      languageDraft === loc
                        ? 'bg-primary/10 text-primary border border-primary/30'
                        : 'bg-muted/50 hover:bg-muted/80'
                    }`}
                    disabled={savingLanguage}
                  >
                    <Globe className='h-4 w-4' />
                    <span className='text-sm'>
                      {localeNames[loc as Locale]}
                    </span>
                    {languageDraft === loc && <Check className='h-3 w-3' />}
                  </button>
                ))}
              </div>
              <div className='mt-3 flex items-center gap-2'>
                <Button
                  size='sm'
                  onClick={handleSaveLanguage}
                  disabled={
                    savingLanguage ||
                    !user ||
                    languageDraft === (user.language || locale)
                  }
                >
                  {savingLanguage ? t('savingLanguage') : t('saveLanguage')}
                </Button>
                {languageError ? (
                  <p className='text-xs text-destructive'>{languageError}</p>
                ) : null}
                {!languageError && languageSaved ? (
                  <p className='text-xs text-emerald-600'>
                    {t('languageSaved')}
                  </p>
                ) : null}
              </div>
            </div>
          )}

          {/* 快速链接区域 */}
          <div>
            <h4 className='text-sm font-semibold mb-3 text-muted-foreground'>
              {t('quickLinks')}
            </h4>
            <div className='grid grid-cols-2 gap-2'>
              <Link
                href='https://github.com/apache/seatunnel'
                target='_blank'
                rel='noopener noreferrer'
                className='flex items-center gap-3 p-2 rounded-md hover:bg-muted/50 transition-colors group'
              >
                <div className='flex items-center justify-center w-8 h-8 rounded-md bg-orange-500/10 group-hover:bg-orange-500/20 transition-colors'>
                  <GithubIcon className='h-4 w-4 text-orange-600' />
                </div>
                <span className='text-sm font-medium'>
                  {t('seatunnelRepo')}
                </span>
              </Link>
              <Link
                href='https://github.com/LeonYoah/stx'
                target='_blank'
                rel='noopener noreferrer'
                className='flex items-center gap-3 p-2 rounded-md hover:bg-muted/50 transition-colors group'
              >
                <div className='flex items-center justify-center w-8 h-8 rounded-md bg-blue-500/10 group-hover:bg-blue-500/20 transition-colors'>
                  <GithubIcon className='h-4 w-4 text-blue-600' />
                </div>
                <span className='text-sm font-medium'>{t('stxRepo')}</span>
              </Link>
            </div>
          </div>

          <div>
            <h4 className='text-sm font-semibold mb-3 text-muted-foreground'>
              {t('about')}
            </h4>
            <div className='space-y-2'>
              <div className='text-xs text-muted-foreground font-light'>
                {t('version')}: 1.1.0
              </div>
              <div className='text-xs text-muted-foreground font-light'>
                {t('buildTime')}: 2025-09-27
              </div>
              <div className='text-xs text-muted-foreground font-light'>
                {t('description')}
              </div>
            </div>
          </div>

          {!isLoading && !user && (
            <div className='text-center text-muted-foreground'>
              {t('notLoggedIn')}
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
});
ProfileButton.displayName = 'ProfileButton';

// Dock 项目类型
interface DockItem {
  title: string;
  icon: React.ReactNode;
  href?: string;
  customComponent?: React.ReactNode;
}

export function ManagementBar() {
  const {user} = useAuth();
  const tDock = useTranslations('dock');

  // 使用 useMemo 缓存 dockItems，只在关键依赖变化时重新计算
  const dockItems = useMemo((): DockItem[] => {
    const items: DockItem[] = [];

    // 控制台入口 / Dashboard entry
    items.push({
      title: tDock('dashboard'),
      icon: StaticIcons.dashboard,
      href: '/dashboard',
    });

    items.push({
      title: tDock('workbench'),
      icon: StaticIcons.workbench,
      href: '/workbench',
    });

    // 主机管理入口 / Host management entry
    items.push({
      title: tDock('hostManagement'),
      icon: StaticIcons.server,
      href: '/hosts',
    });

    // 集群管理入口 / Cluster management entry
    items.push({
      title: tDock('clusterManagement'),
      icon: StaticIcons.layers,
      href: '/clusters',
    });

    // 监控中心入口 / Monitoring center entry
    items.push({
      title: tDock('monitoringCenter'),
      icon: StaticIcons.monitoring,
      href: '/monitoring',
    });

    // 诊断中心入口 / Diagnostics center entry
    items.push({
      title: tDock('diagnosticsCenter'),
      icon: StaticIcons.diagnostics,
      href: '/diagnostics',
    });

    // 安装包管理入口 / Package management entry
    items.push({
      title: tDock('packageManagement'),
      icon: StaticIcons.package,
      href: '/packages',
    });

    // 插件市场入口 / Plugin marketplace entry
    items.push({
      title: tDock('pluginMarketplace'),
      icon: StaticIcons.puzzle,
      href: '/plugins',
    });

    // 命令记录入口暂时隐藏 / Command logs entry hidden for now
    // items.push({
    //   title: tDock('commandLogs'),
    //   icon: StaticIcons.terminal,
    //   href: '/commands',
    // });

    // 审计日志入口 / Audit logs entry
    items.push({
      title: tDock('auditLogs'),
      icon: StaticIcons.scrollText,
      href: '/audit-logs',
    });

    // 管理员入口 / Admin entry
    if (user?.is_admin) {
      items.push({
        title: tDock('userManagement'),
        icon: StaticIcons.users,
        href: '/admin/users',
      });
    }

    // 分隔符 / Divider
    items.push({
      title: 'divider',
      icon: StaticIcons.divider,
    });

    // 个人信息
    items.push({
      title: tDock('profile'),
      icon: StaticIcons.user,
      customComponent: <ProfileButton />,
    });

    return items;
  }, [user?.is_admin, tDock]);

  return (
    <div className='fixed z-50 bottom-4 right-4 pb-[max(1rem,env(safe-area-inset-bottom))] md:pb-0 md:bottom-4 md:left-1/2 md:-translate-x-1/2 md:right-auto'>
      <FloatingDock
        items={dockItems}
        desktopClassName='bg-background/70 backdrop-blur-md border border-border/40 shadow-lg shadow-black/10 dark:shadow-white/5 h-16 pb-3 px-4 gap-2'
        mobileButtonClassName='bg-background/70 backdrop-blur-md border border-border/40 shadow-lg shadow-black/10 dark:shadow-white/5 h-12 w-12'
      />
    </div>
  );
}
