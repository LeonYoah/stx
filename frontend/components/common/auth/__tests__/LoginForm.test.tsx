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

import {describe, it, expect, beforeEach, vi} from 'vitest';
import {render, screen, fireEvent, waitFor} from '@testing-library/react';
import {LoginForm} from '../LoginForm';

// Mock next/navigation
vi.mock('next/navigation', () => ({
  useSearchParams: () => ({
    get: vi.fn().mockReturnValue(null),
  }),
  useRouter: () => ({
    push: vi.fn(),
  }),
}));

// Mock next-intl
vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => {
    const translations: Record<string, string> = {
      'auth.brand.badge': '运维平台',
      'auth.brand.headline': '让 SeaTunnel 运维不再黑箱',
      'auth.brand.subtitle':
        '把配置、升级、诊断、恢复、调试放到一个统一入口；并原生提供 AI Agent 智能运维入口（CLI + Skill）。',
      'auth.brand.chipsLabel': '产品能力',
      'auth.brand.chipWizard': '配置向导',
      'auth.brand.chipInstall': '一键安装',
      'auth.brand.chipAgent': 'AI Agent · CLI + Skill',
      'auth.brand.chipCliSkill': 'CLI + Skill',
      'auth.brand.chipMarketplace': '插件市场',
      'auth.brand.chipUpgrade': '一键升级',
      'auth.brand.chipCheckpoint': 'Checkpoint 可视化',
      'auth.brand.chipDebugJob': '作业在线调试',
      'auth.brand.capabilityLabel': '核心能力',
      'auth.brand.capabilityPrefix': '能力：',
      'auth.brand.capabilityAgent': 'AI Agent 智能运维入口',
      'auth.brand.capabilityCluster': '集群感知',
      'auth.brand.capabilityObserve': '运行可观测',
      'auth.brand.capabilityRecover': '任务一键恢复',
      'auth.brand.capabilityConnector': 'Connector 一键下载',
      'auth.brand.capabilityCheckpoint': 'Checkpoint 可视化',
      'auth.brand.capabilityDag': 'HOCON DAG 解析',
      'auth.brand.footerAgent': 'AI Agent CLI',
      'auth.brand.footerDeploy': '自动化部署',
      'auth.brand.footerSubmit': '任务提交',
      'auth.brand.footerAlert': '巡检告警',
      'auth.brand.footerMarketplace': '插件市场',
      'auth.login.title': '欢迎登录 STX',
      'auth.login.subtitle': '登录后统一管理集群、任务、连接器、诊断与恢复操作。',
      'auth.login.username': '用户名',
      'auth.login.usernamePlaceholder': '请输入用户名',
      'auth.login.password': '密码',
      'auth.login.passwordPlaceholder': '请输入密码',
      'auth.login.loginButton': '进入控制台',
      'auth.login.loggingIn': '登录中...',
      'auth.login.orLoginWith': '或使用以下方式登录',
      'auth.login.showPassword': '显示密码',
      'auth.login.hidePassword': '隐藏密码',
      'auth.login.showPasswordShort': '显',
      'auth.login.hidePasswordShort': '隐',
      'auth.login.toggleTheme': '切换主题',
      'auth.logout.success': '您已成功登出平台',
      'auth.errors.emptyUsername': '请输入用户名',
      'auth.errors.emptyPassword': '请输入密码',
      'terms.agreement': '登录即表示您同意我们的',
      'terms.termsOfService': '服务条款',
      'terms.and': '和',
      'terms.privacyPolicy': '隐私政策',
      'terms.termsDialog.title': '服务条款',
      'terms.termsDialog.description': '请仔细阅读以下服务条款',
      'terms.termsDialog.general.title': '1. 一般条款',
      'terms.termsDialog.general.content1': '本服务条款规定了您使用服务的条件',
      'terms.termsDialog.general.content2':
        '通过访问或使用我们的服务，您同意受这些条款的约束',
      'terms.termsDialog.general.content3': '我们保留随时修改这些条款的权利',
      'terms.termsDialog.usage.title': '2. 使用规则',
      'terms.termsDialog.usage.content1':
        '您同意以合法和负责任的方式使用我们的服务',
      'terms.termsDialog.usage.content2': '禁止的行为包括但不限于：',
      'terms.termsDialog.usage.prohibited1': '发布非法内容',
      'terms.termsDialog.usage.prohibited2': '尝试未经授权访问',
      'terms.termsDialog.usage.prohibited3': '干扰服务运行',
      'terms.termsDialog.usage.prohibited4': '违反法律法规',
      'terms.termsDialog.content.title': '3. 内容规范',
      'terms.termsDialog.content.intro': '禁止分发以下内容：',
      'terms.termsDialog.content.pornography': '色情内容',
      'terms.termsDialog.content.promotion': '推广内容',
      'terms.termsDialog.content.illegal': '违法内容',
      'terms.termsDialog.content.harmful': '有害信息',
      'terms.termsDialog.content.false': '虚假信息',
      'terms.termsDialog.content.infringement': '侵权内容',
      'terms.termsDialog.content.warning': '违规内容将被删除',
      'terms.termsDialog.legal.title': '4. 法律合规',
      'terms.termsDialog.legal.intro': '本服务遵守相关法律法规',
      'terms.termsDialog.legal.law1': '网络安全法',
      'terms.termsDialog.legal.law2': '数据安全法',
      'terms.termsDialog.legal.law3': '个人信息保护法',
      'terms.termsDialog.legal.law4': '互联网信息服务管理办法',
      'terms.termsDialog.legal.law5': '网络信息内容生态治理规定',
      'terms.termsDialog.legal.compliance': '用户必须遵守法律法规',
      'terms.termsDialog.legal.cooperation': '我们将配合相关部门调查',
      'terms.termsDialog.account.title': '5. 账户责任',
      'terms.termsDialog.account.content1': '您负责维护账户安全',
      'terms.termsDialog.account.content2': '您对账户活动承担责任',
      'terms.termsDialog.account.content3': '发现未授权使用请通知我们',
      'terms.termsDialog.intellectual.title': '6. 知识产权',
      'terms.termsDialog.intellectual.content1': '内容受版权保护',
      'terms.termsDialog.intellectual.content2': '未经许可不得使用',
      'terms.termsDialog.limitation.title': '7. 责任限制',
      'terms.termsDialog.limitation.content1': '我们不对间接损害承担责任',
      'terms.termsDialog.limitation.content2': '总责任不超过支付金额',
      'terms.privacyDialog.title': '隐私政策',
      'terms.privacyDialog.description': '我们重视您的隐私',
      'terms.privacyDialog.collection.title': '1. 信息收集',
      'terms.privacyDialog.collection.intro': '我们收集以下信息：',
      'terms.privacyDialog.collection.item1': '账户信息',
      'terms.privacyDialog.collection.item2': '使用数据',
      'terms.privacyDialog.collection.item3': '技术信息',
      'terms.privacyDialog.collection.item4': '日志信息',
      'terms.privacyDialog.usage.title': '2. 信息使用',
      'terms.privacyDialog.usage.intro': '我们使用信息用于：',
      'terms.privacyDialog.usage.item1': '提供服务',
      'terms.privacyDialog.usage.item2': '改善体验',
      'terms.privacyDialog.usage.item3': '防止欺诈',
      'terms.privacyDialog.usage.item4': '遵守法律',
      'terms.privacyDialog.usage.item5': '发送通知',
      'terms.privacyDialog.sharing.title': '3. 信息共享',
      'terms.privacyDialog.sharing.content1': '我们不会出售您的信息',
      'terms.privacyDialog.sharing.content2': '以下情况可能共享：',
      'terms.privacyDialog.sharing.item1': '经您同意',
      'terms.privacyDialog.sharing.item2': '法律要求',
      'terms.privacyDialog.sharing.item3': '保护权利',
      'terms.privacyDialog.sharing.item4': '与服务提供商合作',
      'terms.privacyDialog.security.title': '4. 数据安全',
      'terms.privacyDialog.security.intro': '我们采用安全措施：',
      'terms.privacyDialog.security.item1': '数据加密',
      'terms.privacyDialog.security.item2': '访问控制',
      'terms.privacyDialog.security.item3': '安全审计',
      'terms.privacyDialog.security.item4': '权限管理',
      'terms.privacyDialog.security.warning': '没有方法是100%安全的',
      'terms.privacyDialog.retention.title': '5. 数据保留',
      'terms.privacyDialog.retention.intro': '我们保留信息的时间：',
      'terms.privacyDialog.retention.item1': '账户存在期间',
      'terms.privacyDialog.retention.item2': '90天',
      'terms.privacyDialog.retention.item3': '1年',
      'terms.privacyDialog.retention.deletion': '您可以要求删除数据',
      'terms.privacyDialog.rights.title': '6. 您的权利',
      'terms.privacyDialog.rights.intro': '您享有以下权利：',
      'terms.privacyDialog.rights.item1': '访问权',
      'terms.privacyDialog.rights.item2': '更正权',
      'terms.privacyDialog.rights.item3': '删除权',
      'terms.privacyDialog.rights.item4': '限制处理权',
    };
    return translations[key] || key;
  },
}));

// Mock useAuth hook
const {
  mockLoginWithCredentials,
  mockLoginWithOAuth,
  mockClearError,
  mockGetEnabledOAuthProviders,
} = vi.hoisted(() => ({
  mockLoginWithCredentials: vi.fn(),
  mockLoginWithOAuth: vi.fn(),
  mockClearError: vi.fn(),
  mockGetEnabledOAuthProviders: vi.fn(),
}));

vi.mock('@/hooks/use-auth', () => ({
  useAuth: () => ({
    loginWithCredentials: mockLoginWithCredentials,
    loginWithOAuth: mockLoginWithOAuth,
    error: null,
    clearError: mockClearError,
    user: null,
    isAuthenticated: false,
  }),
}));

vi.mock('@/hooks/use-theme-utils', () => ({
  useThemeUtils: () => ({
    toggle: vi.fn(),
    getIcon: () => 'theme-icon',
    getAction: () => '切换深色模式',
  }),
}));

vi.mock('@/lib/services', () => ({
  __esModule: true,
  default: {
    auth: {
      getEnabledOAuthProviders: mockGetEnabledOAuthProviders,
    },
  },
}));

describe('LoginForm', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetEnabledOAuthProviders.mockResolvedValue(['github', 'google']);
  });

  describe('表单渲染 (Requirements 8.1, 8.2)', () => {
    it('应该显示平台欢迎标题', () => {
      render(<LoginForm />);
      expect(screen.getByText('欢迎登录 STX')).toBeInTheDocument();
    });

    it('应该显示用户名输入框', () => {
      render(<LoginForm />);
      expect(screen.getByLabelText('用户名')).toBeInTheDocument();
      expect(screen.getByPlaceholderText('请输入用户名')).toBeInTheDocument();
    });

    it('应该显示密码输入框', () => {
      render(<LoginForm />);
      expect(screen.getByLabelText('密码')).toBeInTheDocument();
      expect(screen.getByPlaceholderText('请输入密码')).toBeInTheDocument();
    });

    it('应该显示登录按钮', () => {
      render(<LoginForm />);
      expect(
        screen.getByRole('button', {name: /进入控制台/}),
      ).toBeInTheDocument();
    });

    it('应该显示已启用的 OAuth 登录按钮（GitHub 和 Google）', async () => {
      render(<LoginForm />);

      expect(
        await screen.findByRole('button', {name: /GitHub/}),
      ).toBeInTheDocument();
      expect(
        await screen.findByRole('button', {name: /Google/}),
      ).toBeInTheDocument();
    });

    it('应该隐藏未启用的 OAuth 登录按钮', async () => {
      mockGetEnabledOAuthProviders.mockResolvedValueOnce([]);

      render(<LoginForm />);

      await waitFor(() => {
        expect(
          screen.queryByText('或使用以下方式登录'),
        ).not.toBeInTheDocument();
        expect(
          screen.queryByRole('button', {name: /GitHub/}),
        ).not.toBeInTheDocument();
        expect(
          screen.queryByRole('button', {name: /Google/}),
        ).not.toBeInTheDocument();
      });
    });

    it('应该显示服务条款和隐私政策链接', () => {
      render(<LoginForm />);
      expect(screen.getByText('服务条款')).toBeInTheDocument();
      expect(screen.getByText('隐私政策')).toBeInTheDocument();
    });
  });

  describe('表单验证 (Requirements 1.3)', () => {
    it('当用户名为空时应该显示验证错误', async () => {
      render(<LoginForm />);

      const passwordInput = screen.getByPlaceholderText('请输入密码');
      fireEvent.change(passwordInput, {target: {value: 'password123'}});

      const submitButton = screen.getByRole('button', {name: /进入控制台/});
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('请输入用户名')).toBeInTheDocument();
      });
      expect(mockLoginWithCredentials).not.toHaveBeenCalled();
    });

    it('当密码为空时应该显示验证错误', async () => {
      render(<LoginForm />);

      const usernameInput = screen.getByPlaceholderText('请输入用户名');
      fireEvent.change(usernameInput, {target: {value: 'admin'}});

      const submitButton = screen.getByRole('button', {name: /进入控制台/});
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('请输入密码')).toBeInTheDocument();
      });
      expect(mockLoginWithCredentials).not.toHaveBeenCalled();
    });

    it('当用户名只有空格时应该显示验证错误', async () => {
      render(<LoginForm />);

      const usernameInput = screen.getByPlaceholderText('请输入用户名');
      const passwordInput = screen.getByPlaceholderText('请输入密码');

      fireEvent.change(usernameInput, {target: {value: '   '}});
      fireEvent.change(passwordInput, {target: {value: 'password123'}});

      const submitButton = screen.getByRole('button', {name: /进入控制台/});
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('请输入用户名')).toBeInTheDocument();
      });
      expect(mockLoginWithCredentials).not.toHaveBeenCalled();
    });
  });

  describe('表单提交 (Requirements 1.5, 8.3)', () => {
    it('当输入有效凭证时应该调用 loginWithCredentials', async () => {
      mockLoginWithCredentials.mockResolvedValue(undefined);

      render(<LoginForm />);

      const usernameInput = screen.getByPlaceholderText('请输入用户名');
      const passwordInput = screen.getByPlaceholderText('请输入密码');

      fireEvent.change(usernameInput, {target: {value: 'admin'}});
      fireEvent.change(passwordInput, {target: {value: 'password123'}});

      const submitButton = screen.getByRole('button', {name: /进入控制台/});
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(mockLoginWithCredentials).toHaveBeenCalledWith(
          'admin',
          'password123',
          '/dashboard',
        );
      });
    });

    it('提交时应该显示加载状态', async () => {
      // 创建一个永不 resolve 的 Promise 来保持加载状态
      mockLoginWithCredentials.mockImplementation(() => new Promise(() => {}));

      render(<LoginForm />);

      const usernameInput = screen.getByPlaceholderText('请输入用户名');
      const passwordInput = screen.getByPlaceholderText('请输入密码');

      fireEvent.change(usernameInput, {target: {value: 'admin'}});
      fireEvent.change(passwordInput, {target: {value: 'password123'}});

      const submitButton = screen.getByRole('button', {name: /进入控制台/});
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('登录中...')).toBeInTheDocument();
      });
    });

    it('加载时应该禁用输入框和按钮', async () => {
      mockLoginWithCredentials.mockImplementation(() => new Promise(() => {}));

      render(<LoginForm />);

      const usernameInput = screen.getByPlaceholderText('请输入用户名');
      const passwordInput = screen.getByPlaceholderText('请输入密码');

      fireEvent.change(usernameInput, {target: {value: 'admin'}});
      fireEvent.change(passwordInput, {target: {value: 'password123'}});

      const submitButton = screen.getByRole('button', {name: /进入控制台/});
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(usernameInput).toBeDisabled();
        expect(passwordInput).toBeDisabled();
      });
    });
  });

  describe('OAuth 登录', () => {
    it('点击 GitHub 按钮应该调用 loginWithOAuth', async () => {
      render(<LoginForm />);

      const githubButton = await screen.findByRole('button', {name: /GitHub/});
      fireEvent.click(githubButton);

      await waitFor(() => {
        expect(mockLoginWithOAuth).toHaveBeenCalledWith('github', '/dashboard');
      });
    });

    it('点击 Google 按钮应该调用 loginWithOAuth', async () => {
      render(<LoginForm />);

      const googleButton = await screen.findByRole('button', {name: /Google/});
      fireEvent.click(googleButton);

      await waitFor(() => {
        expect(mockLoginWithOAuth).toHaveBeenCalledWith('google', '/dashboard');
      });
    });
  });

  describe('错误处理', () => {
    it('输入时应该清除验证错误', async () => {
      render(<LoginForm />);

      // 先触发验证错误
      const submitButton = screen.getByRole('button', {name: /进入控制台/});
      fireEvent.click(submitButton);

      await waitFor(() => {
        expect(screen.getByText('请输入用户名')).toBeInTheDocument();
      });

      // 输入用户名后错误应该消失
      const usernameInput = screen.getByPlaceholderText('请输入用户名');
      fireEvent.change(usernameInput, {target: {value: 'admin'}});

      await waitFor(() => {
        expect(screen.queryByText('请输入用户名')).not.toBeInTheDocument();
      });
    });
  });
});
