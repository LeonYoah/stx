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
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.
 */

import {Suspense} from 'react';
import {useSearchParams} from 'next/navigation';
import {UserManagement} from '@/components/common/admin/UserManagement';
import {TokenManagement} from '@/components/common/admin/TokenManagement';

function AccountsPageInner() {
  const tab = useSearchParams().get('tab');
  return tab === 'tokens' ? <TokenManagement /> : <UserManagement />;
}

export default function UsersPage() {
  return (
    <Suspense>
      <AccountsPageInner />
    </Suspense>
  );
}
