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

// 用户状态开关：当前用户不可停用自己；停用他人需确认；启用立即生效（2026-10-23 唯一管理员误点停用自己后被锁在门外）。
// User status switch: the current user cannot deactivate themselves; deactivating others needs confirmation; enabling
// applies at once (on 2026-10-23 the sole admin mis-clicked and locked themselves out).

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {UserManagement} from '../UserManagement';

const listUsers = vi.fn();
const updateUser = vi.fn();

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: vi.fn()}),
  usePathname: () => '/admin/users',
}));

vi.mock('@/hooks/use-auth', () => ({
  useAuth: () => ({user: {id: 1, username: 'admin', is_admin: true}}),
}));

vi.mock('@/lib/services', () => ({
  default: {
    adminUser: {
      listUsers: (...args: unknown[]) => listUsers(...args),
      updateUser: (...args: unknown[]) => updateUser(...args),
    },
  },
}));

vi.mock('@gsap/react', () => ({useGSAP: () => undefined}));
vi.mock('@/lib/animations/gsap-motion', () => ({animateTableRows: vi.fn()}));
vi.mock('sonner', () => ({toast: {success: vi.fn(), error: vi.fn()}}));

const baseUser = {
  nickname: '',
  email: '',
  is_admin: false,
  created_at: '2026-10-21T10:11:41Z',
  updated_at: '2026-10-21T10:11:41Z',
};

function renderPage() {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <UserManagement />
    </NextIntlClientProvider>,
  );
}

describe('UserManagement status switch', () => {
  beforeEach(() => {
    listUsers.mockReset();
    updateUser.mockReset();
    updateUser.mockResolvedValue({});
    listUsers.mockResolvedValue({
      users: [
        {...baseUser, id: 1, username: 'admin', is_admin: true, is_active: true},
        {...baseUser, id: 2, username: 'bob', is_active: true},
        {...baseUser, id: 3, username: 'carol', is_active: false},
      ],
      total: 3,
    });
  });

  it('当前登录用户的开关置灰，不能停用自己', async () => {
    renderPage();
    const self = await screen.findByRole('switch', {name: '启用 admin'});
    expect(self).toBeDisabled();
    expect(screen.getByRole('switch', {name: '启用 bob'})).toBeEnabled();
    fireEvent.click(self);
    expect(updateUser).not.toHaveBeenCalled();
  });

  it('停用他人先确认，确认后才提交', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('switch', {name: '启用 bob'}));
    expect(updateUser).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/确定要停用用户 bob 吗/),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', {name: '停用'}));
    await waitFor(() =>
      expect(updateUser).toHaveBeenCalledWith(2, {is_active: false}),
    );
  });

  it('取消确认不提交', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('switch', {name: '启用 bob'}));
    fireEvent.click(await screen.findByRole('button', {name: '取消'}));
    await waitFor(() =>
      expect(screen.queryByText(/确定要停用用户 bob 吗/)).not.toBeInTheDocument(),
    );
    expect(updateUser).not.toHaveBeenCalled();
  });

  it('启用他人立即提交', async () => {
    renderPage();
    fireEvent.click(await screen.findByRole('switch', {name: '启用 carol'}));
    await waitFor(() =>
      expect(updateUser).toHaveBeenCalledWith(3, {is_active: true}),
    );
  });
});
