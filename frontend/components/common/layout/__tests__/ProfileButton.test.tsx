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

import {describe, it, expect, beforeEach, vi, afterEach} from 'vitest';
import {render, screen, fireEvent, act, waitFor} from '@testing-library/react';
import {ProfileButton} from '../ManagementBar';

const push = vi.fn();

vi.mock('next/navigation', () => ({
  useRouter: () => ({push}),
  usePathname: () => '/dashboard',
}));

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, values?: Record<string, string>) => {
    const map: Record<string, string> = {
      title: '个人信息',
      logout: '退出登录',
      versionUnavailable: '暂无法获取',
      communityScore: '社区分数',
      emailSettings: '邮箱',
      emailLabel: '邮箱',
      emailPlaceholder: 'email',
      saveEmail: '保存',
      savingEmail: '保存中',
      emailSaved: 'ok',
      emailSaveFailed: 'fail',
      emailRequired: 'required',
      invalidEmail: 'invalid',
      themeSettings: '主题',
      languageSettings: '语言',
      saveLanguage: '保存语言',
      savingLanguage: '保存中',
      languageSaved: 'ok',
      languageSaveFailed: 'fail',
      languageInvalid: 'invalid',
      quickLinks: '快速链接',
      seatunnelRepo: 'SeaTunnel',
      stxRepo: 'STX',
      about: '关于 STX',
      version: '版本',
      buildTime: '构建时间',
      gitCommit: '提交',
      checkUpdate: '检查更新',
      adminOnlyTooltip: '仅管理员可升级',
      notLoggedIn: '未登录',
      unknown: '未知',
      'trustLevel.member': '成员',
      timezoneSettings: '时区',
      timezoneFollowBrowser: `跟随浏览器（${values?.timezone || ''}）`,
      timezoneHint: 'hint',
      saveTimezone: '保存时区',
      savingTimezone: '保存中',
      timezoneSaved: 'ok',
      timezoneSaveFailed: 'fail',
      timezoneInvalid: 'invalid',
    };
    return map[key] || key;
  },
}));

vi.mock('@/hooks/use-auth', () => ({
  useAuth: () => ({
    user: {
      id: 1,
      username: 'admin',
      email: 'admin@example.com',
      is_admin: true,
      trust_level: 2,
      language: 'zh',
      timezone: '',
    },
    isLoading: false,
    logout: vi.fn(),
    checkAuthStatus: vi.fn(),
  }),
}));

vi.mock('@/hooks/use-theme-utils', () => ({
  useThemeUtils: () => ({
    toggle: vi.fn(),
    getIcon: () => null,
    getAction: () => '切换主题',
  }),
}));

vi.mock('@/lib/i18n', () => ({
  useLocale: () => ({locale: 'zh', syncLocale: vi.fn()}),
  locales: ['zh', 'en'],
  localeNames: {zh: '中文', en: 'English'},
}));

vi.mock('@/lib/services', () => ({
  default: {
    capability: {
      getVersion: vi.fn().mockResolvedValue({
        version: '1.0.0',
        build_time: 'unknown',
        git_commit: 'abc1234',
      }),
    },
    auth: {
      updateProfile: vi.fn(),
    },
  },
}));

vi.mock('@/lib/timezone', () => ({
  FOLLOW_BROWSER_TIMEZONE: '',
  getBrowserTimezone: () => 'Asia/Shanghai',
  isValidTimezone: () => true,
  listTimezones: () => ['Asia/Shanghai', 'UTC'],
  resolveDisplayTimezone: () => 'Asia/Shanghai',
  setUserTimezonePreference: vi.fn(),
}));

describe('ProfileButton', () => {
  beforeEach(() => {
    push.mockReset();
    vi.useFakeTimers({shouldAdvanceTime: true});
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('opens profile dialog without navigating to /system-upgrade', async () => {
    render(<ProfileButton />);

    fireEvent.click(screen.getByTestId('profile-dock-trigger'));

    expect(await screen.findByTestId('profile-dialog')).toBeTruthy();
    expect(push).not.toHaveBeenCalled();
    expect(screen.getByTestId('profile-check-update')).toBeTruthy();
  });

  it('navigates to /system-upgrade only when 检查更新 is clicked after arming', async () => {
    render(<ProfileButton />);

    fireEvent.click(screen.getByTestId('profile-dock-trigger'));
    expect(await screen.findByTestId('profile-dialog')).toBeTruthy();

    // Before arming window elapses, click must not navigate (click-through guard).
    fireEvent.click(screen.getByTestId('profile-check-update'));
    expect(push).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(400);
    });

    fireEvent.click(screen.getByTestId('profile-check-update'));
    await waitFor(() => {
      expect(push).toHaveBeenCalledWith('/system-upgrade');
    });
  });
});
