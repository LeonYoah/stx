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

import {Suspense} from 'react';
import {LoginForm} from '@/components/common/auth/LoginForm';
import {Metadata} from 'next';
import '@/components/common/auth/login-auth.css';

export const metadata: Metadata = {
  title: '登录',
};

/**
 * 登录页：全屏品牌壳由 LoginForm 自行铺开。
 * Login page: LoginForm owns the full-bleed brand shell.
 */
export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}
