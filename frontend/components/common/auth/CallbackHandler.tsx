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

import {useEffect, useState} from 'react';
import {useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {LiquidButton} from '@/components/animate-ui/buttons/liquid';
import {CheckCircle2, AlertCircle} from 'lucide-react';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import {STXMark} from '@/components/icons/logo';

/**
 * 回调处理组件属性
 */
export type CallbackHandlerProps = React.ComponentProps<'div'>;

/**
 * OAuth回调处理组件
 * 处理OAuth登录回调，显示验证状态
 */
export function CallbackHandler({className, ...props}: CallbackHandlerProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const t = useTranslations('auth.callback');
  const [status, setStatus] = useState<'loading' | 'success' | 'error'>(
    'loading',
  );
  const [error, setError] = useState<string>('');

  useEffect(() => {
    /**
     * 处理OAuth回调
     */
    const handleCallback = async () => {
      try {
        const state = searchParams.get('state');
        const code = searchParams.get('code');
        const errorParam = searchParams.get('error');

        if (errorParam) {
          throw new Error(`${t('oauthFailed')}: ${errorParam}`);
        }

        if (!state || !code) {
          throw new Error(t('missingParams'));
        }

        await services.auth.handleCallback({state, code});

        const redirectTo = sessionStorage.getItem('oauth_redirect_to');
        const targetPath = redirectTo || '/dashboard';

        setStatus('success');

        sessionStorage.removeItem('oauth_redirect_to');

        setTimeout(() => {
          window.location.href = targetPath;
        }, 1000);
      } catch (err) {
        console.error('回调处理错误:', err);

        let errorMessage = t('processingFailed');

        if (err instanceof Error) {
          if (err.message.includes('Invalid state')) {
            errorMessage = t('sessionExpired') || '登录会话已过期，请重新登录';
          } else {
            errorMessage = err.message;
          }
        }

        setError(errorMessage);
        setStatus('error');
      }
    };

    handleCallback();
  }, [searchParams, t]);

  return (
    <div className='fixed inset-0 flex items-center justify-center w-full h-screen overflow-hidden'>
      <div
        className={cn(
          'flex flex-col gap-6 w-full max-w-md px-6 py-8 rounded-2xl max-h-screen overflow-y-auto',
          className,
        )}
        {...props}
      >
        <div className='flex flex-col gap-6'>
          <div className='flex flex-col items-center gap-2'>
            <STXMark
              className='size-12 rounded-xl ring-1 ring-black/5 dark:ring-white/10'
              priority
            />
            <h1 className='text-xl font-bold'>{t('title')}</h1>
          </div>
          <div className='flex flex-col items-center gap-4'>
            {status === 'loading' && (
              <div className='flex flex-col items-center gap-4 w-full max-w-sm'>
                <div className='w-8 h-8 border-3 border-primary border-t-transparent rounded-full animate-spin' />
                <div className='text-center space-y-2'>
                  <h2 className='text-lg font-medium'>{t('verifying')}</h2>
                </div>
              </div>
            )}
            {status === 'success' && (
              <div className='flex flex-col items-center gap-3'>
                <CheckCircle2 className='h-8 w-8 text-green-500' />
                <h2 className='text-lg font-medium text-green-500'>
                  {t('success')}
                </h2>
              </div>
            )}
            {status === 'error' && (
              <div className='flex flex-col items-center gap-4 w-full max-w-sm'>
                <AlertCircle className='h-8 w-8 text-destructive' />
                <div className='text-center space-y-3 w-full'>
                  <h2 className='text-md font-medium text-destructive'>
                    {t('failed')} ｜{error}
                  </h2>
                  <LiquidButton
                    className='w-full mt-4'
                    onClick={() => router.push('/login')}
                  >
                    {t('retryLogin')}
                  </LiquidButton>
                </div>
              </div>
            )}
          </div>
        </div>
        <div className='text-muted-foreground text-center text-xs text-balance'>
          <span className='[&_a]:underline [&_a]:underline-offset-4 [&_a:hover]:text-primary'>
            {t('footer')}
          </span>
        </div>
      </div>
    </div>
  );
}
