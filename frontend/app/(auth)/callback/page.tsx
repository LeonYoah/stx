/**
 * Copyright 2024 Apache Software Foundation
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import {Suspense} from 'react';
import {CallbackHandler} from '@/components/common/auth/CallbackHandler';
import {Metadata} from 'next';

export const metadata: Metadata = {
  title: 'OAuth 登录处理 | STX',
};

/**
 * OAuth回调页面
 * 处理OAuth登录回调流程
 */
export default function AuthCallbackPage() {
  return (
    <div className='bg-background flex min-h-svh flex-col items-center justify-center gap-6 p-6 md:p-10'>
      <div className='w-full max-w-sm'>
        <Suspense>
          <CallbackHandler />
        </Suspense>
      </div>
    </div>
  );
}
