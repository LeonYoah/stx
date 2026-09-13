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

import {useState, useEffect, FormEvent} from 'react';
import {useSearchParams, useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {
  Accordion,
  AccordionItem,
  AccordionTrigger,
  AccordionContent,
} from '@/components/animate-ui/radix/accordion';
import {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from '@/components/animate-ui/radix/dialog';
import {LoaderCircle, Github} from 'lucide-react';
import {useAuth} from '@/hooks/use-auth';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import {STXLogo, STXLogoDark} from '@/components/icons/logo';

/**
 * 登录表单组件属性
 */
export type LoginFormProps = React.ComponentProps<'div'>;

/**
 * Google 图标组件
 */
function GoogleIcon({className}: {className?: string}) {
  return (
    <svg
      className={className}
      viewBox='0 0 24 24'
      width='20'
      height='20'
      xmlns='http://www.w3.org/2000/svg'
    >
      <path
        d='M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z'
        fill='#4285F4'
      />
      <path
        d='M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z'
        fill='#34A853'
      />
      <path
        d='M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z'
        fill='#FBBC05'
      />
      <path
        d='M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z'
        fill='#EA4335'
      />
    </svg>
  );
}

/**
 * 登录表单组件
 * 支持用户名密码登录（默认）和 OAuth 登录（备选）
 */
export function LoginForm({className, ...props}: LoginFormProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [isButtonLoading, setIsButtonLoading] = useState(false);
  const [enabledOAuthProviders, setEnabledOAuthProviders] = useState<string[]>(
    [],
  );
  const [logoutMessage, setLogoutMessage] = useState('');
  const [validationError, setValidationError] = useState('');
  const {
    loginWithCredentials,
    loginWithOAuth,
    error,
    clearError,
    user,
    isAuthenticated,
  } = useAuth();
  const searchParams = useSearchParams();
  const router = useRouter();
  const t = useTranslations();

  useEffect(() => {
    const isLoggedOut = searchParams.get('logout') === 'true';
    if (isLoggedOut) {
      setLogoutMessage(t('auth.logout.success'));
      // 使用 pushState 去除 logout 参数
      const url = new URL(window.location.href);
      url.searchParams.delete('logout');
      window.history.pushState({}, '', url.toString());
    } else {
      setLogoutMessage('');
    }
  }, [searchParams, t]);

  useEffect(() => {
    let mounted = true;

    const loadEnabledProviders = async () => {
      try {
        const providers = await services.auth.getEnabledOAuthProviders();
        if (mounted) {
          setEnabledOAuthProviders(
            Array.isArray(providers)
              ? providers.map((provider) => provider.toLowerCase())
              : [],
          );
        }
      } catch {
        if (mounted) {
          setEnabledOAuthProviders([]);
        }
      }
    };

    void loadEnabledProviders();

    return () => {
      mounted = false;
    };
  }, []);

  useEffect(() => {
    if (isAuthenticated && user && !searchParams.get('logout') && !error) {
      router.push('/dashboard');
    }
  }, [isAuthenticated, user, router, searchParams, error]);

  const showGitHubLogin = enabledOAuthProviders.includes('github');
  const showGoogleLogin = enabledOAuthProviders.includes('google');
  const hasEnabledOAuthProviders = showGitHubLogin || showGoogleLogin;

  /**
   * 验证表单输入
   */
  const validateForm = (): boolean => {
    if (!username.trim()) {
      setValidationError(t('auth.errors.emptyUsername'));
      return false;
    }
    if (!password) {
      setValidationError(t('auth.errors.emptyPassword'));
      return false;
    }
    setValidationError('');
    return true;
  };

  /**
   * 处理用户名密码登录
   */
  const handleCredentialsLogin = async (e: FormEvent) => {
    e.preventDefault();

    if (!validateForm()) {
      return;
    }

    clearError();
    setLogoutMessage('');
    setIsButtonLoading(true);

    try {
      const redirectPath = searchParams.get('redirect');
      const validRedirectPath =
        redirectPath && redirectPath !== '/' && redirectPath !== '/login'
          ? redirectPath
          : '/dashboard';

      await loginWithCredentials(username, password, validRedirectPath);
    } catch {
      // 错误已在 hook 中处理
    } finally {
      setIsButtonLoading(false);
    }
  };

  /**
   * 处理 OAuth 登录
   */
  const handleOAuthLogin = async (provider: string) => {
    clearError();
    setLogoutMessage('');
    setValidationError('');

    try {
      const redirectPath = searchParams.get('redirect');
      const validRedirectPath =
        redirectPath && redirectPath !== '/' && redirectPath !== '/login'
          ? redirectPath
          : '/dashboard';

      await loginWithOAuth(provider, validRedirectPath);
    } catch {
      // 错误已在 hook 中处理
    }
  };

  return (
    <div className='fixed inset-0 flex items-center justify-center w-full h-screen overflow-hidden'>
      <div
        className={cn(
          'flex flex-col gap-6 w-full max-w-md px-6 py-8 rounded-2xl max-h-screen overflow-y-auto',
          className,
        )}
        {...props}
      >
        <form onSubmit={handleCredentialsLogin}>
          <div className='flex flex-col gap-6 transition-all duration-500 ease-in-out'>
            <div className='flex flex-col items-center gap-2'>
              <STXLogo
                className='m-2 h-auto w-48 dark:hidden'
                priority
              />
              <STXLogoDark
                className='m-2 hidden h-auto w-48 dark:block'
                priority
              />
              <h1 className='text-xl font-bold text-center'>
                {t('auth.login.title')}
              </h1>
              <p className='text-muted-foreground text-sm'>
                {t('auth.login.subtitle')}
              </p>
            </div>

            {/* 登出成功提示 */}
            {logoutMessage && (
              <div className='text-success text-sm mt-2 rounded-md text-center'>
                {logoutMessage}
              </div>
            )}

            {/* 验证错误信息显示 */}
            {validationError && (
              <div className='text-destructive text-sm mt-2 rounded-md text-center'>
                {validationError}
              </div>
            )}

            {/* API 错误信息显示 */}
            {error && !validationError && (
              <div className='text-destructive text-sm mt-2 rounded-md text-center'>
                {error}
              </div>
            )}

            {/* 用户名输入框 */}
            <div className='space-y-2'>
              <Label htmlFor='username'>{t('auth.login.username')}</Label>
              <Input
                id='username'
                type='text'
                placeholder={t('auth.login.usernamePlaceholder')}
                value={username}
                onChange={(e) => {
                  setUsername(e.target.value);
                  setValidationError('');
                }}
                disabled={isButtonLoading}
                autoComplete='username'
              />
            </div>

            {/* 密码输入框 */}
            <div className='space-y-2'>
              <Label htmlFor='password'>{t('auth.login.password')}</Label>
              <Input
                id='password'
                type='password'
                placeholder={t('auth.login.passwordPlaceholder')}
                value={password}
                onChange={(e) => {
                  setPassword(e.target.value);
                  setValidationError('');
                }}
                disabled={isButtonLoading}
                autoComplete='current-password'
              />
            </div>

            {/* 登录按钮 */}
            <Button type='submit' className='w-full' disabled={isButtonLoading}>
              {isButtonLoading ? (
                <>
                  <LoaderCircle className='h-4 w-4 animate-spin' />
                  {t('auth.login.loggingIn')}
                </>
              ) : (
                t('auth.login.loginButton')
              )}
            </Button>

            {hasEnabledOAuthProviders ? (
              <>
                {/* OAuth 登录分隔线 */}
                <div className='relative'>
                  <div className='absolute inset-0 flex items-center'>
                    <span className='w-full border-t' />
                  </div>
                  <div className='relative flex justify-center text-xs uppercase'>
                    <span className='bg-background px-2 text-muted-foreground'>
                      {t('auth.login.orLoginWith')}
                    </span>
                  </div>
                </div>

                {/* OAuth 登录按钮 */}
                <div
                  className={cn(
                    'grid gap-4',
                    showGitHubLogin && showGoogleLogin
                      ? 'grid-cols-2'
                      : 'grid-cols-1',
                  )}
                >
                  {showGitHubLogin ? (
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() => handleOAuthLogin('github')}
                      disabled={isButtonLoading}
                    >
                      <Github className='h-4 w-4' />
                      GitHub
                    </Button>
                  ) : null}
                  {showGoogleLogin ? (
                    <Button
                      type='button'
                      variant='outline'
                      onClick={() => handleOAuthLogin('google')}
                      disabled={isButtonLoading}
                    >
                      <GoogleIcon className='h-4 w-4' />
                      Google
                    </Button>
                  ) : null}
                </div>
              </>
            ) : null}
          </div>
        </form>

        <div className='text-muted-foreground text-center text-xs text-balance mt-2'>
          <span className='[&_button]:underline [&_button]:underline-offset-4 [&_button:hover]:text-primary'>
            {t('terms.agreement')}{' '}
            <Dialog>
              <DialogTrigger asChild>
                <button className='text-inherit bg-transparent border-none p-0 cursor-pointer'>
                  {t('terms.termsOfService')}
                </button>
              </DialogTrigger>
              <DialogContent className='max-w-3xl max-h-[80vh] overflow-y-auto'>
                <DialogHeader>
                  <DialogTitle>{t('terms.termsDialog.title')}</DialogTitle>
                  <DialogDescription>
                    {t('terms.termsDialog.description')}
                  </DialogDescription>
                </DialogHeader>
                <div className='mt-4'>
                  <Accordion type='single' collapsible className='w-full'>
                    <AccordionItem value='general'>
                      <AccordionTrigger>
                        {t('terms.termsDialog.general.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.termsDialog.general.content1')}</p>
                          <p>{t('terms.termsDialog.general.content2')}</p>
                          <p>{t('terms.termsDialog.general.content3')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='usage'>
                      <AccordionTrigger>
                        {t('terms.termsDialog.usage.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.termsDialog.usage.content1')}</p>
                          <p>{t('terms.termsDialog.usage.content2')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.termsDialog.usage.prohibited1')}</li>
                            <li>{t('terms.termsDialog.usage.prohibited2')}</li>
                            <li>{t('terms.termsDialog.usage.prohibited3')}</li>
                            <li>{t('terms.termsDialog.usage.prohibited4')}</li>
                          </ul>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='content'>
                      <AccordionTrigger>
                        {t('terms.termsDialog.content.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.termsDialog.content.intro')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>
                              <strong>
                                {t('terms.termsDialog.content.pornography')}
                              </strong>
                            </li>
                            <li>
                              <strong>
                                {t('terms.termsDialog.content.promotion')}
                              </strong>
                            </li>
                            <li>
                              <strong>
                                {t('terms.termsDialog.content.illegal')}
                              </strong>
                            </li>
                            <li>
                              <strong>
                                {t('terms.termsDialog.content.harmful')}
                              </strong>
                            </li>
                            <li>
                              <strong>
                                {t('terms.termsDialog.content.false')}
                              </strong>
                            </li>
                            <li>
                              <strong>
                                {t('terms.termsDialog.content.infringement')}
                              </strong>
                            </li>
                          </ul>
                          <p>{t('terms.termsDialog.content.warning')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='legal'>
                      <AccordionTrigger>
                        {t('terms.termsDialog.legal.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.termsDialog.legal.intro')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.termsDialog.legal.law1')}</li>
                            <li>{t('terms.termsDialog.legal.law2')}</li>
                            <li>{t('terms.termsDialog.legal.law3')}</li>
                            <li>{t('terms.termsDialog.legal.law4')}</li>
                            <li>{t('terms.termsDialog.legal.law5')}</li>
                          </ul>
                          <p>{t('terms.termsDialog.legal.compliance')}</p>
                          <p>{t('terms.termsDialog.legal.cooperation')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='account'>
                      <AccordionTrigger>
                        {t('terms.termsDialog.account.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.termsDialog.account.content1')}</p>
                          <p>{t('terms.termsDialog.account.content2')}</p>
                          <p>{t('terms.termsDialog.account.content3')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='intellectual'>
                      <AccordionTrigger>
                        {t('terms.termsDialog.intellectual.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.termsDialog.intellectual.content1')}</p>
                          <p>{t('terms.termsDialog.intellectual.content2')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='limitation'>
                      <AccordionTrigger>
                        {t('terms.termsDialog.limitation.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.termsDialog.limitation.content1')}</p>
                          <p>{t('terms.termsDialog.limitation.content2')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>
                  </Accordion>
                </div>
              </DialogContent>
            </Dialog>{' '}
            {t('terms.and')}{' '}
            <Dialog>
              <DialogTrigger asChild>
                <button className='text-inherit bg-transparent border-none p-0 cursor-pointer'>
                  {t('terms.privacyPolicy')}
                </button>
              </DialogTrigger>
              <DialogContent className='max-w-3xl max-h-[80vh] overflow-y-auto'>
                <DialogHeader>
                  <DialogTitle>{t('terms.privacyDialog.title')}</DialogTitle>
                  <DialogDescription>
                    {t('terms.privacyDialog.description')}
                  </DialogDescription>
                </DialogHeader>
                <div className='mt-4'>
                  <Accordion type='single' collapsible className='w-full'>
                    <AccordionItem value='collection'>
                      <AccordionTrigger>
                        {t('terms.privacyDialog.collection.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.privacyDialog.collection.intro')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.privacyDialog.collection.item1')}</li>
                            <li>{t('terms.privacyDialog.collection.item2')}</li>
                            <li>{t('terms.privacyDialog.collection.item3')}</li>
                            <li>{t('terms.privacyDialog.collection.item4')}</li>
                          </ul>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='usage-info'>
                      <AccordionTrigger>
                        {t('terms.privacyDialog.usage.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.privacyDialog.usage.intro')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.privacyDialog.usage.item1')}</li>
                            <li>{t('terms.privacyDialog.usage.item2')}</li>
                            <li>{t('terms.privacyDialog.usage.item3')}</li>
                            <li>{t('terms.privacyDialog.usage.item4')}</li>
                            <li>{t('terms.privacyDialog.usage.item5')}</li>
                          </ul>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='sharing'>
                      <AccordionTrigger>
                        {t('terms.privacyDialog.sharing.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.privacyDialog.sharing.content1')}</p>
                          <p>{t('terms.privacyDialog.sharing.content2')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.privacyDialog.sharing.item1')}</li>
                            <li>{t('terms.privacyDialog.sharing.item2')}</li>
                            <li>{t('terms.privacyDialog.sharing.item3')}</li>
                            <li>{t('terms.privacyDialog.sharing.item4')}</li>
                          </ul>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='security'>
                      <AccordionTrigger>
                        {t('terms.privacyDialog.security.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.privacyDialog.security.intro')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.privacyDialog.security.item1')}</li>
                            <li>{t('terms.privacyDialog.security.item2')}</li>
                            <li>{t('terms.privacyDialog.security.item3')}</li>
                            <li>{t('terms.privacyDialog.security.item4')}</li>
                          </ul>
                          <p>{t('terms.privacyDialog.security.warning')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='retention'>
                      <AccordionTrigger>
                        {t('terms.privacyDialog.retention.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.privacyDialog.retention.intro')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.privacyDialog.retention.item1')}</li>
                            <li>{t('terms.privacyDialog.retention.item2')}</li>
                            <li>{t('terms.privacyDialog.retention.item3')}</li>
                          </ul>
                          <p>{t('terms.privacyDialog.retention.deletion')}</p>
                        </div>
                      </AccordionContent>
                    </AccordionItem>

                    <AccordionItem value='rights'>
                      <AccordionTrigger>
                        {t('terms.privacyDialog.rights.title')}
                      </AccordionTrigger>
                      <AccordionContent>
                        <div className='space-y-3 text-sm'>
                          <p>{t('terms.privacyDialog.rights.intro')}</p>
                          <ul className='list-disc pl-6 space-y-1'>
                            <li>{t('terms.privacyDialog.rights.item1')}</li>
                            <li>{t('terms.privacyDialog.rights.item2')}</li>
                            <li>{t('terms.privacyDialog.rights.item3')}</li>
                            <li>{t('terms.privacyDialog.rights.item4')}</li>
                          </ul>
                        </div>
                      </AccordionContent>
                    </AccordionItem>
                  </Accordion>
                </div>
              </DialogContent>
            </Dialog>
          </span>
        </div>
      </div>
    </div>
  );
}
