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

import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import {
  Eye,
  EyeOff,
  BellRing,
  CircleCheck,
  CircleX,
  History,
  Mail,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Search,
  Send,
  Trash2,
  Webhook,
  X,
} from 'lucide-react';
import services from '@/lib/services';
import type {ClusterInfo} from '@/lib/services/cluster';
import type {BasicUserInfo} from '@/lib/services/core/types';
import type {
  AlertPolicy,
  AlertPolicyBuilderKind,
  AlertPolicyCenterBootstrapData,
  AlertPolicyCondition,
  AlertPolicyListData,
  AlertPolicyTemplateSummary,
  AlertSeverity,
  NotificationChannel,
  NotificationChannelConnectionTestResult,
  NotificationChannelEmailConfig,
  NotificationDelivery,
  NotificationDeliveryListData,
  NotificationRecipientUser,
  NotificationChannelTestResult,
  UpsertAlertPolicyRequest,
  UpsertNotificationChannelRequest,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {
  resolvePreferredClusterId,
  rememberPreferredClusterId,
  isSoleDefaultCluster,
} from '@/lib/cluster-preference';
import {
  StatPillsBar,
  type StatPillItem,
  TableLoadingBar,
  TableSkeletonRows,
} from '@/components/common/layout';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {Card, CardContent, CardHeader, CardTitle} from '@/components/ui/card';
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Separator} from '@/components/ui/separator';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Switch} from '@/components/ui/switch';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import {Tabs, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {Textarea} from '@/components/ui/textarea';

// 规则与通知三级分段：告警规则 | 通知通道 | 投递记录
// Tertiary sections for Rules & Notifications: rules | channels | history
type PolicySection = 'rules' | 'channels' | 'history';

// 通道列表筛选：全部 / 邮件 / Webhook / 已启用
// Channel list filter: all / email / webhook / enabled
type ChannelPillFilter = 'all' | 'email' | 'webhook' | 'enabled';

// 投递记录状态筛选（客户端聚合 pill）
// Delivery history status filter (client-aggregated pills)
type HistoryPillFilter = 'all' | 'sent' | 'failed';

function resolvePolicySection(value: string | null): PolicySection {
  if (value === 'channels' || value === 'history' || value === 'rules') {
    return value;
  }
  return 'rules';
}

type StrategyEditorMode = 'static' | 'custom_promql';

type PolicyFormState = {
  name: string;
  description: string;
  strategyMode: StrategyEditorMode;
  templateKey: string;
  conditionOperator: string;
  conditionThreshold: string;
  conditionWindowMinutes: string;
  clusterId: string;
  severity: AlertSeverity;
  cooldownMinutes: string;
  sendRecovery: boolean;
  enabled: boolean;
  promql: string;
  emailChannelId: string;
  webhookChannelId: string;
  receiverUserIds: string[];
};

type EmailChannelFormState = {
  id: number | null;
  name: string;
  enabled: boolean;
  description: string;
  protocol: string;
  security: 'none' | 'starttls' | 'ssl';
  host: string;
  port: string;
  username: string;
  password: string;
  from: string;
  fromName: string;
  recipients: string;
};

type WebhookChannelFormState = {
  id: number | null;
  name: string;
  enabled: boolean;
  description: string;
  endpoint: string;
  secret: string;
};

type EmailTestReceiverOption = 'none' | string;

const EMPTY_BOOTSTRAP: AlertPolicyCenterBootstrapData = {
  generated_at: '',
  capability_mode: '',
  capabilities: [],
  builders: [],
  templates: [],
  components: [],
  notifiable_users: [],
  default_receiver_user_ids: [],
};

const EMPTY_HISTORY: NotificationDeliveryListData = {
  generated_at: '',
  page: 1,
  page_size: 10,
  total: 0,
  deliveries: [],
};

const CONDITION_OPERATORS = ['>', '>=', '<', '<=', '==', '!='] as const;

function createDefaultPolicyForm(): PolicyFormState {
  return {
    name: '',
    description: '',
    strategyMode: 'static',
    templateKey: '',
    conditionOperator: '',
    conditionThreshold: '',
    conditionWindowMinutes: '',
    clusterId: '',
    severity: 'warning',
    cooldownMinutes: '10',
    sendRecovery: true,
    enabled: true,
    promql: '',
    emailChannelId: 'none',
    webhookChannelId: 'none',
    receiverUserIds: [],
  };
}

function createDefaultEmailChannelForm(): EmailChannelFormState {
  return {
    id: null,
    name: '',
    enabled: true,
    description: '',
    protocol: 'smtp',
    security: 'ssl',
    host: '',
    port: defaultPortForSecurity('ssl'),
    username: '',
    password: '',
    from: '',
    fromName: '',
    recipients: '',
  };
}

function createDefaultWebhookChannelForm(): WebhookChannelFormState {
  return {
    id: null,
    name: '',
    enabled: true,
    description: '',
    endpoint: '',
    secret: '',
  };
}

function defaultPortForSecurity(
  security: EmailChannelFormState['security'],
): string {
  switch (security) {
    case 'none':
      return '25';
    case 'starttls':
      return '587';
    case 'ssl':
    default:
      return '465';
  }
}

function formatDateTime(value?: string | null): string {
  if (!value) {
    return '-';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toLocaleString();
}

function resolveSeverityVariant(
  severity: AlertSeverity,
): 'secondary' | 'destructive' {
  return severity === 'critical' ? 'destructive' : 'secondary';
}

function resolveDeliveryStatusVariant(
  status?: string,
): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (status) {
    case 'sent':
      return 'default';
    case 'failed':
      return 'destructive';
    case 'sending':
    case 'retrying':
      return 'secondary';
    default:
      return 'outline';
  }
}

function normalizePolicies(data?: AlertPolicyListData): AlertPolicy[] {
  if (!data || !Array.isArray(data.policies)) {
    return [];
  }
  return data.policies;
}

function normalizeChannels(
  channels: NotificationChannel[],
): NotificationChannel[] {
  return Array.isArray(channels) ? channels : [];
}

function notificationMethodSummary(
  policy: AlertPolicy,
  channelMap: Map<number, NotificationChannel>,
): string {
  const items = (policy.notification_channel_ids || [])
    .map((id) => channelMap.get(id))
    .filter(Boolean)
    .map((channel) => channel?.name || '')
    .filter(Boolean);
  return items.length > 0 ? items.join(' / ') : '-';
}

function parseRecipients(value: string): string[] {
  return value
    .split(/[;,\n]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

function supportsTemplateCondition(
  template?: AlertPolicyTemplateSummary | null,
): boolean {
  if (!template) {
    return false;
  }
  return (
    template.source_kind === 'metrics_template' ||
    Boolean(
      template.default_operator ||
      template.default_threshold ||
      template.default_window_minutes,
    )
  );
}

function buildConditionDefaults(
  template?: AlertPolicyTemplateSummary | null,
): Pick<
  PolicyFormState,
  'conditionOperator' | 'conditionThreshold' | 'conditionWindowMinutes'
> {
  if (!supportsTemplateCondition(template)) {
    return {
      conditionOperator: '',
      conditionThreshold: '',
      conditionWindowMinutes: '',
    };
  }

  return {
    conditionOperator: template?.default_operator || '>',
    conditionThreshold: template?.default_threshold || '',
    conditionWindowMinutes:
      template?.default_window_minutes !== undefined &&
      template?.default_window_minutes !== null
        ? String(template.default_window_minutes)
        : '1',
  };
}

function buildConditionStateFromPolicy(
  policy: AlertPolicy,
  template?: AlertPolicyTemplateSummary | null,
): Pick<
  PolicyFormState,
  'conditionOperator' | 'conditionThreshold' | 'conditionWindowMinutes'
> {
  const condition = Array.isArray(policy.conditions)
    ? policy.conditions[0]
    : null;
  if (!condition) {
    return buildConditionDefaults(template);
  }

  return {
    conditionOperator: condition.operator || template?.default_operator || '>',
    conditionThreshold:
      condition.threshold || template?.default_threshold || '',
    conditionWindowMinutes:
      condition.window_minutes !== undefined &&
      condition.window_minutes !== null
        ? String(condition.window_minutes)
        : String(template?.default_window_minutes || 1),
  };
}

function buildPromQLPreview(
  template: AlertPolicyTemplateSummary | null,
  form: Pick<
    PolicyFormState,
    'conditionOperator' | 'conditionThreshold' | 'conditionWindowMinutes'
  >,
): string {
  if (!template?.suggested_promql) {
    return '';
  }

  let preview = template.suggested_promql;
  const windowMinutes = Number.parseInt(form.conditionWindowMinutes, 10);
  if (Number.isFinite(windowMinutes) && windowMinutes > 0) {
    preview = preview.replace(/\[(\d+)m\]/g, `[${windowMinutes}m]`);
  }

  const operator = form.conditionOperator.trim();
  const threshold = form.conditionThreshold.trim();
  if (operator && threshold) {
    preview = preview.replace(
      /\s*(>=|<=|==|!=|>|<)\s*([^\s)]+)\s*$/,
      ` ${operator} ${threshold}`,
    );
  }
  return preview;
}

function getEmailConfig(
  channel?: NotificationChannel | null,
): NotificationChannelEmailConfig | null {
  if (!channel?.config?.email) {
    return null;
  }
  return channel.config.email;
}

function createEmailChannelFormFromChannel(
  channel: NotificationChannel,
): EmailChannelFormState {
  const config = getEmailConfig(channel);
  return {
    id: channel.id,
    name: channel.name,
    enabled: channel.enabled,
    description: channel.description || '',
    protocol: config?.protocol || 'smtp',
    security: config?.security || 'ssl',
    host: config?.host || '',
    port: String(config?.port || 465),
    username: config?.username || '',
    password: config?.password || '',
    from: config?.from || '',
    fromName: config?.from_name || '',
    recipients: (config?.recipients || []).join(', '),
  };
}

function buildEmailChannelPayload(
  form: EmailChannelFormState,
  enabledOverride?: boolean,
): UpsertNotificationChannelRequest {
  return {
    name: form.name.trim(),
    type: 'email',
    enabled: enabledOverride ?? form.enabled,
    description: form.description.trim(),
    config: {
      email: {
        protocol: form.protocol,
        security: form.security,
        host: form.host.trim(),
        port: Number.parseInt(form.port, 10) || 0,
        username: form.username.trim(),
        password: form.password,
        from: form.from.trim(),
        from_name: form.fromName.trim(),
        recipients: parseRecipients(form.recipients),
      },
    },
  };
}

function serializeEmailChannelForm(form: EmailChannelFormState): string {
  return JSON.stringify(form);
}

function createWebhookChannelFormFromChannel(
  channel: NotificationChannel,
): WebhookChannelFormState {
  return {
    id: channel.id,
    name: channel.name,
    enabled: channel.enabled,
    description: channel.description || '',
    endpoint: channel.endpoint || '',
    secret: channel.secret || '',
  };
}

function buildWebhookChannelPayload(
  form: WebhookChannelFormState,
  enabledOverride?: boolean,
): UpsertNotificationChannelRequest {
  return {
    name: form.name.trim(),
    type: 'webhook',
    enabled: enabledOverride ?? form.enabled,
    description: form.description.trim(),
    endpoint: form.endpoint.trim(),
    secret: form.secret.trim(),
  };
}

function serializeWebhookChannelForm(form: WebhookChannelFormState): string {
  return JSON.stringify(form);
}

function resolveDefaultEmailTestReceiver(
  currentUser: BasicUserInfo | null,
  notifiableUsers: NotificationRecipientUser[],
): EmailTestReceiverOption {
  if (!currentUser) {
    return 'none';
  }
  const matchedUser = notifiableUsers.find(
    (user) => user.id === currentUser.id,
  );
  return matchedUser ? String(matchedUser.id) : 'none';
}

function createPolicyFormFromPolicy(
  policy: AlertPolicy,
  channels: NotificationChannel[],
  templates: AlertPolicyTemplateSummary[],
): PolicyFormState {
  const form = createDefaultPolicyForm();
  const matchedTemplate =
    templates.find((template) => template.key === policy.template_key) || null;
  const selectedChannels = (policy.notification_channel_ids || [])
    .map((id) => channels.find((channel) => channel.id === id))
    .filter(Boolean) as NotificationChannel[];
  const selectedEmailChannel =
    selectedChannels.find((channel) => channel.type === 'email') || null;
  const selectedWebhookChannel =
    selectedChannels.find((channel) => channel.type === 'webhook') || null;

  return {
    ...form,
    name: policy.name || '',
    description: policy.description || '',
    strategyMode:
      policy.policy_type === 'custom_promql' ? 'custom_promql' : 'static',
    templateKey: policy.template_key || '',
    ...buildConditionStateFromPolicy(policy, matchedTemplate),
    clusterId: policy.cluster_id || '',
    severity: policy.severity,
    cooldownMinutes: String(policy.cooldown_minutes ?? 0),
    sendRecovery: policy.send_recovery,
    enabled: policy.enabled,
    promql: policy.promql || '',
    emailChannelId: selectedEmailChannel
      ? String(selectedEmailChannel.id)
      : 'none',
    webhookChannelId: selectedWebhookChannel
      ? String(selectedWebhookChannel.id)
      : 'none',
    receiverUserIds: Array.isArray(policy.receiver_user_ids)
      ? policy.receiver_user_ids.map((id) => String(id))
      : [],
  };
}

function buildPolicyPayload(
  form: PolicyFormState,
  selectedTemplate: AlertPolicyTemplateSummary | null,
): UpsertAlertPolicyRequest {
  const notificationChannelIds = [form.emailChannelId, form.webhookChannelId]
    .filter((value) => value !== 'none')
    .map((value) => Number.parseInt(value, 10))
    .filter((value) => Number.isFinite(value));
  const receiverUserIds = (form.receiverUserIds || [])
    .map((value) => Number.parseInt(value, 10))
    .filter((value) => Number.isFinite(value) && value > 0);

  const policyType: AlertPolicyBuilderKind =
    form.strategyMode === 'custom_promql'
      ? 'custom_promql'
      : (selectedTemplate?.source_kind as AlertPolicyBuilderKind) ||
        'platform_health';
  const parsedConditionWindowMinutes = Number.parseInt(
    form.conditionWindowMinutes,
    10,
  );
  const conditions: AlertPolicyCondition[] =
    form.strategyMode === 'static' &&
    supportsTemplateCondition(selectedTemplate)
      ? [
          {
            metric_key: selectedTemplate?.key || '',
            operator:
              form.conditionOperator.trim() ||
              selectedTemplate?.default_operator ||
              '>',
            threshold:
              form.conditionThreshold.trim() ||
              selectedTemplate?.default_threshold ||
              '',
            window_minutes:
              Number.isFinite(parsedConditionWindowMinutes) &&
              parsedConditionWindowMinutes >= 0
                ? parsedConditionWindowMinutes
                : selectedTemplate?.default_window_minutes || 0,
          },
        ]
      : [];

  return {
    name: form.name.trim(),
    description: form.description.trim(),
    policy_type: policyType,
    template_key:
      form.strategyMode === 'custom_promql' ? undefined : form.templateKey,
    legacy_rule_key:
      form.strategyMode === 'custom_promql'
        ? undefined
        : selectedTemplate?.legacy_rule_key || undefined,
    cluster_id: form.clusterId,
    severity: form.severity,
    enabled: form.enabled,
    cooldown_minutes: Number.parseInt(form.cooldownMinutes, 10) || 0,
    send_recovery: form.sendRecovery,
    promql:
      form.strategyMode === 'custom_promql' ? form.promql.trim() : undefined,
    conditions,
    notification_channel_ids: notificationChannelIds,
    receiver_user_ids: receiverUserIds,
  };
}

function getPolicyExecutionStatusLabel(
  t: ReturnType<typeof useTranslations>,
  status?: string,
): string {
  switch (status) {
    case 'sent':
      return t('history.statuses.sent');
    case 'failed':
      return t('history.statuses.failed');
    case 'partial':
      return t('executionStatuses.partial');
    case 'matched':
      return t('executionStatuses.matched');
    default:
      return t('executionStatuses.idle');
  }
}

function getTemplateTranslation(
  t: ReturnType<typeof useTranslations>,
  templateKey: string,
  field: 'name' | 'description',
): string | null {
  if (!templateKey) {
    return null;
  }

  try {
    return t(`templates.${templateKey}.${field}` as never);
  } catch {
    return null;
  }
}

function getTemplateDisplayName(
  template: Pick<AlertPolicyTemplateSummary, 'key' | 'name'> | null | undefined,
  t: ReturnType<typeof useTranslations>,
): string {
  if (!template) {
    return '-';
  }

  return (
    getTemplateTranslation(t, template.key, 'name') ||
    template.name ||
    template.key
  );
}

function getTemplateDescription(
  template:
    | Pick<AlertPolicyTemplateSummary, 'key' | 'description'>
    | null
    | undefined,
  t: ReturnType<typeof useTranslations>,
): string {
  if (!template) {
    return '';
  }

  return (
    getTemplateTranslation(t, template.key, 'description') ||
    template.description ||
    ''
  );
}

export function MonitoringPolicyCenter() {
  const rootT = useTranslations('monitoringCenter');
  const t = useTranslations('monitoringCenter.policyCenterV2');
  const legacyT = useTranslations('monitoringCenter.policyCenter');
  const searchParams = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();

  // 三级分段与 URL section 同步（缺省 rules，可省略 query）
  // Sync tertiary section with URL section param (default rules; omit ok)
  const activeSection = useMemo(
    () => resolvePolicySection(searchParams.get('section')),
    [searchParams],
  );

  const setActiveSection = useCallback(
    (next: PolicySection) => {
      const params = new URLSearchParams(searchParams.toString());
      // 确保二级 tab 落在 policies，避免仅带 section 的脏链
      // Ensure secondary tab stays on policies for section deep-links
      if (params.get('tab') !== 'policies') {
        params.set('tab', 'policies');
      }
      if (next === 'rules') {
        params.delete('section');
      } else {
        params.set('section', next);
      }
      const query = params.toString();
      router.replace(query ? `${pathname}?${query}` : pathname);
    },
    [pathname, router, searchParams],
  );

  const [bootstrap, setBootstrap] =
    useState<AlertPolicyCenterBootstrapData>(EMPTY_BOOTSTRAP);
  const [currentUser, setCurrentUser] = useState<BasicUserInfo | null>(null);
  const [policies, setPolicies] = useState<AlertPolicy[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [loading, setLoading] = useState<boolean>(true);
  const [savingPolicy, setSavingPolicy] = useState<boolean>(false);
  const [editingPolicyId, setEditingPolicyId] = useState<number | null>(null);
  const [policyDialogOpen, setPolicyDialogOpen] = useState<boolean>(false);
  const [policySearchKeyword, setPolicySearchKeyword] = useState<string>('');
  const [policyClusterFilter, setPolicyClusterFilter] =
    useState<string>('all');
  const [policyStatusFilter, setPolicyStatusFilter] = useState<string>('all');
  const [deletingPolicyId, setDeletingPolicyId] = useState<number | null>(null);
  const [historyPolicyIdFilter, setHistoryPolicyIdFilter] = useState<
    number | null
  >(null);
  const [historyData, setHistoryData] =
    useState<NotificationDeliveryListData>(EMPTY_HISTORY);
  const [historyLoading, setHistoryLoading] = useState<boolean>(false);
  const [historyStatusFilter, setHistoryStatusFilter] =
    useState<HistoryPillFilter>('all');
  const [historySearchKeyword, setHistorySearchKeyword] = useState<string>('');
  const [channelPillFilter, setChannelPillFilter] =
    useState<ChannelPillFilter>('all');
  const [channelListSearch, setChannelListSearch] = useState<string>('');

  const [emailDialogOpen, setEmailDialogOpen] = useState<boolean>(false);
  // 放弃未保存变更确认（系统 AlertDialog，替代 window.confirm）
  // Discard-unsaved confirm via in-app AlertDialog (replaces window.confirm)
  const [emailDiscardDialogOpen, setEmailDiscardDialogOpen] =
    useState<boolean>(false);
  const emailDiscardActionRef = useRef<(() => void) | null>(null);
  const emailChannelFormRef = useRef<EmailChannelFormState>(
    createDefaultEmailChannelForm(),
  );
  const [emailChannelForm, setEmailChannelForm] =
    useState<EmailChannelFormState>(createDefaultEmailChannelForm());
  const [emailChannelBaseline, setEmailChannelBaseline] = useState<string>(
    serializeEmailChannelForm(createDefaultEmailChannelForm()),
  );
  const [emailChannelTouched, setEmailChannelTouched] =
    useState<boolean>(false);
  const [savingEmailChannel, setSavingEmailChannel] = useState<boolean>(false);
  const [testingEmailChannelId, setTestingEmailChannelId] = useState<
    number | null
  >(null);
  const [showEmailPassword, setShowEmailPassword] = useState<boolean>(false);
  const [testingEmailConnection, setTestingEmailConnection] =
    useState<boolean>(false);
  const [emailTestReceiver, setEmailTestReceiver] =
    useState<EmailTestReceiverOption>('none');
  const [
    lastEmailChannelConnectionResult,
    setLastEmailChannelConnectionResult,
  ] = useState<NotificationChannelConnectionTestResult | null>(null);
  const [
    lastEmailChannelConnectionSnapshot,
    setLastEmailChannelConnectionSnapshot,
  ] = useState<string | null>(null);
  const [lastEmailChannelTestResult, setLastEmailChannelTestResult] =
    useState<NotificationChannelTestResult | null>(null);
  const [lastEmailChannelTestSnapshot, setLastEmailChannelTestSnapshot] =
    useState<string | null>(null);
  const [webhookDialogOpen, setWebhookDialogOpen] = useState<boolean>(false);
  const [webhookDiscardDialogOpen, setWebhookDiscardDialogOpen] =
    useState<boolean>(false);
  const webhookDiscardActionRef = useRef<(() => void) | null>(null);
  const webhookChannelFormRef = useRef<WebhookChannelFormState>(
    createDefaultWebhookChannelForm(),
  );
  const [webhookChannelForm, setWebhookChannelForm] =
    useState<WebhookChannelFormState>(createDefaultWebhookChannelForm());
  const [webhookChannelBaseline, setWebhookChannelBaseline] = useState<string>(
    serializeWebhookChannelForm(createDefaultWebhookChannelForm()),
  );
  const [webhookChannelTouched, setWebhookChannelTouched] =
    useState<boolean>(false);
  const [savingWebhookChannel, setSavingWebhookChannel] =
    useState<boolean>(false);
  const [testingWebhookChannelId, setTestingWebhookChannelId] = useState<
    number | null
  >(null);
  const [showWebhookSecret, setShowWebhookSecret] = useState<boolean>(false);
  const [lastWebhookChannelTestResult, setLastWebhookChannelTestResult] =
    useState<NotificationChannelTestResult | null>(null);
  const [lastWebhookChannelTestSnapshot, setLastWebhookChannelTestSnapshot] =
    useState<string | null>(null);
  const [form, setForm] = useState<PolicyFormState>(createDefaultPolicyForm());

  const loadResources = useCallback(async () => {
    setLoading(true);
    try {
      const [
        bootstrapResult,
        policiesResult,
        channelsResult,
        clustersResult,
        currentUserResult,
      ] = await Promise.all([
        services.monitoring.getAlertPolicyCenterBootstrapSafe(),
        services.monitoring.listAlertPoliciesSafe(),
        services.monitoring.listNotificationChannelsSafe(),
        services.cluster.getClustersSafe({current: 1, size: 100}),
        services.auth
          .getUserInfo()
          .then((data) => ({success: true as const, data}))
          .catch(() => ({success: false as const})),
      ]);

      if (bootstrapResult.success && bootstrapResult.data) {
        setBootstrap(bootstrapResult.data);
      } else {
        toast.error(bootstrapResult.error || legacyT('loadError'));
        setBootstrap(EMPTY_BOOTSTRAP);
      }

      if (policiesResult.success && policiesResult.data) {
        setPolicies(normalizePolicies(policiesResult.data));
      } else {
        toast.error(policiesResult.error || legacyT('policyListLoadError'));
        setPolicies([]);
      }

      if (channelsResult.success && channelsResult.data) {
        setChannels(normalizeChannels(channelsResult.data.channels || []));
      } else {
        toast.error(channelsResult.error || legacyT('channelListLoadError'));
        setChannels([]);
      }

      if (clustersResult.success && clustersResult.data) {
        setClusters(clustersResult.data.clusters || []);
      } else {
        toast.error(clustersResult.error || legacyT('clusterListLoadError'));
        setClusters([]);
      }

      if (currentUserResult.success) {
        setCurrentUser(currentUserResult.data);
      } else {
        setCurrentUser(null);
      }
    } finally {
      setLoading(false);
    }
  }, [legacyT]);

  useEffect(() => {
    void loadResources();
  }, [loadResources]);

  const channelMap = useMemo(
    () => new Map(channels.map((channel) => [channel.id, channel])),
    [channels],
  );

  const emailChannels = useMemo(
    () => channels.filter((channel) => channel.type === 'email'),
    [channels],
  );
  const webhookChannels = useMemo(
    () => channels.filter((channel) => channel.type === 'webhook'),
    [channels],
  );
  const notifiableUsers = useMemo(
    () =>
      Array.isArray(bootstrap.notifiable_users)
        ? bootstrap.notifiable_users
        : [],
    [bootstrap.notifiable_users],
  );
  const defaultReceiverUserIds = useMemo(
    () =>
      Array.isArray(bootstrap.default_receiver_user_ids)
        ? bootstrap.default_receiver_user_ids.map((id) => String(id))
        : [],
    [bootstrap.default_receiver_user_ids],
  );
  const defaultEmailTestReceiver = useMemo(
    () => resolveDefaultEmailTestReceiver(currentUser, notifiableUsers),
    [currentUser, notifiableUsers],
  );
  const currentUserNotifiableRecipient = useMemo(
    () =>
      currentUser
        ? notifiableUsers.find((user) => user.id === currentUser.id) || null
        : null,
    [currentUser, notifiableUsers],
  );
  const currentUserDisplayName = useMemo(
    () =>
      currentUser
        ? currentUser.nickname ||
          currentUser.username ||
          t('channel.currentUser')
        : '',
    [currentUser, t],
  );
  const preferredDefaultReceiverUserIds = useMemo(() => {
    if (currentUserNotifiableRecipient) {
      return [String(currentUserNotifiableRecipient.id)];
    }
    return defaultReceiverUserIds;
  }, [currentUserNotifiableRecipient, defaultReceiverUserIds]);

  const builderMap = useMemo(() => {
    const map = new Map<string, string>();
    for (const builder of bootstrap.builders || []) {
      map.set(builder.key, builder.status);
    }
    return map;
  }, [bootstrap.builders]);

  const templateGroups = useMemo(() => {
    const platformTemplates = (bootstrap.templates || []).filter(
      (template) => template.source_kind === 'platform_health',
    );
    const metricsTemplates = (bootstrap.templates || []).filter(
      (template) => template.source_kind === 'metrics_template',
    );
    return {
      platformTemplates,
      metricsTemplates,
    };
  }, [bootstrap.templates]);

  const availableStaticTemplates = useMemo(() => {
    const items = [...templateGroups.platformTemplates];
    if (builderMap.get('metrics_template') === 'available') {
      items.push(...templateGroups.metricsTemplates);
    }
    return items;
  }, [builderMap, templateGroups]);
  const allStaticTemplates = useMemo(
    () => [
      ...templateGroups.platformTemplates,
      ...templateGroups.metricsTemplates,
    ],
    [templateGroups.metricsTemplates, templateGroups.platformTemplates],
  );
  const metricsTemplatesAvailable =
    builderMap.get('metrics_template') === 'available';

  const selectedTemplate = useMemo(
    () =>
      allStaticTemplates.find(
        (template) => template.key === form.templateKey,
      ) || null,
    [allStaticTemplates, form.templateKey],
  );
  const selectedTemplateSupportsCondition = useMemo(
    () => supportsTemplateCondition(selectedTemplate),
    [selectedTemplate],
  );
  const selectedTemplatePromQLPreview = useMemo(
    () => buildPromQLPreview(selectedTemplate, form),
    [form, selectedTemplate],
  );

  const currentEmailChannel = useMemo(
    () =>
      emailChannels.find(
        (channel) => String(channel.id) === form.emailChannelId,
      ) || null,
    [emailChannels, form.emailChannelId],
  );
  const applyEmailChannelFormSnapshot = useCallback(
    (nextForm: EmailChannelFormState) => {
      setEmailChannelForm(nextForm);
      setEmailChannelBaseline(serializeEmailChannelForm(nextForm));
      setEmailChannelTouched(false);
    },
    [],
  );
  const updateEmailChannelForm = useCallback(
    (updater: (prev: EmailChannelFormState) => EmailChannelFormState) => {
      setEmailChannelTouched(true);
      setEmailChannelForm((prev) => updater(prev));
    },
    [],
  );
  const emailChannelSerialized = useMemo(
    () => serializeEmailChannelForm(emailChannelForm),
    [emailChannelForm],
  );
  // 保持最新表单快照，供打开弹窗后吸收浏览器自动填充 / Keep latest form for autofill absorb
  emailChannelFormRef.current = emailChannelForm;
  const emailChannelDirty = useMemo(
    () =>
      emailChannelTouched && emailChannelSerialized !== emailChannelBaseline,
    [emailChannelBaseline, emailChannelSerialized, emailChannelTouched],
  );
  const selectedEmailTestRecipient = useMemo(
    () =>
      notifiableUsers.find((user) => String(user.id) === emailTestReceiver) ||
      null,
    [emailTestReceiver, notifiableUsers],
  );
  const editingEmailChannel = useMemo(
    () =>
      emailChannelForm.id !== null
        ? emailChannels.find((channel) => channel.id === emailChannelForm.id) ||
          null
        : null,
    [emailChannelForm.id, emailChannels],
  );
  const applyWebhookChannelFormSnapshot = useCallback(
    (nextForm: WebhookChannelFormState) => {
      setWebhookChannelForm(nextForm);
      setWebhookChannelBaseline(serializeWebhookChannelForm(nextForm));
      setWebhookChannelTouched(false);
    },
    [],
  );
  const updateWebhookChannelForm = useCallback(
    (updater: (prev: WebhookChannelFormState) => WebhookChannelFormState) => {
      setWebhookChannelTouched(true);
      setWebhookChannelForm((prev) => updater(prev));
    },
    [],
  );
  const webhookChannelSerialized = useMemo(
    () => serializeWebhookChannelForm(webhookChannelForm),
    [webhookChannelForm],
  );
  webhookChannelFormRef.current = webhookChannelForm;
  const webhookChannelDirty = useMemo(
    () =>
      webhookChannelTouched &&
      webhookChannelSerialized !== webhookChannelBaseline,
    [webhookChannelBaseline, webhookChannelSerialized, webhookChannelTouched],
  );
  const editingWebhookChannel = useMemo(
    () =>
      webhookChannelForm.id !== null
        ? webhookChannels.find(
            (channel) => channel.id === webhookChannelForm.id,
          ) || null
        : null,
    [webhookChannelForm.id, webhookChannels],
  );
  const currentWebhookChannel = useMemo(
    () =>
      webhookChannels.find(
        (channel) => String(channel.id) === form.webhookChannelId,
      ) || null,
    [form.webhookChannelId, webhookChannels],
  );
  const emailConnectionStatusOutdated = useMemo(
    () =>
      lastEmailChannelConnectionResult !== null &&
      lastEmailChannelConnectionSnapshot !== emailChannelSerialized,
    [
      emailChannelSerialized,
      lastEmailChannelConnectionResult,
      lastEmailChannelConnectionSnapshot,
    ],
  );
  const emailTestStatusOutdated = useMemo(
    () =>
      lastEmailChannelTestResult !== null &&
      lastEmailChannelTestSnapshot !== emailChannelSerialized,
    [
      emailChannelSerialized,
      lastEmailChannelTestResult,
      lastEmailChannelTestSnapshot,
    ],
  );
  const isTestingCurrentEmailChannel = useMemo(
    () =>
      testingEmailChannelId !== null &&
      (testingEmailChannelId === 0 ||
        testingEmailChannelId === emailChannelForm.id),
    [emailChannelForm.id, testingEmailChannelId],
  );
  const webhookTestStatusOutdated = useMemo(
    () =>
      lastWebhookChannelTestResult !== null &&
      lastWebhookChannelTestSnapshot !== webhookChannelSerialized,
    [
      lastWebhookChannelTestResult,
      lastWebhookChannelTestSnapshot,
      webhookChannelSerialized,
    ],
  );
  const isTestingCurrentWebhookChannel = useMemo(
    () =>
      testingWebhookChannelId !== null &&
      (testingWebhookChannelId === 0 ||
        testingWebhookChannelId === webhookChannelForm.id),
    [testingWebhookChannelId, webhookChannelForm.id],
  );
  const emailTestRecipientHint = useMemo(() => {
    if (selectedEmailTestRecipient) {
      return t('channel.testRecipientHintSelected', {
        value: selectedEmailTestRecipient.email,
      });
    }
    if (currentUser && !currentUserNotifiableRecipient) {
      return t('channel.testRecipientHintCurrentUserMissing', {
        value: currentUserDisplayName,
      });
    }
    return t('channel.testRecipientHint');
  }, [
    currentUser,
    currentUserDisplayName,
    currentUserNotifiableRecipient,
    selectedEmailTestRecipient,
    t,
  ]);

  useEffect(() => {
    if (!emailDialogOpen) {
      return;
    }
    if (emailTestReceiver !== 'none') {
      return;
    }
    if (defaultEmailTestReceiver === 'none') {
      return;
    }
    setEmailTestReceiver(defaultEmailTestReceiver);
  }, [defaultEmailTestReceiver, emailDialogOpen, emailTestReceiver]);

  // 弹窗打开后短暂窗口内吸收浏览器自动填充，避免误判未保存变更
  // After open, absorb browser autofill briefly so it is not treated as dirty edits
  useEffect(() => {
    if (!emailDialogOpen) {
      return;
    }
    const timer = window.setTimeout(() => {
      setEmailChannelBaseline(
        serializeEmailChannelForm(emailChannelFormRef.current),
      );
      setEmailChannelTouched(false);
    }, 500);
    return () => window.clearTimeout(timer);
  }, [emailDialogOpen]);

  useEffect(() => {
    if (!webhookDialogOpen) {
      return;
    }
    const timer = window.setTimeout(() => {
      setWebhookChannelBaseline(
        serializeWebhookChannelForm(webhookChannelFormRef.current),
      );
      setWebhookChannelTouched(false);
    }, 500);
    return () => window.clearTimeout(timer);
  }, [webhookDialogOpen]);

  // 请求放弃未保存变更：无脏数据则直接执行，否则弹出系统确认框
  // Request discard of unsaved changes: run immediately if clean, else show AlertDialog
  const requestEmailChannelDiscard = useCallback(
    (action: () => void) => {
      if (!emailDialogOpen || !emailChannelDirty) {
        action();
        return;
      }
      emailDiscardActionRef.current = action;
      setEmailDiscardDialogOpen(true);
    },
    [emailChannelDirty, emailDialogOpen],
  );
  const closeEmailChannelDialog = useCallback(
    (discardChanges: boolean = false) => {
      if (discardChanges && emailChannelDirty) {
        try {
          const restoredForm = JSON.parse(
            emailChannelBaseline,
          ) as EmailChannelFormState;
          setEmailChannelForm(restoredForm);
        } catch {
          // no-op: keep current form if baseline cannot be restored
        }
      }
      setEmailDialogOpen(false);
      setShowEmailPassword(false);
      setEmailChannelTouched(false);
    },
    [emailChannelBaseline, emailChannelDirty],
  );
  const requestWebhookChannelDiscard = useCallback(
    (action: () => void) => {
      if (!webhookDialogOpen || !webhookChannelDirty) {
        action();
        return;
      }
      webhookDiscardActionRef.current = action;
      setWebhookDiscardDialogOpen(true);
    },
    [webhookChannelDirty, webhookDialogOpen],
  );
  const closeWebhookChannelDialog = useCallback(
    (discardChanges: boolean = false) => {
      if (discardChanges && webhookChannelDirty) {
        try {
          const restoredForm = JSON.parse(
            webhookChannelBaseline,
          ) as WebhookChannelFormState;
          setWebhookChannelForm(restoredForm);
        } catch {
          // no-op: keep current form if baseline cannot be restored
        }
      }
      setWebhookDialogOpen(false);
      setShowWebhookSecret(false);
      setWebhookChannelTouched(false);
    },
    [webhookChannelBaseline, webhookChannelDirty],
  );

  useEffect(() => {
    if (clusters.length === 0) {
      return;
    }
    setForm((prev) => {
      if (prev.clusterId) {
        return prev;
      }
      const preferred = resolvePreferredClusterId(clusters, {
        fallbackToFirst: true,
      });
      return {
        ...prev,
        clusterId: preferred != null ? String(preferred) : '',
      };
    });
  }, [clusters]);

  useEffect(() => {
    if (availableStaticTemplates.length === 0) {
      return;
    }
    setForm((prev) => {
      if (prev.strategyMode !== 'static' || prev.templateKey) {
        return prev;
      }
      const defaultTemplate = availableStaticTemplates[0];
      return {
        ...prev,
        templateKey: defaultTemplate.key,
        ...buildConditionDefaults(defaultTemplate),
      };
    });
  }, [availableStaticTemplates]);

  const resetPolicyForm = useCallback(() => {
    const defaultTemplate = availableStaticTemplates[0] || null;
    setEditingPolicyId(null);
    setForm((prev) => {
      const nextForm = createDefaultPolicyForm();
      const preferred =
        resolvePreferredClusterId(clusters, {fallbackToFirst: true}) ??
        (clusters[0] ? clusters[0].id : null);
      return {
        ...nextForm,
        ...buildConditionDefaults(defaultTemplate),
        clusterId:
          prev.clusterId || (preferred != null ? String(preferred) : ''),
        templateKey: defaultTemplate?.key || nextForm.templateKey,
        receiverUserIds: preferredDefaultReceiverUserIds,
      };
    });
  }, [availableStaticTemplates, clusters, preferredDefaultReceiverUserIds]);

  useEffect(() => {
    if (
      editingPolicyId !== null ||
      preferredDefaultReceiverUserIds.length === 0
    ) {
      return;
    }
    setForm((prev) => {
      if ((prev.receiverUserIds || []).length > 0) {
        return prev;
      }
      return {
        ...prev,
        receiverUserIds: preferredDefaultReceiverUserIds,
      };
    });
  }, [editingPolicyId, preferredDefaultReceiverUserIds]);

  useEffect(() => {
    const shouldBlockLeave =
      (emailDialogOpen && emailChannelDirty) ||
      (webhookDialogOpen && webhookChannelDirty);
    if (!shouldBlockLeave) {
      return;
    }
    const handler = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  }, [
    emailChannelDirty,
    emailDialogOpen,
    webhookChannelDirty,
    webhookDialogOpen,
  ]);

  const handleRefresh = useCallback(async () => {
    await loadResources();
  }, [loadResources]);

  // 打开策略编辑弹窗
  // Open the policy editing dialog
  const handleEditPolicy = useCallback(
    (policy: AlertPolicy) => {
      setEditingPolicyId(policy.id);
      setForm(createPolicyFormFromPolicy(policy, channels, allStaticTemplates));
      setPolicyDialogOpen(true);
    },
    [allStaticTemplates, channels],
  );

  // 快捷切换策略的启用/停用状态
  // Quick toggle enabling/disabling status of an alert policy
  const handleTogglePolicyEnabled = useCallback(
    async (policy: AlertPolicy, nextEnabled: boolean) => {
      try {
        const payload: UpsertAlertPolicyRequest = {
          name: policy.name,
          description: policy.description,
          cluster_id: policy.cluster_id,
          policy_type: (policy.policy_type as AlertPolicyBuilderKind) || 'platform_health',
          template_key: policy.template_key || undefined,
          legacy_rule_key: policy.legacy_rule_key || undefined,
          promql: policy.promql || undefined,
          severity: policy.severity,
          enabled: nextEnabled,
          cooldown_minutes: policy.cooldown_minutes,
          send_recovery: policy.send_recovery,
          conditions: policy.conditions || undefined,
          receiver_user_ids: policy.receiver_user_ids,
          notification_channel_ids: policy.notification_channel_ids,
        };
        const result = await services.monitoring.updateAlertPolicySafe(
          policy.id,
          payload,
        );
        if (!result.success) {
          toast.error(result.error || t('updateError'));
          return;
        }
        toast.success(
          nextEnabled ? t('toggleEnabledSuccess') : t('toggleDisabledSuccess'),
        );
        await loadResources();
      } catch {
        toast.error(t('updateError'));
      }
    },
    [loadResources, t],
  );

  const handleDeletePolicy = async (policyId: number) => {
    setDeletingPolicyId(policyId);
    try {
      const result = await services.monitoring.deleteAlertPolicySafe(policyId);
      if (!result.success) {
        toast.error(result.error || t('deleteError'));
        return;
      }
      toast.success(t('deleteSuccess'));
      if (editingPolicyId === policyId) {
        resetPolicyForm();
        setPolicyDialogOpen(false);
      }
      await loadResources();
    } finally {
      setDeletingPolicyId(null);
    }
  };

  // 从规则行跳到投递记录分段，并按规则过滤
  // Jump from a rule row into delivery history section filtered by that rule
  const openPolicyDeliveryHistory = useCallback(
    (policy: AlertPolicy) => {
      setHistoryPolicyIdFilter(policy.id);
      setHistoryStatusFilter('all');
      setHistorySearchKeyword('');
      setActiveSection('history');
    },
    [setActiveSection],
  );

  // 加载全局投递记录（页内主工作面；较大 page_size 供客户端 pill 聚合）
  // Load global delivery history (in-page surface; larger page size for client pills)
  const loadDeliveryHistory = useCallback(async () => {
    setHistoryLoading(true);
    try {
      const result = await services.monitoring.listNotificationDeliveriesSafe({
        page: 1,
        page_size: 100,
        policy_id: historyPolicyIdFilter ?? undefined,
      });
      if (!result.success || !result.data) {
        toast.error(result.error || t('deliveryHistory.loadError'));
        setHistoryData(EMPTY_HISTORY);
        return;
      }
      setHistoryData(result.data);
    } finally {
      setHistoryLoading(false);
    }
  }, [historyPolicyIdFilter, t]);

  useEffect(() => {
    if (activeSection !== 'history') {
      return;
    }
    void loadDeliveryHistory();
  }, [activeSection, loadDeliveryHistory]);

  const handleSubmitPolicy = async () => {
    if (!form.name.trim()) {
      toast.error(t('nameRequired'));
      return;
    }
    if (!form.clusterId) {
      toast.error(t('clusterRequired'));
      return;
    }
    if (form.strategyMode === 'static' && !selectedTemplate) {
      toast.error(t('templateRequired'));
      return;
    }
    if (form.strategyMode === 'custom_promql' && !form.promql.trim()) {
      toast.error(t('promqlRequired'));
      return;
    }
    if (selectedTemplateSupportsCondition) {
      if (!form.conditionThreshold.trim()) {
        toast.error(t('conditionThresholdRequired'));
        return;
      }
      const parsedWindowMinutes = Number.parseInt(
        form.conditionWindowMinutes,
        10,
      );
      if (
        !Number.isFinite(parsedWindowMinutes) ||
        parsedWindowMinutes < 0 ||
        parsedWindowMinutes > 10080
      ) {
        toast.error(t('conditionWindowInvalid'));
        return;
      }
    }
    if (
      form.emailChannelId !== 'none' &&
      form.receiverUserIds.length === 0 &&
      (currentEmailChannel?.config?.email?.recipients?.length || 0) === 0
    ) {
      toast.error(t('receiverRequired'));
      return;
    }

    const payload = buildPolicyPayload(form, selectedTemplate);
    if (
      form.strategyMode === 'static' &&
      selectedTemplate?.legacy_rule_key &&
      payload.cluster_id === 'all'
    ) {
      toast.error(t('concreteClusterRequired'));
      return;
    }

    setSavingPolicy(true);
    try {
      const result =
        editingPolicyId === null
          ? await services.monitoring.createAlertPolicySafe(payload)
          : await services.monitoring.updateAlertPolicySafe(
              editingPolicyId,
              payload,
            );
      if (!result.success) {
        toast.error(
          result.error ||
            (editingPolicyId === null ? t('createError') : t('updateError')),
        );
        return;
      }
      toast.success(
        editingPolicyId === null ? t('createSuccess') : t('updateSuccess'),
      );
      await loadResources();
      resetPolicyForm();
      setPolicyDialogOpen(false);
    } finally {
      setSavingPolicy(false);
    }
  };

  const openNewEmailChannelDialog = () => {
    requestEmailChannelDiscard(() => {
      const nextForm = createDefaultEmailChannelForm();
      applyEmailChannelFormSnapshot(nextForm);
      setShowEmailPassword(false);
      setEmailTestReceiver(defaultEmailTestReceiver);
      setLastEmailChannelConnectionResult(null);
      setLastEmailChannelConnectionSnapshot(null);
      setLastEmailChannelTestResult(null);
      setLastEmailChannelTestSnapshot(null);
      setEmailDialogOpen(true);
    });
  };

  const handleEditEmailChannel = useCallback(
    (channel: NotificationChannel) => {
      requestEmailChannelDiscard(() => {
        const nextForm = createEmailChannelFormFromChannel(channel);
        applyEmailChannelFormSnapshot(nextForm);
        setShowEmailPassword(false);
        setEmailTestReceiver(defaultEmailTestReceiver);
        setLastEmailChannelConnectionResult(null);
        setLastEmailChannelConnectionSnapshot(null);
        setLastEmailChannelTestResult(null);
        setLastEmailChannelTestSnapshot(null);
        setEmailDialogOpen(true);
      });
    },
    [
      applyEmailChannelFormSnapshot,
      defaultEmailTestReceiver,
      requestEmailChannelDiscard,
    ],
  );

  const handleSaveEmailChannel = async (enabledOverride?: boolean) => {
    if (!emailChannelForm.name.trim()) {
      toast.error(t('channel.nameRequired'));
      return;
    }

    const payload = buildEmailChannelPayload(emailChannelForm, enabledOverride);

    setSavingEmailChannel(true);
    try {
      const result =
        emailChannelForm.id === null
          ? await services.monitoring.createNotificationChannelSafe(payload)
          : await services.monitoring.updateNotificationChannelSafe(
              emailChannelForm.id,
              payload,
            );
      if (!result.success || !result.data) {
        toast.error(
          result.error ||
            (emailChannelForm.id === null
              ? t('channel.createError')
              : t('channel.updateError')),
        );
        return;
      }
      toast.success(
        emailChannelForm.id === null
          ? t('channel.createSuccess')
          : t('channel.updateSuccess'),
      );
      const savedChannel = result.data;
      await loadResources();
      const nextForm = createEmailChannelFormFromChannel(savedChannel);
      applyEmailChannelFormSnapshot(nextForm);
      setLastEmailChannelConnectionResult(null);
      setLastEmailChannelConnectionSnapshot(null);
      setLastEmailChannelTestResult(null);
      setLastEmailChannelTestSnapshot(null);
      setShowEmailPassword(false);
      setEmailTestReceiver(defaultEmailTestReceiver);
      setForm((prev) => ({
        ...prev,
        emailChannelId: String(savedChannel.id),
      }));
    } finally {
      setSavingEmailChannel(false);
    }
  };

  const handleTestEmailChannel = async () => {
    const explicitReceiverUserId =
      emailTestReceiver !== 'none' ? Number.parseInt(emailTestReceiver, 10) : 0;
    const fallbackRecipients = parseRecipients(emailChannelForm.recipients);

    if (explicitReceiverUserId <= 0 && fallbackRecipients.length === 0) {
      toast.error(
        currentUser && !currentUserNotifiableRecipient
          ? t('channel.testReceiverCurrentUserMissingEmail')
          : t('channel.testReceiverRequired'),
      );
      return;
    }

    const isDraftTest = emailChannelForm.id === null || emailChannelDirty;
    const testingMarker = isDraftTest ? 0 : emailChannelForm.id;
    setTestingEmailChannelId(testingMarker);
    try {
      const result = isDraftTest
        ? await services.monitoring.testNotificationChannelDraftSafe({
            channel: buildEmailChannelPayload(emailChannelForm),
            receiver_user_id:
              explicitReceiverUserId > 0 ? explicitReceiverUserId : undefined,
          })
        : await services.monitoring.testNotificationChannelSafe(
            emailChannelForm.id as number,
            explicitReceiverUserId > 0
              ? {receiver_user_id: explicitReceiverUserId}
              : undefined,
          );
      if (!result.success || !result.data) {
        toast.error(result.error || t('channel.testError'));
        return;
      }
      setLastEmailChannelTestResult(result.data);
      setLastEmailChannelTestSnapshot(emailChannelSerialized);
      if (result.data.status === 'sent') {
        toast.success(t('channel.testSuccess'));
      } else {
        toast.error(result.data.last_error || t('channel.testError'));
      }
    } finally {
      setTestingEmailChannelId(null);
    }
  };

  const handleTestEmailConnection = async () => {
    const payload = buildEmailChannelPayload(emailChannelForm);
    setTestingEmailConnection(true);
    try {
      const result =
        await services.monitoring.testNotificationChannelConnectionSafe(
          payload,
        );
      if (!result.success || !result.data) {
        toast.error(result.error || t('channel.connectionTestError'));
        return;
      }
      setLastEmailChannelConnectionResult(result.data);
      setLastEmailChannelConnectionSnapshot(emailChannelSerialized);
      if (result.data.status === 'sent') {
        toast.success(t('channel.connectionTestSuccess'));
      } else {
        toast.error(result.data.last_error || t('channel.connectionTestError'));
      }
    } finally {
      setTestingEmailConnection(false);
    }
  };

  const handleDeleteEmailChannel = async () => {
    if (emailChannelForm.id === null) {
      return;
    }
    if (!window.confirm(t('channel.deleteConfirm'))) {
      return;
    }

    const deletingId = emailChannelForm.id;
    const result =
      await services.monitoring.deleteNotificationChannelSafe(deletingId);
    if (!result.success) {
      toast.error(result.error || t('channel.deleteError'));
      return;
    }

    toast.success(t('channel.deleteSuccess'));
    await loadResources();
    const nextForm = createDefaultEmailChannelForm();
    applyEmailChannelFormSnapshot(nextForm);
    setLastEmailChannelConnectionResult(null);
    setLastEmailChannelConnectionSnapshot(null);
    setLastEmailChannelTestResult(null);
    setLastEmailChannelTestSnapshot(null);
    setEmailTestReceiver(defaultEmailTestReceiver);
    setShowEmailPassword(false);
    setForm((prev) => ({
      ...prev,
      emailChannelId:
        prev.emailChannelId === String(deletingId)
          ? 'none'
          : prev.emailChannelId,
    }));
  };

  const openNewWebhookChannelDialog = useCallback(() => {
    requestWebhookChannelDiscard(() => {
      const nextForm = createDefaultWebhookChannelForm();
      applyWebhookChannelFormSnapshot(nextForm);
      setShowWebhookSecret(false);
      setLastWebhookChannelTestResult(null);
      setLastWebhookChannelTestSnapshot(null);
      setWebhookDialogOpen(true);
    });
  }, [applyWebhookChannelFormSnapshot, requestWebhookChannelDiscard]);

  const handleEditWebhookChannel = useCallback(
    (channel: NotificationChannel) => {
      requestWebhookChannelDiscard(() => {
        const nextForm = createWebhookChannelFormFromChannel(channel);
        applyWebhookChannelFormSnapshot(nextForm);
        setShowWebhookSecret(false);
        setLastWebhookChannelTestResult(null);
        setLastWebhookChannelTestSnapshot(null);
        setWebhookDialogOpen(true);
      });
    },
    [applyWebhookChannelFormSnapshot, requestWebhookChannelDiscard],
  );

  const handleSaveWebhookChannel = async (enabledOverride?: boolean) => {
    if (!webhookChannelForm.name.trim()) {
      toast.error(t('webhook.nameRequired'));
      return;
    }
    if (!webhookChannelForm.endpoint.trim()) {
      toast.error(t('webhook.endpointRequired'));
      return;
    }

    const payload = buildWebhookChannelPayload(
      webhookChannelForm,
      enabledOverride,
    );

    setSavingWebhookChannel(true);
    try {
      const result =
        webhookChannelForm.id === null
          ? await services.monitoring.createNotificationChannelSafe(payload)
          : await services.monitoring.updateNotificationChannelSafe(
              webhookChannelForm.id,
              payload,
            );
      if (!result.success || !result.data) {
        toast.error(
          result.error ||
            (webhookChannelForm.id === null
              ? t('webhook.createError')
              : t('webhook.updateError')),
        );
        return;
      }
      toast.success(
        webhookChannelForm.id === null
          ? t('webhook.createSuccess')
          : t('webhook.updateSuccess'),
      );
      const savedChannel = result.data;
      await loadResources();
      applyWebhookChannelFormSnapshot(
        createWebhookChannelFormFromChannel(savedChannel),
      );
      setShowWebhookSecret(false);
      setLastWebhookChannelTestResult(null);
      setLastWebhookChannelTestSnapshot(null);
      setForm((prev) => ({
        ...prev,
        webhookChannelId: String(savedChannel.id),
      }));
    } finally {
      setSavingWebhookChannel(false);
    }
  };

  const handleTestWebhookChannel = async () => {
    if (!webhookChannelForm.name.trim()) {
      toast.error(t('webhook.nameRequired'));
      return;
    }
    if (!webhookChannelForm.endpoint.trim()) {
      toast.error(t('webhook.endpointRequired'));
      return;
    }

    const isDraftTest = webhookChannelForm.id === null || webhookChannelDirty;
    const testingMarker = isDraftTest ? 0 : webhookChannelForm.id;
    setTestingWebhookChannelId(testingMarker);
    try {
      const result = isDraftTest
        ? await services.monitoring.testNotificationChannelDraftSafe({
            channel: buildWebhookChannelPayload(webhookChannelForm),
          })
        : await services.monitoring.testNotificationChannelSafe(
            webhookChannelForm.id as number,
          );
      if (!result.success || !result.data) {
        toast.error(result.error || t('webhook.testError'));
        return;
      }
      setLastWebhookChannelTestResult(result.data);
      setLastWebhookChannelTestSnapshot(webhookChannelSerialized);
      if (result.data.status === 'sent') {
        toast.success(t('webhook.testSuccess'));
      } else {
        toast.error(result.data.last_error || t('webhook.testError'));
      }
    } finally {
      setTestingWebhookChannelId(null);
    }
  };

  const handleDeleteWebhookChannel = async () => {
    if (webhookChannelForm.id === null) {
      return;
    }
    if (!window.confirm(t('webhook.deleteConfirm'))) {
      return;
    }

    const deletingID = webhookChannelForm.id;
    const result =
      await services.monitoring.deleteNotificationChannelSafe(deletingID);
    if (!result.success) {
      toast.error(result.error || t('webhook.deleteError'));
      return;
    }

    toast.success(t('webhook.deleteSuccess'));
    await loadResources();
    applyWebhookChannelFormSnapshot(createDefaultWebhookChannelForm());
    setShowWebhookSecret(false);
    setLastWebhookChannelTestResult(null);
    setLastWebhookChannelTestSnapshot(null);
    setForm((prev) => ({
      ...prev,
      webhookChannelId:
        prev.webhookChannelId === String(deletingID)
          ? 'none'
          : prev.webhookChannelId,
    }));
  };

  const emailFallbackRecipients = useMemo(
    () => parseRecipients(emailChannelForm.recipients),
    [emailChannelForm.recipients],
  );

  // 多条件过滤后的规则列表
  // Filtered rule rows based on keyword, cluster, and status
  const policyRows = useMemo(() => {
    return policies.filter((policy) => {
      if (
        policyClusterFilter !== 'all' &&
        policy.cluster_id !== policyClusterFilter
      ) {
        return false;
      }
      if (policyStatusFilter === 'enabled' && !policy.enabled) {
        return false;
      }
      if (policyStatusFilter === 'disabled' && policy.enabled) {
        return false;
      }
      const kw = policySearchKeyword.trim().toLowerCase();
      if (kw) {
        const name = (policy.name || '').toLowerCase();
        const desc = (policy.description || '').toLowerCase();
        const tmpl = (policy.template_key || '').toLowerCase();
        return name.includes(kw) || desc.includes(kw) || tmpl.includes(kw);
      }
      return true;
    });
  }, [policies, policyClusterFilter, policySearchKeyword, policyStatusFilter]);

  // 规则段 StatPills：全部 / 已启用 / 已停用（客户端聚合）
  // Rules-section StatPills: all / enabled / disabled (client aggregated)
  const rulePillItems = useMemo<StatPillItem[]>(() => {
    const enabledCount = policies.filter((p) => p.enabled).length;
    const disabledCount = policies.length - enabledCount;
    return [
      {key: 'all', label: t('statPills.all'), count: policies.length},
      {
        key: 'enabled',
        label: t('statPills.enabled'),
        count: enabledCount,
        variant: 'success',
      },
      {
        key: 'disabled',
        label: t('statPills.disabled'),
        count: disabledCount,
      },
    ];
  }, [policies, t]);

  // 通道列表：邮件 + Webhook 合并，按 pill / 关键字筛选
  // Channel list: merge email + webhook, filter by pill / keyword
  const channelRows = useMemo(() => {
    let rows = channels;
    if (channelPillFilter === 'email') {
      rows = rows.filter((c) => c.type === 'email');
    } else if (channelPillFilter === 'webhook') {
      rows = rows.filter((c) => c.type === 'webhook');
    } else if (channelPillFilter === 'enabled') {
      rows = rows.filter((c) => c.enabled);
    }
    const kw = channelListSearch.trim().toLowerCase();
    if (!kw) {
      return rows;
    }
    return rows.filter((channel) => {
      const config =
        channel.type === 'email' ? getEmailConfig(channel) : null;
      return [
        channel.name,
        channel.description,
        channel.endpoint,
        config?.from,
        config?.host,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(kw));
    });
  }, [channelListSearch, channelPillFilter, channels]);

  const channelPillItems = useMemo<StatPillItem[]>(() => {
    const emailCount = channels.filter((c) => c.type === 'email').length;
    const webhookCount = channels.filter((c) => c.type === 'webhook').length;
    const enabledCount = channels.filter((c) => c.enabled).length;
    return [
      {key: 'all', label: t('statPills.all'), count: channels.length},
      {key: 'email', label: t('statPills.email'), count: emailCount, icon: <Mail className='h-3 w-3' />},
      {key: 'webhook', label: t('statPills.webhook'), count: webhookCount, icon: <Webhook className='h-3 w-3' />},
      {
        key: 'enabled',
        label: t('statPills.enabled'),
        count: enabledCount,
        variant: 'success',
      },
    ];
  }, [channels, t]);

  // 投递记录：状态 pill + 关键字客户端过滤
  // Delivery rows: client-side status pill + keyword filter
  const historyRows = useMemo(() => {
    const deliveries = Array.isArray(historyData.deliveries)
      ? historyData.deliveries
      : [];
    let rows = deliveries;
    if (historyStatusFilter === 'sent') {
      rows = rows.filter((d) => d.status === 'sent');
    } else if (historyStatusFilter === 'failed') {
      rows = rows.filter((d) => d.status === 'failed');
    }
    const kw = historySearchKeyword.trim().toLowerCase();
    if (!kw) {
      return rows;
    }
    return rows.filter((delivery) =>
      [
        delivery.alert_name,
        delivery.channel_name,
        delivery.last_error,
        delivery.cluster_name,
      ]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(kw)),
    );
  }, [historyData.deliveries, historySearchKeyword, historyStatusFilter]);

  const historyPillItems = useMemo<StatPillItem[]>(() => {
    const deliveries = Array.isArray(historyData.deliveries)
      ? historyData.deliveries
      : [];
    // pill 计数基于当前已载入结果集（与告警事件页一致，不假装全局精确）
    // Pill counts reflect the currently loaded result set (same as alerts page)
    const sentCount = deliveries.filter((d) => d.status === 'sent').length;
    const failedCount = deliveries.filter((d) => d.status === 'failed').length;
    return [
      {
        key: 'all',
        label: t('statPills.all'),
        count: deliveries.length,
      },
      {
        key: 'sent',
        label: t('statPills.sent'),
        count: sentCount,
        variant: 'success',
      },
      {
        key: 'failed',
        label: t('statPills.failed'),
        count: failedCount,
        variant: 'danger',
      },
    ];
  }, [historyData.deliveries, t]);

  const policyNameById = useMemo(() => {
    const map = new Map<number, string>();
    for (const policy of policies) {
      map.set(policy.id, policy.name);
    }
    return map;
  }, [policies]);

  return (
    <div className='flex-1 flex flex-col space-y-3'>
      {/* 三级分段：告警规则 | 通知通道 | 投递记录 / Tertiary sections */}
      <Tabs
        value={activeSection}
        onValueChange={(value) => setActiveSection(value as PolicySection)}
        className='w-full'
      >
        <TabsList className='grid w-full grid-cols-3 sm:w-[420px] bg-muted/60 p-1 h-8.5'>
          <TabsTrigger
            value='rules'
            className='data-[state=active]:bg-background data-[state=active]:shadow-xs text-xs font-medium py-1'
          >
            {t('sections.rules')}
          </TabsTrigger>
          <TabsTrigger
            value='channels'
            className='data-[state=active]:bg-background data-[state=active]:shadow-xs text-xs font-medium py-1'
          >
            {t('sections.channels')}
          </TabsTrigger>
          <TabsTrigger
            value='history'
            className='data-[state=active]:bg-background data-[state=active]:shadow-xs text-xs font-medium py-1'
          >
            {t('sections.history')}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {/* ===== 告警规则段 / Alert Rules Section ===== */}
      {activeSection === 'rules' ? (
        <Card className='border-border/70 shadow-xs overflow-hidden flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-300px)]'>
          <div className='p-3 sm:p-3.5 border-b bg-muted/20 space-y-2.5'>
            <StatPillsBar
              items={rulePillItems}
              activeKey={policyStatusFilter}
              onChange={(key) => {
                setPolicyStatusFilter(key);
              }}
              actions={
                <div className='flex items-center gap-1.5'>
                  <Button
                    variant='ghost'
                    size='sm'
                    onClick={() => {
                      setPolicySearchKeyword('');
                      setPolicyClusterFilter('all');
                      setPolicyStatusFilter('all');
                    }}
                    className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground cursor-pointer'
                  >
                    <RotateCcw className='mr-1 h-3 w-3' />
                    {rootT('alerts.reset')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={handleRefresh}
                    disabled={loading}
                    className='h-7 px-2.5 text-xs cursor-pointer'
                  >
                    <RefreshCw
                      className={cn('mr-1.5 h-3 w-3', loading && 'animate-spin')}
                    />
                    {rootT('refresh')}
                  </Button>
                  <Button
                    size='sm'
                    onClick={() => {
                      resetPolicyForm();
                      setPolicyDialogOpen(true);
                    }}
                    className='h-7 px-2.5 text-xs font-medium shadow-xs cursor-pointer'
                  >
                    <Plus className='mr-1 h-3 w-3' />
                    {t('createNew')}
                  </Button>
                </div>
              }
            />

            {/* 筛选栏左对齐 / Left-aligned filter bar */}
            <div className='flex flex-wrap items-center gap-2 pt-0.5'>
              <div className='relative flex-1 min-w-[200px] max-w-sm'>
                <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground' />
                <Input
                  value={policySearchKeyword}
                  onChange={(e) => setPolicySearchKeyword(e.target.value)}
                  placeholder={t('searchPlaceholder')}
                  className='pl-8 pr-7 h-8 text-xs bg-background'
                />
                {policySearchKeyword ? (
                  <button
                    type='button'
                    onClick={() => setPolicySearchKeyword('')}
                    className='absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
                  >
                    <X className='h-3.5 w-3.5' />
                  </button>
                ) : null}
              </div>

              <Select
                value={policyClusterFilter}
                onValueChange={setPolicyClusterFilter}
              >
                <SelectTrigger className='h-8 w-[145px] text-xs bg-background'>
                  <SelectValue placeholder={t('fields.cluster')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='all'>{t('allClusters')}</SelectItem>
                  {clusters.map((c) => (
                    <SelectItem key={c.id} value={String(c.id)}>
                      {c.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <TableLoadingBar loading={loading && policyRows.length > 0} />
          <div className='overflow-x-auto flex-1'>
            <Table>
              <TableHeader>
                <TableRow className='bg-muted/30 hover:bg-muted/30 h-8'>
                  <TableHead className='min-w-[180px] py-1.5 px-3 text-xs'>{t('columns.name')}</TableHead>
                  <TableHead className='min-w-[170px] py-1.5 px-3 text-xs'>{t('columns.template')}</TableHead>
                  <TableHead className='w-[130px] py-1.5 px-3 text-xs'>{t('columns.cluster')}</TableHead>
                  <TableHead className='w-[100px] py-1.5 px-3 text-xs'>{t('columns.severity')}</TableHead>
                  <TableHead className='min-w-[160px] py-1.5 px-3 text-xs'>{t('columns.methods')}</TableHead>
                  <TableHead className='w-[110px] py-1.5 px-3 text-xs'>{t('columns.status')}</TableHead>
                  <TableHead className='w-[90px] text-center py-1.5 px-3 text-xs'>{t('columns.enabled')}</TableHead>
                  <TableHead className='w-[150px] whitespace-nowrap py-1.5 px-3 text-xs'>{t('columns.updatedAt')}</TableHead>
                  <TableHead className='w-[120px] text-right py-1.5 px-3 text-xs'>{rootT('actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading && policyRows.length === 0 ? (
                  <TableSkeletonRows columns={9} rows={6} />
                ) : policyRows.length === 0 ? (
                  <TableRow>
                    <TableCell
                      colSpan={9}
                      className='h-40 text-center text-muted-foreground'
                    >
                      <div className='flex flex-col items-center justify-center gap-2'>
                        <BellRing className='h-7 w-7 text-muted-foreground/40' />
                        <span className='font-medium'>{t('emptyPolicies')}</span>
                        <Button
                          variant='outline'
                          size='sm'
                          className='mt-1 text-xs'
                          onClick={() => {
                            resetPolicyForm();
                            setPolicyDialogOpen(true);
                          }}
                        >
                          <Plus className='mr-1.5 h-3.5 w-3.5' />
                          {t('createFirst')}
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  policyRows.map((policy) => {
                    const template = availableStaticTemplates.find(
                      (item) => item.key === policy.template_key,
                    );
                    const cluster = clusters.find(
                      (item) => String(item.id) === policy.cluster_id,
                    );
                    return (
                      <TableRow
                        key={policy.id}
                        className={cn(
                          'transition-colors',
                          loading && 'opacity-60 pointer-events-none',
                        )}
                      >
                        <TableCell className='py-2 px-3 font-medium'>
                          <div className='space-y-0.5'>
                            <div className='text-sm text-foreground'>
                              {policy.name}
                            </div>
                            {policy.description ? (
                              <div className='text-xs text-muted-foreground line-clamp-1 max-w-[240px]'>
                                {policy.description}
                              </div>
                            ) : null}
                          </div>
                        </TableCell>
                        <TableCell className='py-2 px-3'>
                          <span className='inline-flex items-center rounded-md bg-muted px-2 py-0.5 text-xs font-medium'>
                            {policy.policy_type === 'custom_promql'
                              ? t('customPromql')
                              : template
                                ? getTemplateDisplayName(template, legacyT)
                                : getTemplateTranslation(
                                    legacyT,
                                    policy.template_key || '',
                                    'name',
                                  ) ||
                                  policy.template_key ||
                                  '-'}
                          </span>
                        </TableCell>
                        <TableCell className='py-2 px-3'>
                          <span className='text-xs font-medium'>
                            {cluster?.name || policy.cluster_id || '-'}
                          </span>
                        </TableCell>
                        <TableCell className='py-2 px-3'>
                          <Badge
                            variant={resolveSeverityVariant(policy.severity)}
                            className='text-[11px]'
                          >
                            {policy.severity === 'critical'
                              ? rootT('alertSeverity.critical')
                              : rootT('alertSeverity.warning')}
                          </Badge>
                        </TableCell>
                        <TableCell className='py-2 px-3 max-w-[220px]'>
                          <span className='text-xs text-muted-foreground'>
                            {notificationMethodSummary(policy, channelMap)}
                          </span>
                        </TableCell>
                        <TableCell className='py-2 px-3'>
                          <Badge
                            variant={resolveDeliveryStatusVariant(
                              policy.last_execution_status,
                            )}
                            className='text-[11px]'
                          >
                            {getPolicyExecutionStatusLabel(
                              legacyT,
                              policy.last_execution_status,
                            )}
                          </Badge>
                        </TableCell>
                        <TableCell className='py-2 px-3 text-center'>
                          <Switch
                            checked={policy.enabled}
                            onCheckedChange={(checked) =>
                              void handleTogglePolicyEnabled(policy, checked)
                            }
                          />
                        </TableCell>
                        <TableCell className='py-2 px-3 text-xs font-mono text-muted-foreground whitespace-nowrap'>
                          {formatDateTime(policy.updated_at)}
                        </TableCell>
                        <TableCell className='py-2 px-3 text-right whitespace-nowrap'>
                          <div className='flex items-center justify-end gap-1'>
                            <Button
                              variant='ghost'
                              size='icon'
                              className='h-7 w-7'
                              onClick={() => openPolicyDeliveryHistory(policy)}
                              title={t('actions.viewHistory')}
                            >
                              <History className='h-3.5 w-3.5' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon'
                              className='h-7 w-7'
                              onClick={() => handleEditPolicy(policy)}
                              title={t('actions.editPolicy')}
                            >
                              <Pencil className='h-3.5 w-3.5' />
                            </Button>
                            <Button
                              variant='ghost'
                              size='icon'
                              className='h-7 w-7 text-destructive hover:text-destructive'
                              onClick={() => void handleDeletePolicy(policy.id)}
                              disabled={deletingPolicyId === policy.id}
                              title={t('actions.deletePolicy')}
                            >
                              <Trash2 className='h-3.5 w-3.5' />
                            </Button>
                          </div>
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </div>
        </Card>
      ) : null}

      {/* ===== 通知通道段 / Notification Channels Section ===== */}
      {activeSection === 'channels' ? (
        <Card className='border-border/70 shadow-xs overflow-hidden flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-300px)]'>
          <div className='p-3 sm:p-3.5 border-b bg-muted/20 space-y-2.5'>
            <StatPillsBar
              items={channelPillItems}
              activeKey={channelPillFilter}
              onChange={(key) => setChannelPillFilter(key as ChannelPillFilter)}
              actions={
                <div className='flex items-center gap-1.5'>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={handleRefresh}
                    disabled={loading}
                    className='h-7 px-2.5 text-xs cursor-pointer'
                  >
                    <RefreshCw
                      className={cn('mr-1.5 h-3 w-3', loading && 'animate-spin')}
                    />
                    {rootT('refresh')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={openNewEmailChannelDialog}
                    className='h-7 px-2.5 text-xs cursor-pointer'
                  >
                    <Mail className='mr-1 h-3 w-3' />
                    {t('channelList.createEmail')}
                  </Button>
                  <Button
                    size='sm'
                    onClick={openNewWebhookChannelDialog}
                    className='h-7 px-2.5 text-xs font-medium shadow-xs cursor-pointer'
                  >
                    <Webhook className='mr-1 h-3 w-3' />
                    {t('channelList.createWebhook')}
                  </Button>
                </div>
              }
            />
            <div className='flex flex-wrap items-center gap-2 pt-0.5'>
              <div className='relative flex-1 min-w-[200px] max-w-sm'>
                <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground' />
                <Input
                  value={channelListSearch}
                  onChange={(e) => setChannelListSearch(e.target.value)}
                  placeholder={t('channelList.searchPlaceholder')}
                  className='pl-8 pr-7 h-8 text-xs bg-background'
                />
                {channelListSearch ? (
                  <button
                    type='button'
                    onClick={() => setChannelListSearch('')}
                    className='absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
                  >
                    <X className='h-3.5 w-3.5' />
                  </button>
                ) : null}
              </div>
            </div>
          </div>

          <TableLoadingBar loading={loading && channelRows.length > 0} />
          <div className='overflow-x-auto flex-1'>
            <Table>
              <TableHeader>
                <TableRow className='bg-muted/30 hover:bg-muted/30 h-8'>
                  <TableHead className='w-[100px] py-1.5 px-3 text-xs'>{t('channelList.columns.type')}</TableHead>
                  <TableHead className='min-w-[160px] py-1.5 px-3 text-xs'>{t('channelList.columns.name')}</TableHead>
                  <TableHead className='min-w-[220px] py-1.5 px-3 text-xs'>{t('channelList.columns.target')}</TableHead>
                  <TableHead className='w-[100px] py-1.5 px-3 text-xs'>{t('channelList.columns.status')}</TableHead>
                  <TableHead className='w-[150px] whitespace-nowrap py-1.5 px-3 text-xs'>{t('channelList.columns.updatedAt')}</TableHead>
                  <TableHead className='w-[100px] text-right py-1.5 px-3 text-xs'>{rootT('actions')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {loading && channelRows.length === 0 ? (
                  <TableSkeletonRows columns={6} rows={6} />
                ) : channelRows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={6} className='h-40 text-center text-muted-foreground'>
                      <div className='flex flex-col items-center justify-center gap-2'>
                        <Mail className='h-7 w-7 text-muted-foreground/40' />
                        <span className='font-medium'>{t('channelList.empty')}</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  channelRows.map((channel) => {
                    const emailConfig =
                      channel.type === 'email' ? getEmailConfig(channel) : null;
                    const target =
                      channel.type === 'email'
                        ? emailConfig?.from ||
                          `${emailConfig?.host || '-'}:${emailConfig?.port || '-'}`
                        : channel.endpoint || '-';
                    return (
                      <TableRow
                        key={channel.id}
                        className={cn(
                          'transition-colors',
                          loading && 'opacity-60 pointer-events-none',
                        )}
                      >
                        <TableCell className='py-2 px-3'>
                          <Badge variant='outline' className='text-[10px] py-0 px-1.5 font-normal gap-1'>
                            {channel.type === 'email' ? (
                              <>
                                <Mail className='h-3 w-3' />
                                {t('channelList.typeEmail')}
                              </>
                            ) : (
                              <>
                                <Webhook className='h-3 w-3' />
                                {t('channelList.typeWebhook')}
                              </>
                            )}
                          </Badge>
                        </TableCell>
                        <TableCell className='py-2 px-3'>
                          <div className='space-y-0.5'>
                            <div className='text-sm font-medium'>{channel.name}</div>
                            {channel.description ? (
                              <div className='text-xs text-muted-foreground line-clamp-1 max-w-[240px]'>
                                {channel.description}
                              </div>
                            ) : null}
                          </div>
                        </TableCell>
                        <TableCell className='py-2 px-3 text-xs text-muted-foreground font-mono truncate max-w-[280px]' title={target}>
                          {target}
                        </TableCell>
                        <TableCell className='py-2 px-3'>
                          <Badge variant={channel.enabled ? 'default' : 'secondary'} className='text-[11px]'>
                            {channel.enabled
                              ? t('channel.statusEnabled')
                              : t('channel.statusDraft')}
                          </Badge>
                        </TableCell>
                        <TableCell className='py-2 px-3 text-xs font-mono text-muted-foreground whitespace-nowrap'>
                          {formatDateTime(channel.updated_at)}
                        </TableCell>
                        <TableCell className='py-2 px-3 text-right'>
                          <Button
                            variant='ghost'
                            size='icon'
                            className='h-7 w-7'
                            title={t('channelList.actions.edit')}
                            onClick={() => {
                              if (channel.type === 'email') {
                                handleEditEmailChannel(channel);
                              } else {
                                handleEditWebhookChannel(channel);
                              }
                            }}
                          >
                            <Pencil className='h-3.5 w-3.5' />
                          </Button>
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </div>
        </Card>
      ) : null}

      {/* ===== 投递记录段 / Delivery History Section ===== */}
      {activeSection === 'history' ? (
        <Card className='border-border/70 shadow-xs overflow-hidden flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-300px)]'>
          <div className='p-3 sm:p-3.5 border-b bg-muted/20 space-y-2.5'>
            <StatPillsBar
              items={historyPillItems}
              activeKey={historyStatusFilter}
              onChange={(key) => setHistoryStatusFilter(key as HistoryPillFilter)}
              actions={
                <div className='flex items-center gap-1.5'>
                  <Button
                    variant='ghost'
                    size='sm'
                    onClick={() => {
                      setHistoryStatusFilter('all');
                      setHistoryPolicyIdFilter(null);
                      setHistorySearchKeyword('');
                    }}
                    className='h-7 px-2 text-xs text-muted-foreground hover:text-foreground cursor-pointer'
                  >
                    <RotateCcw className='mr-1 h-3 w-3' />
                    {t('deliveryHistory.resetFilters')}
                  </Button>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => void loadDeliveryHistory()}
                    disabled={historyLoading}
                    className='h-7 px-2.5 text-xs cursor-pointer'
                  >
                    <RefreshCw
                      className={cn(
                        'mr-1.5 h-3 w-3',
                        historyLoading && 'animate-spin',
                      )}
                    />
                    {rootT('refresh')}
                  </Button>
                </div>
              }
            />
            <div className='flex flex-wrap items-center gap-2 pt-0.5'>
              <div className='relative flex-1 min-w-[200px] max-w-sm'>
                <Search className='absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground' />
                <Input
                  value={historySearchKeyword}
                  onChange={(e) => setHistorySearchKeyword(e.target.value)}
                  placeholder={t('deliveryHistory.searchPlaceholder')}
                  className='pl-8 pr-7 h-8 text-xs bg-background'
                />
                {historySearchKeyword ? (
                  <button
                    type='button'
                    onClick={() => setHistorySearchKeyword('')}
                    className='absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground'
                  >
                    <X className='h-3.5 w-3.5' />
                  </button>
                ) : null}
              </div>
              <Select
                value={
                  historyPolicyIdFilter === null
                    ? 'all'
                    : String(historyPolicyIdFilter)
                }
                onValueChange={(val) => {
                  setHistoryPolicyIdFilter(val === 'all' ? null : Number(val));
                }}
              >
                <SelectTrigger className='h-8 w-[180px] text-xs bg-background'>
                  <SelectValue placeholder={t('deliveryHistory.policyFilter')} />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='all'>
                    {t('deliveryHistory.allPolicies')}
                  </SelectItem>
                  {policies.map((policy) => (
                    <SelectItem key={policy.id} value={String(policy.id)}>
                      {policy.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>

          <TableLoadingBar loading={historyLoading && historyRows.length > 0} />
          <div className='overflow-x-auto flex-1'>
            <Table>
              <TableHeader>
                <TableRow className='bg-muted/30 hover:bg-muted/30 h-8'>
                  <TableHead className='min-w-[140px] py-1.5 px-3 text-xs'>{t('deliveryHistory.columns.policy')}</TableHead>
                  <TableHead className='min-w-[130px] py-1.5 px-3 text-xs'>{t('deliveryHistory.columns.channel')}</TableHead>
                  <TableHead className='w-[90px] py-1.5 px-3 text-xs'>{t('deliveryHistory.columns.event')}</TableHead>
                  <TableHead className='w-[100px] py-1.5 px-3 text-xs'>{t('deliveryHistory.columns.status')}</TableHead>
                  <TableHead className='w-[90px] py-1.5 px-3 text-xs'>{t('deliveryHistory.columns.responseCode')}</TableHead>
                  <TableHead className='w-[150px] whitespace-nowrap py-1.5 px-3 text-xs'>{t('deliveryHistory.columns.sentAt')}</TableHead>
                  <TableHead className='min-w-[180px] py-1.5 px-3 text-xs'>{t('deliveryHistory.columns.error')}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {historyLoading && historyRows.length === 0 ? (
                  <TableSkeletonRows columns={7} rows={6} />
                ) : historyRows.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} className='h-40 text-center text-muted-foreground'>
                      <div className='flex flex-col items-center justify-center gap-2'>
                        <History className='h-7 w-7 text-muted-foreground/40' />
                        <span className='font-medium'>{t('deliveryHistory.empty')}</span>
                      </div>
                    </TableCell>
                  </TableRow>
                ) : (
                  historyRows.map((delivery: NotificationDelivery) => (
                    <TableRow
                      key={delivery.id}
                      className={cn(
                        'transition-colors',
                        historyLoading && 'opacity-60 pointer-events-none',
                      )}
                    >
                      <TableCell className='py-2 px-3 text-xs font-medium'>
                        {policyNameById.get(delivery.policy_id) ||
                          delivery.alert_name ||
                          '-'}
                      </TableCell>
                      <TableCell className='py-2 px-3 text-xs'>
                        {delivery.channel_name || '-'}
                      </TableCell>
                      <TableCell className='py-2 px-3 text-xs'>
                        {delivery.event_type === 'resolved'
                          ? t('deliveryHistory.events.resolved')
                          : delivery.event_type === 'test'
                            ? t('deliveryHistory.events.test')
                            : t('deliveryHistory.events.firing')}
                      </TableCell>
                      <TableCell className='py-2 px-3'>
                        <Badge
                          variant={resolveDeliveryStatusVariant(delivery.status)}
                          className='text-[11px]'
                        >
                          {delivery.status === 'sent'
                            ? t('deliveryHistory.statuses.sent')
                            : delivery.status === 'failed'
                              ? t('deliveryHistory.statuses.failed')
                              : delivery.status === 'sending'
                                ? t('deliveryHistory.statuses.sending')
                                : delivery.status === 'retrying'
                                  ? t('deliveryHistory.statuses.retrying')
                                  : delivery.status === 'canceled'
                                    ? t('deliveryHistory.statuses.canceled')
                                    : t('deliveryHistory.statuses.pending')}
                        </Badge>
                      </TableCell>
                      <TableCell className='py-2 px-3 text-xs font-mono'>
                        {delivery.response_status_code || '-'}
                      </TableCell>
                      <TableCell className='py-2 px-3 text-xs font-mono text-muted-foreground whitespace-nowrap'>
                        {formatDateTime(delivery.sent_at)}
                      </TableCell>
                      <TableCell className='py-2 px-3 text-xs text-muted-foreground max-w-[260px] truncate' title={delivery.last_error || ''}>
                        {delivery.last_error || '-'}
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>
        </Card>
      ) : null}

      {/* 策略创建/编辑独立弹窗 (分层模块化设计) / Policy Editor Modal Dialog */}
      <Dialog
        open={policyDialogOpen}
        onOpenChange={(open) => {
          setPolicyDialogOpen(open);
          if (!open) {
            resetPolicyForm();
          }
        }}
      >
        <DialogContent className='flex h-[88vh] w-[95vw] max-w-[900px] flex-col overflow-hidden p-0 sm:max-w-[900px] border border-border/80 dark:border-border/60 shadow-2xl'>
          <DialogHeader className='px-6 py-4 border-b bg-muted/15'>
            <DialogTitle className='text-lg font-semibold'>
              {editingPolicyId === null
                ? t('editorCreateTitle')
                : t('editorEditTitle')}
            </DialogTitle>
            <DialogDescription className='text-xs text-muted-foreground'>
              {editingPolicyId === null
                ? t('dialogSubtitleCreate')
                : t('dialogSubtitleEdit')}
            </DialogDescription>
          </DialogHeader>

          {/* 表单内容滚动区 / Form Scroll Area */}
          <div className='flex-1 overflow-y-auto px-6 py-5 space-y-6'>
            {/* 第一部分：基本信息 */}
            <div className='space-y-4'>
              <h3 className='text-xs font-semibold text-muted-foreground uppercase tracking-wider'>
                {t('sectionBasic')}
              </h3>
              <div className='grid gap-4 md:grid-cols-2'>
                <div className='space-y-2 md:col-span-2'>
                  <Label>{t('fields.name')}</Label>
                  <Input
                    value={form.name}
                    onChange={(event) =>
                      setForm((prev) => ({...prev, name: event.target.value}))
                    }
                    placeholder={t('placeholders.name')}
                  />
                </div>

                <div className='space-y-2 md:col-span-2'>
                  <Label>{t('fields.description')}</Label>
                  <Textarea
                    value={form.description}
                    onChange={(event) =>
                      setForm((prev) => ({
                        ...prev,
                        description: event.target.value,
                      }))
                    }
                    placeholder={t('placeholders.description')}
                    rows={2}
                  />
                </div>

                <div className='space-y-2'>
                  <Label>{t('fields.cluster')}</Label>
                  <Select
                    value={form.clusterId}
                    onValueChange={(value) => {
                      setForm((prev) => ({...prev, clusterId: value}));
                      if (value && value !== 'all') {
                        rememberPreferredClusterId(value);
                      }
                    }}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder={t('clusterRequired')} />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='all'>{t('allClusters')}</SelectItem>
                      {clusters.map((cluster) => (
                        <SelectItem
                          key={cluster.id}
                          value={String(cluster.id)}
                        >
                          {cluster.name}
                          {isSoleDefaultCluster(clusters, cluster.id)
                            ? ` · ${rootT('defaultClusterBadge')}`
                            : ''}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className='space-y-2'>
                  <Label>{t('fields.severity')}</Label>
                  <Select
                    value={form.severity}
                    onValueChange={(value) =>
                      setForm((prev) => ({
                        ...prev,
                        severity: value as AlertSeverity,
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='warning'>
                        {rootT('alertSeverity.warning')}
                      </SelectItem>
                      <SelectItem value='critical'>
                        {rootT('alertSeverity.critical')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              </div>
            </div>

            <Separator />

            {/* 第二部分：规则与策略配置 */}
            <div className='space-y-4'>
              <h3 className='text-xs font-semibold text-muted-foreground uppercase tracking-wider'>
                {t('sectionRules')}
              </h3>

              <div className='space-y-2'>
                <Label>{t('fields.strategyType')}</Label>
                <div className='rounded-lg border px-4 py-3 text-sm bg-muted/20'>
                  <div className='font-medium'>
                    {form.strategyMode === 'custom_promql'
                      ? t('customPromqlLegacy')
                      : t('staticConfig')}
                  </div>
                  <div className='mt-1 text-xs text-muted-foreground'>
                    {form.strategyMode === 'custom_promql'
                      ? t('customPromqlLegacyDesc')
                      : t('staticConfigDesc')}
                  </div>
                </div>
              </div>

              {form.strategyMode === 'static' ? (
                <div className='space-y-4'>
                  <div className='space-y-2'>
                    <Label>{t('fields.template')}</Label>
                    <Select
                      value={form.templateKey}
                      onValueChange={(value) =>
                        setForm((prev) => {
                          const nextTemplate =
                            availableStaticTemplates.find(
                              (template) => template.key === value,
                            ) || null;
                          return {
                            ...prev,
                            templateKey: value,
                            ...buildConditionDefaults(nextTemplate),
                          };
                        })
                      }
                    >
                      <SelectTrigger>
                        <SelectValue placeholder={t('templateRequired')} />
                      </SelectTrigger>
                      <SelectContent>
                        {templateGroups.platformTemplates.length > 0 ? (
                          <>
                            <div className='px-2 py-1 text-xs font-medium text-muted-foreground'>
                              {t('groups.platformHealth')}
                            </div>
                            {templateGroups.platformTemplates.map(
                              (template) => (
                                <SelectItem
                                  key={template.key}
                                  value={template.key}
                                >
                                  {getTemplateDisplayName(template, legacyT)}
                                </SelectItem>
                              ),
                            )}
                          </>
                        ) : null}
                        {templateGroups.metricsTemplates.length > 0 ? (
                          <>
                            <div className='px-2 py-1 text-xs font-medium text-muted-foreground'>
                              {t('groups.prometheusMetrics')}
                            </div>
                            {templateGroups.metricsTemplates.map((template) => (
                              <SelectItem
                                key={template.key}
                                value={template.key}
                                disabled={!metricsTemplatesAvailable}
                              >
                                {getTemplateDisplayName(template, legacyT)}
                              </SelectItem>
                            ))}
                          </>
                        ) : null}
                      </SelectContent>
                    </Select>
                    {selectedTemplate ? (
                      <div className='space-y-1.5 pt-1'>
                        <p className='text-xs text-muted-foreground'>
                          {getTemplateDescription(selectedTemplate, legacyT)}
                        </p>
                        {selectedTemplate.required_signals &&
                        selectedTemplate.required_signals.length > 0 ? (
                          <div className='flex flex-wrap gap-1.5'>
                            {selectedTemplate.required_signals.map((signal) => (
                              <Badge
                                key={signal}
                                variant='outline'
                                className='font-mono text-[11px]'
                              >
                                {signal}
                              </Badge>
                            ))}
                          </div>
                        ) : null}
                      </div>
                    ) : null}
                  </div>

                  {selectedTemplateSupportsCondition ? (
                    <div className='space-y-3 rounded-lg border bg-muted/20 p-4'>
                      <div className='space-y-0.5'>
                        <Label>{t('fields.triggerCondition')}</Label>
                        <p className='text-xs text-muted-foreground'>
                          {t('conditionHint')}
                        </p>
                      </div>
                      <div className='grid gap-3 sm:grid-cols-3'>
                        <div className='space-y-1.5'>
                          <Label className='text-xs'>{t('fields.operator')}</Label>
                          <Select
                            value={form.conditionOperator}
                            onValueChange={(value) =>
                              setForm((prev) => ({
                                ...prev,
                                conditionOperator: value,
                              }))
                            }
                          >
                            <SelectTrigger className='h-9'>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {CONDITION_OPERATORS.map((operator) => (
                                <SelectItem key={operator} value={operator}>
                                  {operator}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                        <div className='space-y-1.5'>
                          <Label className='text-xs'>{t('fields.threshold')}</Label>
                          <Input
                            value={form.conditionThreshold}
                            onChange={(event) =>
                              setForm((prev) => ({
                                ...prev,
                                conditionThreshold: event.target.value,
                              }))
                            }
                            placeholder={t('placeholders.threshold')}
                            className='h-9'
                          />
                        </div>
                        <div className='space-y-1.5'>
                          <Label className='text-xs'>{t('fields.window')}</Label>
                          <div className='flex items-center gap-1.5'>
                            <Input
                              type='number'
                              min={0}
                              max={10080}
                              value={form.conditionWindowMinutes}
                              onChange={(event) =>
                                setForm((prev) => ({
                                  ...prev,
                                  conditionWindowMinutes: event.target.value,
                                }))
                              }
                              className='h-9'
                            />
                            <span className='text-xs text-muted-foreground whitespace-nowrap'>
                              {t('minutes')}
                            </span>
                          </div>
                        </div>
                      </div>

                      {selectedTemplatePromQLPreview ? (
                        <div className='space-y-1.5 pt-2'>
                          <Label className='text-xs text-muted-foreground'>
                            {t('fields.promqlPreview')}
                          </Label>
                          <Textarea
                            value={selectedTemplatePromQLPreview}
                            readOnly
                            rows={3}
                            className='font-mono text-xs bg-muted/40'
                          />
                        </div>
                      ) : null}
                    </div>
                  ) : null}
                </div>
              ) : (
                <div className='space-y-2'>
                  <Label>{t('fields.promql')}</Label>
                  <Textarea
                    value={form.promql}
                    onChange={(event) =>
                      setForm((prev) => ({...prev, promql: event.target.value}))
                    }
                    placeholder={t('placeholders.promql')}
                    rows={4}
                  />
                </div>
              )}
            </div>

            <Separator />

            <div className='space-y-4'>
              <div className='grid gap-4 md:grid-cols-3'>
                <div className='space-y-2'>
                  <Label>{t('fields.severity')}</Label>
                  <Select
                    value={form.severity}
                    onValueChange={(value) =>
                      setForm((prev) => ({
                        ...prev,
                        severity: value as AlertSeverity,
                      }))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='warning'>
                        {rootT('alertSeverity.warning')}
                      </SelectItem>
                      <SelectItem value='critical'>
                        {rootT('alertSeverity.critical')}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className='space-y-2'>
                  <Label>{t('fields.cooldown')}</Label>
                  <div className='flex items-center gap-2'>
                    <Input
                      type='number'
                      min={0}
                      value={form.cooldownMinutes}
                      onChange={(event) =>
                        setForm((prev) => ({
                          ...prev,
                          cooldownMinutes: event.target.value,
                        }))
                      }
                    />
                    <span className='text-sm text-muted-foreground'>
                      {t('minutes')}
                    </span>
                  </div>
                </div>
                <div className='flex items-end gap-2'>
                  <div className='space-y-2'>
                    <Label>{t('fields.enabled')}</Label>
                    <div>
                      <Switch
                        checked={form.enabled}
                        onCheckedChange={(checked) =>
                          setForm((prev) => ({...prev, enabled: checked}))
                        }
                      />
                    </div>
                  </div>
                </div>
              </div>

              <div className='space-y-3'>
                <div className='flex items-center justify-between gap-2'>
                  <div className='space-y-1'>
                    <Label>{t('fields.receivers')}</Label>
                    <p className='text-xs text-muted-foreground'>
                      {t('receiverHint')}
                    </p>
                  </div>
                  {preferredDefaultReceiverUserIds.length > 0 ? (
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={() =>
                        setForm((prev) => ({
                          ...prev,
                          receiverUserIds: preferredDefaultReceiverUserIds,
                        }))
                      }
                    >
                      {t('receiverResetDefault')}
                    </Button>
                  ) : null}
                </div>
                {notifiableUsers.length === 0 ? (
                  <div className='rounded-lg border border-dashed px-4 py-3 text-sm text-muted-foreground'>
                    {t('receiverEmpty')}
                  </div>
                ) : (
                  <div className='grid gap-2 md:grid-cols-2'>
                    {notifiableUsers.map((user: NotificationRecipientUser) => {
                      const checked = form.receiverUserIds.includes(
                        String(user.id),
                      );
                      return (
                        <div
                          key={user.id}
                          role='button'
                          tabIndex={0}
                          className={cn(
                            'flex items-start gap-3 rounded-lg border px-3 py-3 text-left transition-colors',
                            checked
                              ? 'border-primary bg-primary/5'
                              : 'border-border',
                          )}
                          onClick={() =>
                            setForm((prev) => {
                              const current = new Set(prev.receiverUserIds);
                              const nextValue = String(user.id);
                              if (current.has(nextValue)) {
                                current.delete(nextValue);
                              } else {
                                current.add(nextValue);
                              }
                              return {
                                ...prev,
                                receiverUserIds: Array.from(current),
                              };
                            })
                          }
                          onKeyDown={(event) => {
                            if (event.key !== 'Enter' && event.key !== ' ') {
                              return;
                            }
                            event.preventDefault();
                            setForm((prev) => {
                              const current = new Set(prev.receiverUserIds);
                              const nextValue = String(user.id);
                              if (current.has(nextValue)) {
                                current.delete(nextValue);
                              } else {
                                current.add(nextValue);
                              }
                              return {
                                ...prev,
                                receiverUserIds: Array.from(current),
                              };
                            });
                          }}
                        >
                          <Checkbox
                            checked={checked}
                            className='mt-0.5 pointer-events-none'
                          />
                          <div className='min-w-0 space-y-1'>
                            <div className='flex flex-wrap items-center gap-2'>
                              <span className='font-medium'>
                                {user.nickname || user.username}
                              </span>
                              {user.is_admin ? (
                                <Badge variant='secondary'>
                                  {t('receiverAdmin')}
                                </Badge>
                              ) : null}
                            </div>
                            <div className='text-xs text-muted-foreground'>
                              {user.username} · {user.email}
                            </div>
                          </div>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>

              <div className='space-y-2'>
                <Label>{t('fields.methods')}</Label>
                <div className='grid gap-3 md:grid-cols-2'>
                  <button
                    type='button'
                    className={cn(
                      'rounded-lg border px-4 py-3 text-left transition-colors',
                      form.emailChannelId !== 'none'
                        ? 'border-primary bg-primary/5'
                        : 'border-border',
                    )}
                    onClick={() => {
                      if (emailChannels.length === 0) {
                        openNewEmailChannelDialog();
                        return;
                      }
                      setForm((prev) => ({
                        ...prev,
                        emailChannelId:
                          prev.emailChannelId !== 'none'
                            ? 'none'
                            : String(emailChannels[0].id),
                      }));
                    }}
                  >
                    <div className='flex items-center gap-2 font-medium'>
                      <Mail className='h-4 w-4 text-amber-500' />
                      {t('methods.email')}
                    </div>
                    <div className='mt-1 text-xs text-muted-foreground'>
                      {emailChannels.length > 0
                        ? currentEmailChannel?.name || t('methods.selectEmail')
                        : t('methods.noEmailConfig')}
                    </div>
                  </button>
                  <button
                    type='button'
                    className={cn(
                      'rounded-lg border px-4 py-3 text-left transition-colors',
                      form.webhookChannelId !== 'none'
                        ? 'border-primary bg-primary/5'
                        : 'border-border',
                    )}
                    onClick={() => {
                      if (webhookChannels.length === 0) {
                        openNewWebhookChannelDialog();
                        return;
                      }
                      setForm((prev) => ({
                        ...prev,
                        webhookChannelId:
                          prev.webhookChannelId !== 'none'
                            ? 'none'
                            : String(webhookChannels[0].id),
                      }));
                    }}
                  >
                    <div className='flex items-center gap-2 font-medium'>
                      <Webhook className='h-4 w-4 text-sky-500' />
                      {t('methods.webhook')}
                    </div>
                    <div className='mt-1 text-xs text-muted-foreground'>
                      {webhookChannels.length > 0
                        ? currentWebhookChannel?.name ||
                          t('methods.selectWebhook')
                        : t('methods.noWebhookConfig')}
                    </div>
                  </button>
                </div>
              </div>

              {form.emailChannelId !== 'none' ? (
                <div className='space-y-2'>
                  <div className='flex items-center justify-between gap-2'>
                    <Label>{t('fields.emailChannel')}</Label>
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={openNewEmailChannelDialog}
                    >
                      {t('channel.manageEmail')}
                    </Button>
                  </div>
                  <Select
                    value={form.emailChannelId}
                    onValueChange={(value) =>
                      setForm((prev) => ({...prev, emailChannelId: value}))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {emailChannels.map((channel) => (
                        <SelectItem key={channel.id} value={String(channel.id)}>
                          {channel.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {currentEmailChannel ? (
                    <p className='text-xs text-muted-foreground'>
                      {getEmailConfig(currentEmailChannel)?.recipients?.join(
                        ', ',
                      ) || currentEmailChannel.endpoint}
                    </p>
                  ) : null}
                </div>
              ) : null}

              {form.webhookChannelId !== 'none' ? (
                <div className='space-y-2'>
                  <div className='flex items-center justify-between gap-2'>
                    <Label>{t('fields.webhookChannel')}</Label>
                    <Button
                      variant='ghost'
                      size='sm'
                      onClick={openNewWebhookChannelDialog}
                    >
                      {t('webhook.manage')}
                    </Button>
                  </div>
                  <Select
                    value={form.webhookChannelId}
                    onValueChange={(value) =>
                      setForm((prev) => ({...prev, webhookChannelId: value}))
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {webhookChannels.map((channel) => (
                        <SelectItem key={channel.id} value={String(channel.id)}>
                          {channel.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {currentWebhookChannel ? (
                    <p className='text-xs text-muted-foreground'>
                      {currentWebhookChannel.endpoint || '-'}
                    </p>
                  ) : null}
                </div>
              ) : null}

              <div className='flex items-center justify-between rounded-lg border px-4 py-3'>
                <div>
                  <div className='font-medium'>{t('fields.recovery')}</div>
                  <div className='text-xs text-muted-foreground'>
                    {t('recoveryDesc')}
                  </div>
                </div>
                <Switch
                  checked={form.sendRecovery}
                  onCheckedChange={(checked) =>
                    setForm((prev) => ({...prev, sendRecovery: checked}))
                  }
                />
              </div>
            </div>

          </div>

          {/* 底部操作按钮栏 / Dialog Action Footer */}
          <DialogFooter className='px-6 py-4 border-t bg-muted/15 flex items-center justify-end gap-2'>
            <Button
              variant='outline'
              onClick={() => {
                resetPolicyForm();
                setPolicyDialogOpen(false);
              }}
            >
              <X className='mr-2 h-4 w-4' />
              {t('cancelEdit')}
            </Button>
            <Button
              onClick={() => void handleSubmitPolicy()}
              disabled={savingPolicy}
            >
              {editingPolicyId === null ? (
                <Plus className='mr-2 h-4 w-4' />
              ) : (
                <Save className='mr-2 h-4 w-4' />
              )}
              {editingPolicyId === null ? t('createSubmit') : t('saveSubmit')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={emailDialogOpen}
        onOpenChange={(open) => {
          if (open) {
            setEmailDialogOpen(true);
            return;
          }
          // 关闭时若有脏数据，先走系统确认框 / Confirm discard via in-app dialog when dirty
          requestEmailChannelDiscard(() => closeEmailChannelDialog(true));
        }}
      >
        <DialogContent className='flex h-[85vh] w-[95vw] max-w-[900px] flex-col overflow-hidden p-0 sm:max-w-[900px] border border-border/80 dark:border-border/60 shadow-2xl'>
          <DialogHeader className='px-6 py-4 border-b bg-muted/15 shrink-0'>
            <DialogTitle className='text-lg font-semibold'>{t('channel.dialogTitle')}</DialogTitle>
            <DialogDescription className='text-xs text-muted-foreground'>{t('channel.dialogSubtitle')}</DialogDescription>
          </DialogHeader>

          <div className='min-h-0 flex-1 overflow-y-auto px-6 py-4'>

              <div className='space-y-4'>
                <Card>
                  <CardHeader className='space-y-3'>
                    <div className='flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between'>
                      <div className='space-y-2'>
                        <CardTitle>
                          {emailChannelForm.id === null
                            ? t('channel.createTitle')
                            : t('channel.editTitle')}
                        </CardTitle>
                        <div className='flex flex-wrap items-center gap-2'>
                          <Badge
                            variant={
                              emailChannelForm.enabled ? 'default' : 'secondary'
                            }
                          >
                            {emailChannelForm.enabled
                              ? t('channel.statusEnabled')
                              : t('channel.statusDraft')}
                          </Badge>
                          {editingEmailChannel ? (
                            <span className='text-xs text-muted-foreground'>
                              {t('channel.updatedAt', {
                                value: formatDateTime(
                                  editingEmailChannel.updated_at,
                                ),
                              })}
                            </span>
                          ) : null}
                        </div>
                      </div>

                      <div className='flex flex-wrap items-center gap-2'>
                        <Button
                          variant='outline'
                          onClick={() => void handleTestEmailConnection()}
                          disabled={testingEmailConnection}
                        >
                          <RefreshCw className='mr-2 h-4 w-4' />
                          {t('channel.connectionTest')}
                        </Button>
                        <Button
                          variant='outline'
                          onClick={() => void handleTestEmailChannel()}
                          disabled={isTestingCurrentEmailChannel}
                        >
                          <Send className='mr-2 h-4 w-4' />
                          {t('channel.test')}
                        </Button>
                      </div>
                    </div>

                    <div className='grid gap-3 lg:grid-cols-2'>
                      <div className='rounded-lg border bg-muted/30 px-4 py-3'>
                        <div className='flex items-center gap-2 text-sm font-medium'>
                          {lastEmailChannelConnectionResult?.status ===
                          'sent' ? (
                            <CircleCheck className='h-4 w-4 text-emerald-600' />
                          ) : lastEmailChannelConnectionResult?.status ===
                            'failed' ? (
                            <CircleX className='h-4 w-4 text-destructive' />
                          ) : (
                            <RefreshCw className='h-4 w-4 text-muted-foreground' />
                          )}
                          {lastEmailChannelConnectionResult
                            ? lastEmailChannelConnectionResult.status === 'sent'
                              ? t('channel.connectionLastTest.success')
                              : t('channel.connectionLastTest.failed')
                            : t('channel.connectionLastTest.idle')}
                        </div>
                        <div className='mt-1 text-xs text-muted-foreground'>
                          {t('channel.connectionLastTest.checkedAt', {
                            value: formatDateTime(
                              lastEmailChannelConnectionResult?.checked_at,
                            ),
                          })}
                        </div>
                        {emailConnectionStatusOutdated ? (
                          <div className='mt-2 text-xs text-amber-600'>
                            {t('channel.statusOutdated')}
                          </div>
                        ) : null}
                        {lastEmailChannelConnectionResult?.last_error ? (
                          <div className='mt-2 text-xs text-destructive'>
                            {lastEmailChannelConnectionResult.last_error}
                          </div>
                        ) : null}
                      </div>

                      <div className='rounded-lg border bg-muted/30 px-4 py-3'>
                        <div className='flex items-center gap-2 text-sm font-medium'>
                          {lastEmailChannelTestResult?.status === 'sent' ? (
                            <CircleCheck className='h-4 w-4 text-emerald-600' />
                          ) : lastEmailChannelTestResult?.status ===
                            'failed' ? (
                            <CircleX className='h-4 w-4 text-destructive' />
                          ) : (
                            <Send className='h-4 w-4 text-muted-foreground' />
                          )}
                          {lastEmailChannelTestResult
                            ? lastEmailChannelTestResult.status === 'sent'
                              ? t('channel.lastTest.success')
                              : t('channel.lastTest.failed')
                            : t('channel.lastTest.idle')}
                        </div>
                        <div className='mt-1 text-xs text-muted-foreground'>
                          {t('channel.lastTest.sentAt', {
                            value: formatDateTime(
                              lastEmailChannelTestResult?.sent_at,
                            ),
                          })}
                        </div>
                        {lastEmailChannelTestResult ? (
                          <div className='mt-1 text-xs text-muted-foreground'>
                            {t('channel.lastTest.receiver', {
                              value:
                                lastEmailChannelTestResult.receiver ||
                                t('channel.testRecipientLegacyFallback'),
                            })}
                          </div>
                        ) : null}
                        {emailTestStatusOutdated ? (
                          <div className='mt-2 text-xs text-amber-600'>
                            {t('channel.statusOutdated')}
                          </div>
                        ) : null}
                        {lastEmailChannelTestResult?.last_error ? (
                          <div className='mt-2 text-xs text-destructive'>
                            {lastEmailChannelTestResult.last_error}
                          </div>
                        ) : null}
                      </div>
                    </div>
                  </CardHeader>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>{t('channel.sections.basic')}</CardTitle>
                  </CardHeader>
                  <CardContent className='space-y-4'>
                    <div className='space-y-2'>
                      <Label>{t('channel.fields.name')}</Label>
                      <Input
                        value={emailChannelForm.name}
                        onChange={(event) =>
                          updateEmailChannelForm((prev) => ({
                            ...prev,
                            name: event.target.value,
                          }))
                        }
                      />
                    </div>

                    <div className='space-y-2'>
                      <Label>{t('channel.fields.description')}</Label>
                      <Textarea
                        value={emailChannelForm.description}
                        onChange={(event) =>
                          updateEmailChannelForm((prev) => ({
                            ...prev,
                            description: event.target.value,
                          }))
                        }
                        rows={3}
                      />
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>{t('channel.sections.connection')}</CardTitle>
                  </CardHeader>
                  <CardContent className='space-y-4'>
                    <div className='grid gap-4 md:grid-cols-3'>
                      <div className='space-y-2'>
                        <Label>{t('channel.fields.protocol')}</Label>
                        <Select
                          value={emailChannelForm.protocol}
                          onValueChange={(value) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              protocol: value,
                            }))
                          }
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value='smtp'>SMTP</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className='space-y-2'>
                        <Label>{t('channel.fields.security')}</Label>
                        <Select
                          value={emailChannelForm.security}
                          onValueChange={(value) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              security:
                                value as EmailChannelFormState['security'],
                              port: defaultPortForSecurity(
                                value as EmailChannelFormState['security'],
                              ),
                            }))
                          }
                        >
                          <SelectTrigger>
                            <SelectValue />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value='ssl'>SSL/TLS</SelectItem>
                            <SelectItem value='starttls'>STARTTLS</SelectItem>
                            <SelectItem value='none'>None</SelectItem>
                          </SelectContent>
                        </Select>
                      </div>
                      <div className='space-y-2'>
                        <Label>{t('channel.fields.port')}</Label>
                        <Input
                          type='number'
                          value={emailChannelForm.port}
                          onChange={(event) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              port: event.target.value,
                            }))
                          }
                        />
                      </div>
                    </div>

                    <div className='grid gap-4 md:grid-cols-2'>
                      <div className='space-y-2'>
                        <Label>{t('channel.fields.host')}</Label>
                        <Input
                          value={emailChannelForm.host}
                          onChange={(event) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              host: event.target.value,
                            }))
                          }
                          placeholder='smtp.example.com'
                        />
                      </div>
                      <div className='space-y-2'>
                        <Label>{t('channel.fields.username')}</Label>
                        <Input
                          value={emailChannelForm.username}
                          name='stx-smtp-username'
                          autoComplete='off'
                          autoCorrect='off'
                          autoCapitalize='none'
                          spellCheck={false}
                          onChange={(event) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              username: event.target.value,
                            }))
                          }
                        />
                      </div>
                    </div>

                    <div className='space-y-2'>
                      <Label>{t('channel.fields.password')}</Label>
                      <div className='relative'>
                        <Input
                          type={showEmailPassword ? 'text' : 'password'}
                          value={emailChannelForm.password}
                          name='stx-smtp-password'
                          autoComplete='new-password'
                          onChange={(event) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              password: event.target.value,
                            }))
                          }
                          className='pr-20'
                        />
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='absolute right-1 top-1/2 h-8 -translate-y-1/2 px-2'
                          onClick={() => setShowEmailPassword((prev) => !prev)}
                        >
                          {showEmailPassword ? (
                            <EyeOff className='h-4 w-4' />
                          ) : (
                            <Eye className='h-4 w-4' />
                          )}
                        </Button>
                      </div>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>{t('channel.sections.sender')}</CardTitle>
                  </CardHeader>
                  <CardContent className='space-y-4'>
                    <div className='grid gap-4 md:grid-cols-2'>
                      <div className='space-y-2'>
                        <Label>{t('channel.fields.from')}</Label>
                        <Input
                          value={emailChannelForm.from}
                          onChange={(event) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              from: event.target.value,
                            }))
                          }
                        />
                      </div>
                      <div className='space-y-2'>
                        <Label>{t('channel.fields.fromName')}</Label>
                        <Input
                          value={emailChannelForm.fromName}
                          onChange={(event) =>
                            updateEmailChannelForm((prev) => ({
                              ...prev,
                              fromName: event.target.value,
                            }))
                          }
                        />
                      </div>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>{t('channel.sections.delivery')}</CardTitle>
                  </CardHeader>
                  <CardContent className='space-y-4'>
                    <div className='space-y-2'>
                      <Label>{t('channel.fields.testRecipient')}</Label>
                      <Select
                        value={emailTestReceiver}
                        onValueChange={(value) => setEmailTestReceiver(value)}
                      >
                        <SelectTrigger>
                          <SelectValue
                            placeholder={t('channel.testRecipientPlaceholder')}
                          />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value='none'>
                            {t('channel.testRecipientNone')}
                          </SelectItem>
                          {notifiableUsers.map((user) => (
                            <SelectItem key={user.id} value={String(user.id)}>
                              {`${user.nickname || user.username}${
                                currentUser?.id === user.id
                                  ? ` · ${t('channel.currentUser')}`
                                  : ''
                              } (${user.email})`}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                      <p className='text-xs text-muted-foreground'>
                        {emailTestRecipientHint}
                      </p>
                    </div>

                    {emailFallbackRecipients.length > 0 ? (
                      <div className='rounded-lg border border-dashed px-4 py-3 text-sm text-muted-foreground'>
                        <div className='space-y-2'>
                          <p>
                            {t('channel.legacyRecipientsConfigured', {
                              count: emailFallbackRecipients.length,
                            })}
                          </p>
                          <div className='break-all text-xs'>
                            {emailFallbackRecipients.join(', ')}
                          </div>
                          <Button
                            variant='ghost'
                            size='sm'
                            className='px-0'
                            onClick={() =>
                              updateEmailChannelForm((prev) => ({
                                ...prev,
                                recipients: '',
                              }))
                            }
                          >
                            {t('channel.clearLegacyRecipients')}
                          </Button>
                        </div>
                      </div>
                    ) : null}
                  </CardContent>
                </Card>

                <div className='flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
                  <div>
                    {emailChannelForm.id !== null ? (
                      <Button
                        variant='destructive'
                        onClick={() => void handleDeleteEmailChannel()}
                      >
                        <Trash2 className='mr-2 h-4 w-4' />
                        {t('channel.delete')}
                      </Button>
                    ) : null}
                  </div>

                  <div className='flex flex-wrap items-center justify-end gap-2'>
                    <Button
                      variant='outline'
                      onClick={() =>
                        requestEmailChannelDiscard(() =>
                          closeEmailChannelDialog(true),
                        )
                      }
                    >
                      {t('channel.cancel')}
                    </Button>
                    <Button
                      variant='outline'
                      onClick={() => void handleSaveEmailChannel(false)}
                      disabled={savingEmailChannel}
                    >
                      <Save className='mr-2 h-4 w-4' />
                      {t('channel.saveDraft')}
                    </Button>
                    <Button
                      onClick={() => void handleSaveEmailChannel(true)}
                      disabled={savingEmailChannel}
                    >
                      <Save className='mr-2 h-4 w-4' />
                      {t('channel.saveAndEnable')}
                    </Button>
                  </div>
                </div>
              </div>
            
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={webhookDialogOpen}
        onOpenChange={(open) => {
          if (open) {
            setWebhookDialogOpen(true);
            return;
          }
          requestWebhookChannelDiscard(() => closeWebhookChannelDialog(true));
        }}
      >
        <DialogContent className='flex h-[85vh] w-[95vw] max-w-[900px] flex-col overflow-hidden p-0 sm:max-w-[900px] border border-border/80 dark:border-border/60 shadow-2xl'>
          <DialogHeader className='px-6 py-4 border-b bg-muted/15 shrink-0'>
            <DialogTitle className='text-lg font-semibold'>{t('webhook.dialogTitle')}</DialogTitle>
            <DialogDescription className='text-xs text-muted-foreground'>{t('webhook.dialogSubtitle')}</DialogDescription>
          </DialogHeader>

          <div className='min-h-0 flex-1 overflow-y-auto px-6 py-4'>

              <div className='space-y-4'>
                <Card>
                  <CardHeader className='space-y-3'>
                    <div className='flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between'>
                      <div className='space-y-2'>
                        <CardTitle>
                          {webhookChannelForm.id === null
                            ? t('webhook.createTitle')
                            : t('webhook.editTitle')}
                        </CardTitle>
                        <div className='flex flex-wrap items-center gap-2'>
                          <Badge
                            variant={
                              webhookChannelForm.enabled
                                ? 'default'
                                : 'secondary'
                            }
                          >
                            {webhookChannelForm.enabled
                              ? t('webhook.statusEnabled')
                              : t('webhook.statusDraft')}
                          </Badge>
                          {editingWebhookChannel ? (
                            <span className='text-xs text-muted-foreground'>
                              {t('webhook.updatedAt', {
                                value: formatDateTime(
                                  editingWebhookChannel.updated_at,
                                ),
                              })}
                            </span>
                          ) : null}
                        </div>
                      </div>

                      <div className='flex flex-wrap items-center gap-2'>
                        <Button
                          variant='outline'
                          onClick={() => void handleTestWebhookChannel()}
                          disabled={isTestingCurrentWebhookChannel}
                        >
                          <Send className='mr-2 h-4 w-4' />
                          {t('webhook.test')}
                        </Button>
                      </div>
                    </div>

                    <div className='rounded-lg border bg-muted/30 px-4 py-3'>
                      <div className='flex items-center gap-2 text-sm font-medium'>
                        {lastWebhookChannelTestResult?.status === 'sent' ? (
                          <CircleCheck className='h-4 w-4 text-emerald-600' />
                        ) : lastWebhookChannelTestResult?.status ===
                          'failed' ? (
                          <CircleX className='h-4 w-4 text-destructive' />
                        ) : (
                          <Send className='h-4 w-4 text-muted-foreground' />
                        )}
                        {lastWebhookChannelTestResult
                          ? lastWebhookChannelTestResult.status === 'sent'
                            ? t('webhook.lastTest.success')
                            : t('webhook.lastTest.failed')
                          : t('webhook.lastTest.idle')}
                      </div>
                      <div className='mt-1 text-xs text-muted-foreground'>
                        {t('webhook.lastTest.sentAt', {
                          value: formatDateTime(
                            lastWebhookChannelTestResult?.sent_at,
                          ),
                        })}
                      </div>
                      {lastWebhookChannelTestResult?.status_code ? (
                        <div className='mt-1 text-xs text-muted-foreground'>
                          {t('webhook.lastTest.statusCode', {
                            value: lastWebhookChannelTestResult.status_code,
                          })}
                        </div>
                      ) : null}
                      {webhookTestStatusOutdated ? (
                        <div className='mt-2 text-xs text-amber-600'>
                          {t('webhook.statusOutdated')}
                        </div>
                      ) : null}
                      {lastWebhookChannelTestResult?.last_error ? (
                        <div className='mt-2 text-xs text-destructive'>
                          {lastWebhookChannelTestResult.last_error}
                        </div>
                      ) : null}
                    </div>
                  </CardHeader>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>{t('webhook.sections.basic')}</CardTitle>
                  </CardHeader>
                  <CardContent className='space-y-4'>
                    <div className='space-y-2'>
                      <Label>{t('webhook.fields.name')}</Label>
                      <Input
                        value={webhookChannelForm.name}
                        onChange={(event) =>
                          updateWebhookChannelForm((prev) => ({
                            ...prev,
                            name: event.target.value,
                          }))
                        }
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label>{t('webhook.fields.description')}</Label>
                      <Textarea
                        rows={3}
                        value={webhookChannelForm.description}
                        onChange={(event) =>
                          updateWebhookChannelForm((prev) => ({
                            ...prev,
                            description: event.target.value,
                          }))
                        }
                      />
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>{t('webhook.sections.connection')}</CardTitle>
                  </CardHeader>
                  <CardContent className='space-y-4'>
                    <div className='space-y-2'>
                      <Label>{t('webhook.fields.endpoint')}</Label>
                      <Input
                        value={webhookChannelForm.endpoint}
                        onChange={(event) =>
                          updateWebhookChannelForm((prev) => ({
                            ...prev,
                            endpoint: event.target.value,
                          }))
                        }
                        placeholder='https://hooks.example.com/stx'
                      />
                    </div>

                    <div className='space-y-2'>
                      <Label>{t('webhook.fields.secret')}</Label>
                      <div className='relative'>
                        <Input
                          type={showWebhookSecret ? 'text' : 'password'}
                          value={webhookChannelForm.secret}
                          name='stx-webhook-secret'
                          autoComplete='new-password'
                          onChange={(event) =>
                            updateWebhookChannelForm((prev) => ({
                              ...prev,
                              secret: event.target.value,
                            }))
                          }
                          className='pr-20'
                          placeholder={t('webhook.placeholders.secret')}
                        />
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='absolute right-1 top-1/2 h-8 -translate-y-1/2 px-2'
                          onClick={() => setShowWebhookSecret((prev) => !prev)}
                        >
                          {showWebhookSecret ? (
                            <EyeOff className='h-4 w-4' />
                          ) : (
                            <Eye className='h-4 w-4' />
                          )}
                        </Button>
                      </div>
                      <p className='text-xs text-muted-foreground'>
                        {t('webhook.secretHint')}
                      </p>
                    </div>
                  </CardContent>
                </Card>

                <div className='flex flex-col gap-3 border-t pt-4 sm:flex-row sm:items-center sm:justify-between'>
                  <div>
                    {webhookChannelForm.id !== null ? (
                      <Button
                        variant='destructive'
                        onClick={() => void handleDeleteWebhookChannel()}
                      >
                        <Trash2 className='mr-2 h-4 w-4' />
                        {t('webhook.delete')}
                      </Button>
                    ) : null}
                  </div>

                  <div className='flex flex-wrap items-center justify-end gap-2'>
                    <Button
                      variant='outline'
                      onClick={() =>
                        requestWebhookChannelDiscard(() =>
                          closeWebhookChannelDialog(true),
                        )
                      }
                    >
                      {t('webhook.cancel')}
                    </Button>
                    <Button
                      variant='outline'
                      onClick={() => void handleSaveWebhookChannel(false)}
                      disabled={savingWebhookChannel}
                    >
                      <Save className='mr-2 h-4 w-4' />
                      {t('webhook.saveDraft')}
                    </Button>
                    <Button
                      onClick={() => void handleSaveWebhookChannel(true)}
                      disabled={savingWebhookChannel}
                    >
                      <Save className='mr-2 h-4 w-4' />
                      {t('webhook.saveAndEnable')}
                    </Button>
                  </div>
                </div>
              </div>
            
          </div>
        </DialogContent>
      </Dialog>

      {/* 邮件通道：放弃未保存变更确认 / Email channel discard confirmation */}
      <AlertDialog
        open={emailDiscardDialogOpen}
        onOpenChange={setEmailDiscardDialogOpen}
      >
        <AlertDialogContent className='border border-border/80 dark:border-border/60 shadow-2xl'>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('channel.unsavedChangesTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('channel.unsavedChangesConfirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel
              onClick={() => {
                emailDiscardActionRef.current = null;
              }}
            >
              {t('channel.keepEditing')}
            </AlertDialogCancel>
            <AlertDialogAction
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
              onClick={() => {
                const action = emailDiscardActionRef.current;
                emailDiscardActionRef.current = null;
                action?.();
              }}
            >
              {t('channel.discardChanges')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Webhook 通道：放弃未保存变更确认 / Webhook channel discard confirmation */}
      <AlertDialog
        open={webhookDiscardDialogOpen}
        onOpenChange={setWebhookDiscardDialogOpen}
      >
        <AlertDialogContent className='border border-border/80 dark:border-border/60 shadow-2xl'>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t('webhook.unsavedChangesTitle')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t('webhook.unsavedChangesConfirm')}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel
              onClick={() => {
                webhookDiscardActionRef.current = null;
              }}
            >
              {t('webhook.keepEditing')}
            </AlertDialogCancel>
            <AlertDialogAction
              className='bg-destructive text-destructive-foreground hover:bg-destructive/90'
              onClick={() => {
                const action = webhookDiscardActionRef.current;
                webhookDiscardActionRef.current = null;
                action?.();
              }}
            >
              {t('webhook.discardChanges')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}