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

import {useTranslations} from 'next-intl';
import {KeyRound, Users} from 'lucide-react';
import {ModuleNavTabs} from '@/components/common/layout';

/**
 * 用户管理与令牌管理共用的双 Tab。
 * Shared dual tabs for user management and token management.
 */
export function AccountsNavTabs({
  activeKey,
}: {
  activeKey: 'users' | 'tokens';
}) {
  const t = useTranslations();
  return (
    <ModuleNavTabs
      items={[
        {
          key: 'users',
          label: t('admin.userManagement.title'),
          href: '/admin/users',
          icon: <Users className='size-3.5' />,
        },
        {
          key: 'tokens',
          label: t('admin.tokenManagement.title'),
          href: '/admin/users?tab=tokens',
          icon: <KeyRound className='size-3.5' />,
        },
      ]}
      activeKey={activeKey}
    />
  );
}
