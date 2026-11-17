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

import {useEffect, useState} from 'react';
import services from '@/lib/services';
import type {NotificationRecipientUser} from '@/lib/services/monitoring';

/**
 * 发起人列用的用户列表（services.monitoring.listNotifiableUsers）；失败时静默返回空数组，
 * 发起人列回退显示 #uid。
 * User list behind the initiator column (services.monitoring.listNotifiableUsers); failures fall back to an
 * empty array so the column degrades to "#uid".
 */
export function useNotifiableUsers(): NotificationRecipientUser[] {
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  useEffect(() => {
    let disposed = false;
    void Promise.resolve()
      .then(() => services.monitoring.listNotifiableUsers())
      .then((data) => {
        if (!disposed && Array.isArray(data?.users)) {
          setUsers(data.users);
        }
      })
      .catch(() => {});
    return () => {
      disposed = true;
    };
  }, []);
  return users;
}
