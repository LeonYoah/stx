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
 * 新建用户弹窗：组合 Arc Dialog / Input / PasswordStrength / RadioCards / Alert / Button，
 * 单层扁平分组（留白 + 0.5px 细线）、字段级行内校验与实时密码强度反馈。
 * Create user dialog: composed from Arc Dialog / Input / PasswordStrength / RadioCards /
 * Alert / Button with single-layer flat sections (whitespace + 0.5px hairlines), inline
 * field validation, and live password strength feedback.
 */

import {useMemo, useState, type FormEvent} from 'react';
import {useTranslations} from 'next-intl';
import {ShieldCheck, User} from 'lucide-react';
import {Alert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import {
  PasswordStrength,
  createPasswordRules,
} from '@/components/arc/password-strength/password-strength';
import {RadioCards} from '@/components/arc/radio-cards/radio-cards';
import layout from '@/components/common/layout/list-detail.module.css';
import services from '@/lib/services';
import {toast} from '@/lib/toast';
import {cn} from '@/lib/utils';

function isValidEmail(value: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

interface FieldErrors {
  username?: string;
  password?: string;
  email?: string;
}

export interface CreateUserDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onCreated: () => void;
}

export function CreateUserDialog({
  open,
  onOpenChange,
  onCreated,
}: CreateUserDialogProps) {
  const t = useTranslations();

  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [nickname, setNickname] = useState('');
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<'user' | 'admin'>('user');
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({});
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const passwordRules = useMemo(
    () =>
      createPasswordRules({
        minLength: 6,
        labels: {
          length: (min) =>
            t('admin.userManagement.passwordStrength.ruleLength', {min}),
          case: t('admin.userManagement.passwordStrength.ruleCase'),
          number: t('admin.userManagement.passwordStrength.ruleNumber'),
          symbol: t('admin.userManagement.passwordStrength.ruleSymbol'),
        },
      }),
    [t],
  );

  const passwordMessages = useMemo(
    () => ({
      showPassword: t('admin.userManagement.passwordStrength.showPassword'),
      meterLabel: t('admin.userManagement.passwordStrength.meterLabel'),
      noPassword: t('admin.userManagement.passwordStrength.noPassword'),
      rulesLabel: t('admin.userManagement.passwordStrength.rulesLabel'),
      remainingPrefix: t(
        'admin.userManagement.passwordStrength.remainingPrefix',
      ),
      remainingSuffix: t(
        'admin.userManagement.passwordStrength.remainingSuffix',
      ),
      levels: {
        tooShort: t('admin.userManagement.passwordStrength.tooShort'),
        weak: t('admin.userManagement.passwordStrength.weak'),
        fair: t('admin.userManagement.passwordStrength.fair'),
        good: t('admin.userManagement.passwordStrength.good'),
        strong: t('admin.userManagement.passwordStrength.strong'),
      },
    }),
    [t],
  );

  const resetForm = () => {
    setUsername('');
    setPassword('');
    setNickname('');
    setEmail('');
    setRole('user');
    setFieldErrors({});
    setSubmitError(null);
  };

  const handleOpenChange = (next: boolean) => {
    if (!next && !submitting) {
      resetForm();
    }
    onOpenChange(next);
  };

  const validate = (): FieldErrors => {
    const errs: FieldErrors = {};
    const trimmedUser = username.trim();
    if (!trimmedUser) {
      errs.username = t('admin.userManagement.errors.usernameRequired');
    } else if (trimmedUser.length < 3) {
      errs.username = t('admin.userManagement.errors.usernameTooShort');
    }
    if (!password) {
      errs.password = t('admin.userManagement.errors.passwordRequired');
    } else if (password.length < 6) {
      errs.password = t('admin.userManagement.errors.passwordTooShort');
    }
    const trimmedEmail = email.trim();
    if (trimmedEmail && !isValidEmail(trimmedEmail)) {
      errs.email = t('admin.userManagement.errors.invalidEmail');
    }
    return errs;
  };

  const handleSubmit = async (event: FormEvent) => {
    event.preventDefault();
    if (submitting) {
      return;
    }
    const errs = validate();
    setFieldErrors(errs);
    setSubmitError(null);
    if (Object.keys(errs).length > 0) {
      return;
    }

    setSubmitting(true);
    try {
      await services.adminUser.createUser({
        username: username.trim(),
        password,
        nickname: nickname.trim(),
        email: email.trim(),
        is_admin: role === 'admin',
      });
      toast.success(t('admin.userManagement.createSuccess'));
      resetForm();
      onOpenChange(false);
      onCreated();
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : t('common.error'));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent
        title={t('admin.userManagement.createUser')}
        description={t('admin.userManagement.createDesc')}
        className='w-[min(calc(100vw-2rem),560px)]!'
        bodyClassName='max-h-[min(85dvh,700px)] overflow-y-auto'
      >
        <form onSubmit={(e) => void handleSubmit(e)} noValidate>
          <div className='space-y-4'>
            {submitError ? (
              <Alert tone='danger' title={submitError} />
            ) : null}

            <section
              className={cn(layout.detailSection, layout.detailSectionFirst)}
            >
              <h3 className={layout.sectionTitle}>
                {t('admin.userManagement.sectionCredentials')}
              </h3>
              <div className='space-y-4'>
                <Input
                  id='create-username'
                  label={t('admin.userManagement.username')}
                  placeholder={t('admin.userManagement.usernamePlaceholder')}
                  description={
                    fieldErrors.username
                      ? undefined
                      : t('admin.userManagement.usernameHint')
                  }
                  error={fieldErrors.username}
                  autoComplete='username'
                  value={username}
                  onChange={(e) => {
                    setUsername(e.target.value);
                    if (fieldErrors.username) {
                      setFieldErrors((prev) => ({...prev, username: undefined}));
                    }
                  }}
                />
                <PasswordStrength
                  id='create-password'
                  label={t('admin.userManagement.password')}
                  autoComplete='new-password'
                  value={password}
                  shortLength={6}
                  rules={passwordRules}
                  messages={passwordMessages}
                  error={fieldErrors.password}
                  onValueChange={(next) => {
                    setPassword(next);
                    if (fieldErrors.password) {
                      setFieldErrors((prev) => ({...prev, password: undefined}));
                    }
                  }}
                />
              </div>
            </section>

            <section className={layout.detailSection}>
              <h3 className={layout.sectionTitle}>
                {t('admin.userManagement.sectionProfile')}
              </h3>
              <div className='grid grid-cols-1 gap-4 sm:grid-cols-2'>
                <Input
                  id='create-nickname'
                  label={t('admin.userManagement.nickname')}
                  placeholder={t('admin.userManagement.nicknamePlaceholder')}
                  autoComplete='name'
                  value={nickname}
                  onChange={(e) => setNickname(e.target.value)}
                />
                <Input
                  id='create-email'
                  type='email'
                  label={t('admin.userManagement.email')}
                  placeholder={t('admin.userManagement.emailPlaceholder')}
                  autoComplete='email'
                  error={fieldErrors.email}
                  value={email}
                  onChange={(e) => {
                    setEmail(e.target.value);
                    if (fieldErrors.email) {
                      setFieldErrors((prev) => ({...prev, email: undefined}));
                    }
                  }}
                />
              </div>
            </section>

            <section className={layout.detailSection}>
              <h3 className={layout.sectionTitle}>
                {t('admin.userManagement.sectionRole')}
              </h3>
              <RadioCards
                layout='grid'
                minColumnWidth={200}
                value={role}
                onValueChange={(v) => setRole(v as 'user' | 'admin')}
                options={[
                  {
                    value: 'user',
                    label: t('admin.userManagement.roleStandard'),
                    description: t('admin.userManagement.roleStandardDesc'),
                    icon: <User className='size-4' aria-hidden />,
                  },
                  {
                    value: 'admin',
                    label: t('admin.userManagement.roleAdmin'),
                    description: t('admin.userManagement.roleAdminDesc'),
                    icon: <ShieldCheck className='size-4' aria-hidden />,
                  },
                ]}
              />
            </section>
          </div>

          <div className='mt-5 flex items-center justify-end gap-2 border-t-[0.5px] border-border pt-4'>
            <Button
              type='button'
              variant='secondary'
              size='sm'
              disabled={submitting}
              onClick={() => handleOpenChange(false)}
            >
              {t('common.cancel')}
            </Button>
            <Button
              type='submit'
              size='sm'
              loading={submitting}
              disabled={submitting}
            >
              {submitting ? t('common.creating') : t('common.create')}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
