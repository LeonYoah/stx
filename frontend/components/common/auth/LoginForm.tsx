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

import {useState, useEffect, FormEvent, type ReactNode} from 'react';
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
import {LoaderCircle, Github, Lock, User} from 'lucide-react';
import {useAuth} from '@/hooks/use-auth';
import {useThemeUtils} from '@/hooks/use-theme-utils';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import {LoginBrandPanel} from '@/components/common/auth/LoginBrandPanel';

/**
 * 登录表单组件属性
 * Login form component props
 */
export type LoginFormProps = React.ComponentProps<'div'>;

/**
 * Google 图标组件
 * Google mark icon
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
 * 带左侧图标的表单字段壳层。
 * Form field shell with a leading icon.
 */
function AuthField({
  label,
  htmlFor,
  icon,
  children,
}: {
  label: string;
  htmlFor: string;
  icon: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className='stx-field'>
      <Label htmlFor={htmlFor} className='stx-field-label'>
        {label}
      </Label>
      <div className='stx-control'>
        <span className='stx-control-icon' aria-hidden='true'>
          {icon}
        </span>
        {children}
      </div>
    </div>
  );
}

/**
 * 登录页服务条款 / 隐私政策链接与弹窗。
 * Terms of service and privacy policy links with dialogs.
 */
function AuthLegalLinks() {
  const t = useTranslations();

  return (
    <div className='stx-terms'>
      <span>
        {t('terms.agreement')}{' '}
        <Dialog>
          <DialogTrigger asChild>
            <button type='button'>{t('terms.termsOfService')}</button>
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
            <button type='button'>{t('terms.privacyPolicy')}</button>
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
  );
}

/**
 * 登录表单组件：左右分栏品牌壳 + 凭证 / OAuth 登录。
 * Login form: split brand shell with credentials and OAuth login.
 */
export function LoginForm({className, ...props}: LoginFormProps) {
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
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
  const themeUtils = useThemeUtils();
  const [themeMounted, setThemeMounted] = useState(false);

  useEffect(() => {
    setThemeMounted(true);
  }, []);

  useEffect(() => {
    const isLoggedOut = searchParams.get('logout') === 'true';
    if (isLoggedOut) {
      setLogoutMessage(t('auth.logout.success'));
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
   * Validate credential fields
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
   * Handle username/password login
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
      // 错误已在 hook 中处理 / Error already handled in the auth hook
    } finally {
      setIsButtonLoading(false);
    }
  };

  /**
   * 处理 OAuth 登录
   * Handle OAuth login
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
      // 错误已在 hook 中处理 / Error already handled in the auth hook
    }
  };

  const alertMessage = validationError || error || logoutMessage;
  const alertClass = logoutMessage
    ? 'stx-alert stx-alert-success'
    : alertMessage
      ? 'stx-alert stx-alert-error'
      : 'stx-alert';

  return (
    <div className={cn('stx-auth-body', className)} {...props}>
      <div className='stx-auth-bg' aria-hidden='true'>
        <div className='stx-auth-bg__grid' />
        <div className='stx-auth-bg__noise' />
      </div>

      <main className='stx-auth-shell'>
        <div className='stx-auth-card'>
          <LoginBrandPanel />

          <section className='stx-form-panel'>
            <div className='stx-form-shell'>
              <div className='stx-panel-inner'>
                <div className='stx-panel-toolbar'>
                  <button
                    type='button'
                    className='stx-theme-toggle'
                    onClick={themeUtils.toggle}
                    aria-label={
                      themeMounted
                        ? themeUtils.getAction()
                        : t('auth.login.toggleTheme')
                    }
                    title={
                      themeMounted
                        ? themeUtils.getAction()
                        : t('auth.login.toggleTheme')
                    }
                  >
                    {themeMounted
                      ? themeUtils.getIcon('h-4 w-4')
                      : null}
                  </button>
                </div>

                <div className='stx-auth-header'>
                  <h2>{t('auth.login.title')}</h2>
                </div>

                <div className={alertClass} role='status'>
                  {alertMessage || '\u00A0'}
                </div>

                <form
                  className='stx-form-grid'
                  onSubmit={handleCredentialsLogin}
                >
                  <AuthField
                    label={t('auth.login.username')}
                    htmlFor='username'
                    icon={<User className='h-4 w-4' />}
                  >
                    <Input
                      id='username'
                      className='stx-auth-input'
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
                  </AuthField>

                  <AuthField
                    label={t('auth.login.password')}
                    htmlFor='password'
                    icon={<Lock className='h-4 w-4' />}
                  >
                    <Input
                      id='password'
                      className='stx-auth-input stx-auth-input-with-toggle'
                      type={showPassword ? 'text' : 'password'}
                      placeholder={t('auth.login.passwordPlaceholder')}
                      value={password}
                      onChange={(e) => {
                        setPassword(e.target.value);
                        setValidationError('');
                      }}
                      disabled={isButtonLoading}
                      autoComplete='current-password'
                    />
                    <button
                      type='button'
                      className='stx-toggle-pass'
                      onClick={() => setShowPassword((value) => !value)}
                      aria-label={
                        showPassword
                          ? t('auth.login.hidePassword')
                          : t('auth.login.showPassword')
                      }
                      disabled={isButtonLoading}
                    >
                      {showPassword
                        ? t('auth.login.hidePasswordShort')
                        : t('auth.login.showPasswordShort')}
                    </button>
                  </AuthField>

                  <Button
                    type='submit'
                    className='stx-btn-primary'
                    disabled={isButtonLoading}
                  >
                    {isButtonLoading ? (
                      <>
                        <LoaderCircle className='h-4 w-4 animate-spin' />
                        {t('auth.login.loggingIn')}
                      </>
                    ) : (
                      <>
                        <span>{t('auth.login.loginButton')}</span>
                        <span aria-hidden='true'>→</span>
                      </>
                    )}
                  </Button>
                </form>

                {hasEnabledOAuthProviders ? (
                  <div className='stx-social-login'>
                    <div className='stx-divider'>
                      <span>{t('auth.login.orLoginWith')}</span>
                    </div>
                    <div
                      className={cn(
                        'stx-oauth-grid',
                        showGitHubLogin && showGoogleLogin && 'is-split',
                      )}
                    >
                      {showGitHubLogin ? (
                        <Button
                          type='button'
                          variant='outline'
                          className='stx-oauth-btn'
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
                          className='stx-oauth-btn'
                          onClick={() => handleOAuthLogin('google')}
                          disabled={isButtonLoading}
                        >
                          <GoogleIcon className='h-4 w-4' />
                          Google
                        </Button>
                      ) : null}
                    </div>
                  </div>
                ) : null}

                <AuthLegalLinks />
              </div>

              <div className='stx-auth-footer'>
                <span>STX</span>
                <span>© {new Date().getFullYear()}</span>
              </div>
            </div>
          </section>
        </div>
      </main>
    </div>
  );
}
