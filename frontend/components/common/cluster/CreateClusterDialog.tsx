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
 * Create Cluster Dialog Component
 * 创建集群对话框组件
 *
 * Dialog for registering an existing SeaTunnel cluster.
 * Supports process discovery to auto-fill cluster info from running processes.
 * 用于注册已有 SeaTunnel 集群的对话框。
 * 支持进程发现以从运行中的进程自动填充集群信息。
 */

import {useState, useEffect} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {Loader2, Search, Server, Cpu, CheckCircle2} from 'lucide-react';
import {toast} from 'sonner';
import services from '@/lib/services';
import {DeploymentMode, CreateClusterRequest} from '@/lib/services/cluster/types';
import {HostInfo, AgentStatus} from '@/lib/services/host/types';
import {DiscoveredProcess} from '@/lib/services/discovery/discovery.service';

interface CreateClusterDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

/**
 * Create Cluster Dialog Component
 * 创建集群对话框组件
 */
export function CreateClusterDialog({open, onOpenChange, onSuccess}: CreateClusterDialogProps) {
  const t = useTranslations();
  const [loading, setLoading] = useState(false);
  const [loadingHosts, setLoadingHosts] = useState(false);

  // Form state / 表单状态
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [deploymentMode, setDeploymentMode] = useState<DeploymentMode>(DeploymentMode.HYBRID);
  const [version, setVersion] = useState('');

  // Host and discovery state / 主机和发现状态
  const [availableHosts, setAvailableHosts] = useState<HostInfo[]>([]);
  const [selectedHostIds, setSelectedHostIds] = useState<Set<number>>(new Set());
  const [discoveryLoading, setDiscoveryLoading] = useState(false);
  // Map of hostId -> discovered processes / 主机ID -> 发现的进程映射
  const [discoveredProcessesByHost, setDiscoveredProcessesByHost] = useState<
    Map<number, DiscoveredProcess[]>
  >(new Map());
  const [selectedProcesses, setSelectedProcesses] = useState<Set<string>>(new Set());

  /**
   * Load available hosts when dialog opens
   * 对话框打开时加载可用主机
   */
  useEffect(() => {
    if (open) {
      loadAvailableHosts();
    }
  }, [open]);

  /**
   * Load available hosts with Agent installed
   * 加载已安装 Agent 的可用主机
   */
  const loadAvailableHosts = async () => {
    setLoadingHosts(true);
    try {
      const result = await services.host.getHostsSafe({
        current: 1,
        size: 100,
        agent_status: AgentStatus.INSTALLED,
      });

      if (result.success && result.data) {
        setAvailableHosts(result.data.hosts || []);
      } else {
        setAvailableHosts([]);
      }
    } finally {
      setLoadingHosts(false);
    }
  };

  /**
   * Reset form
   * 重置表单
   */
  const resetForm = () => {
    setName('');
    setDescription('');
    setDeploymentMode(DeploymentMode.HYBRID);
    setVersion('');
    setSelectedHostIds(new Set());
    setDiscoveredProcessesByHost(new Map());
    setSelectedProcesses(new Set());
  };

  /**
   * Toggle host selection
   * 切换主机选择
   */
  const toggleHostSelection = (hostId: number) => {
    const newSelected = new Set(selectedHostIds);
    if (newSelected.has(hostId)) {
      newSelected.delete(hostId);
      // Remove discovered processes for this host / 移除该主机的发现进程
      const newProcesses = new Map(discoveredProcessesByHost);
      newProcesses.delete(hostId);
      setDiscoveredProcessesByHost(newProcesses);
    } else {
      newSelected.add(hostId);
    }
    setSelectedHostIds(newSelected);
  };

  /**
   * Select all hosts
   * 全选主机
   */
  const selectAllHosts = () => {
    setSelectedHostIds(new Set(availableHosts.map((h) => h.id)));
  };

  /**
   * Deselect all hosts
   * 取消全选主机
   */
  const deselectAllHosts = () => {
    setSelectedHostIds(new Set());
    setDiscoveredProcessesByHost(new Map());
    setSelectedProcesses(new Set());
  };

  /**
   * Handle process discovery for all selected hosts
   * 处理所有选中主机的进程发现
   */
  const handleDiscoverProcesses = async () => {
    if (selectedHostIds.size === 0) {
      toast.error(t('cluster.selectHostFirst'));
      return;
    }

    setDiscoveryLoading(true);
    setDiscoveredProcessesByHost(new Map());
    setSelectedProcesses(new Set());

    const newProcessesByHost = new Map<number, DiscoveredProcess[]>();
    let totalProcesses = 0;
    let failedHosts = 0;

    try {
      // Discover processes on all selected hosts in parallel
      // 并行在所有选中的主机上发现进程
      const hostIds = Array.from(selectedHostIds);
      const results = await Promise.allSettled(
        hostIds.map((hostId) => services.discovery.discoverProcesses(hostId))
      );

      results.forEach((result, index) => {
        const hostId = hostIds[index];
        if (result.status === 'fulfilled' && result.value.success && result.value.processes) {
          newProcessesByHost.set(hostId, result.value.processes);
          totalProcesses += result.value.processes.length;
        } else {
          failedHosts++;
        }
      });

      setDiscoveredProcessesByHost(newProcessesByHost);

      if (totalProcesses === 0) {
        toast.info(t('discovery.noProcessesFound'));
      } else {
        toast.success(
          t('discovery.processesFoundOnHosts', {
            count: totalProcesses,
            hosts: hostIds.length - failedHosts,
          })
        );
        // Auto-select all discovered processes by default to save user manual clicks
        // 发现后默认全选所有扫描出的进程，减少用户逐个点击的成本
        const allKeys = new Set<string>();
        newProcessesByHost.forEach((procs, hId) => {
          procs.forEach((p) => allKeys.add(getProcessKey(hId, p)));
        });
        setSelectedProcesses(allKeys);

        // Auto-detect deployment mode from all discovered processes
        // 从所有发现的进程自动检测部署模式
        const allProcesses = Array.from(newProcessesByHost.values()).flat();
        autoDetectDeploymentMode(allProcesses);
      }

      if (failedHosts > 0) {
        toast.warning(t('discovery.someHostsFailed', {count: failedHosts}));
      }
    } catch {
      toast.error(t('discovery.discoverError'));
    } finally {
      setDiscoveryLoading(false);
    }
  };

  /**
   * Auto-detect deployment mode and version from discovered processes
   * 从发现的进程自动检测部署模式和版本
   */
  const autoDetectDeploymentMode = (processes: DiscoveredProcess[]) => {
    const hasMaster = processes.some((p) => p.role === 'master');
    const hasWorker = processes.some((p) => p.role === 'worker');
    const hasHybrid = processes.some((p) => p.role === 'hybrid');

    if (hasMaster || hasWorker) {
      setDeploymentMode(DeploymentMode.SEPARATED);
    } else if (hasHybrid) {
      setDeploymentMode(DeploymentMode.HYBRID);
    }

    // Auto-fill version from first process with valid version (always update if found)
    // 从第一个有效版本的进程自动填充版本（发现后始终更新）
    const processWithVersion = processes.find((p) => p.version && p.version !== 'unknown');
    if (processWithVersion) {
      setVersion(processWithVersion.version);
    }
  };

  /**
   * Toggle process selection
   * 切换进程选择
   */
  const toggleProcessSelection = (processKey: string) => {
    const newSelected = new Set(selectedProcesses);
    if (newSelected.has(processKey)) {
      newSelected.delete(processKey);
    } else {
      newSelected.add(processKey);
    }
    setSelectedProcesses(newSelected);
  };

  /**
   * Select all discovered processes
   * 全选所有发现的进程
   */
  const selectAllProcesses = () => {
    const allKeys = new Set<string>();
    discoveredProcessesByHost.forEach((procs, hId) => {
      procs.forEach((p) => allKeys.add(getProcessKey(hId, p)));
    });
    setSelectedProcesses(allKeys);
  };

  /**
   * Deselect all processes
   * 取消全选进程
   */
  const deselectAllProcesses = () => {
    setSelectedProcesses(new Set());
  };

  /**
   * Toggle all processes for a specific host
   * 切换特定主机下所有进程的选择
   */
  const toggleHostProcesses = (hostId: number) => {
    const hostProcs = discoveredProcessesByHost.get(hostId) || [];
    const hostKeys = hostProcs.map((p) => getProcessKey(hostId, p));
    const allSelected = hostKeys.every((k) => selectedProcesses.has(k));
    const next = new Set(selectedProcesses);
    if (allSelected) {
      hostKeys.forEach((k) => next.delete(k));
    } else {
      hostKeys.forEach((k) => next.add(k));
    }
    setSelectedProcesses(next);
  };

  /**
   * Parse selected process key to get hostId, pid, installDir
   * 解析选中的进程键获取主机ID、PID、安装目录
   */
  const parseProcessKey = (key: string) => {
    const parts = key.split('-');
    const hostId = parseInt(parts[0], 10);
    const pid = parseInt(parts[1], 10);
    const installDir = parts.slice(2).join('-'); // install_dir may contain '-'
    return {hostId, pid, installDir};
  };

  /**
   * Get selected processes with host info
   * 获取选中的进程及其主机信息
   */
  const getSelectedProcessesWithHost = () => {
    const result: Array<{
      hostId: number;
      pid: number;
      role: string;
      installDir: string;
      hazelcastPort: number;
      apiPort: number;
    }> = [];

    selectedProcesses.forEach((key) => {
      const {hostId, pid, installDir} = parseProcessKey(key);
      const processes = discoveredProcessesByHost.get(hostId);
      if (processes) {
        const proc = processes.find((p) => p.pid === pid && p.install_dir === installDir);
        if (proc) {
          result.push({
            hostId,
            pid: proc.pid,
            role: proc.role,
            installDir: proc.install_dir,
            hazelcastPort: proc.hazelcast_port || 0,
            apiPort: proc.api_port || 0,
          });
        }
      }
    });

    return result;
  };

  /**
   * Handle submit
   * 处理提交
   */
  const handleSubmit = async () => {
    // Validate required fields / 验证必填字段
    if (!name.trim()) {
      toast.error(t('cluster.nameRequired'));
      return;
    }

    if (!version.trim()) {
      toast.error(t('cluster.versionRequired'));
      return;
    }

    // Get selected processes info / 获取选中的进程信息
    const selectedProcessInfo = getSelectedProcessesWithHost();
    if (selectedProcessInfo.length === 0) {
      toast.error(t('cluster.selectAtLeastOneNode'));
      return;
    }

    // Role validation in separated mode / 分离部署模式下的角色完整性校验
    if (deploymentMode === DeploymentMode.SEPARATED) {
      const hasMaster = selectedProcessInfo.some((p) => p.role === 'master');
      const hasWorker = selectedProcessInfo.some((p) => p.role === 'worker');
      if (!hasMaster || !hasWorker) {
        toast.error(t('cluster.separatedRequiresMasterAndWorker'));
        return;
      }
    }

    setLoading(true);
    try {
      // Build nodes from selected processes / 从选中的进程构建节点
      // In separated mode, each process (master/worker) is a separate node
      // 在分离模式下，每个进程（master/worker）是一个独立的节点
      // Only merge if same role on same host+installDir (shouldn't happen normally)
      // 只有在同一主机+安装目录上有相同角色时才合并（正常情况下不会发生）
      const nodeMap = new Map<
        string,
        {
          hostId: number;
          installDir: string;
          role: string;
          pids: number[];
          hazelcastPort: number;
          apiPort: number;
        }
      >();

      selectedProcessInfo.forEach((proc) => {
        // Key includes role to keep master and worker as separate nodes
        // 键包含角色以保持 master 和 worker 作为独立节点
        const nodeKey = `${proc.hostId}-${proc.installDir}-${proc.role}`;
        const existing = nodeMap.get(nodeKey);
        if (existing) {
          existing.pids.push(proc.pid);
          // Use the port if available / 如果有端口则使用
          if (proc.hazelcastPort > 0) {
            existing.hazelcastPort = proc.hazelcastPort;
          }
          if (proc.apiPort > 0) {
            existing.apiPort = proc.apiPort;
          }
        } else {
          nodeMap.set(nodeKey, {
            hostId: proc.hostId,
            installDir: proc.installDir,
            role: proc.role,
            pids: [proc.pid],
            hazelcastPort: proc.hazelcastPort,
            apiPort: proc.apiPort,
          });
        }
      });

      // Convert to nodes array with discovered PID / 转换为包含发现 PID 的节点数组
      const nodes = Array.from(nodeMap.values()).map((node) => ({
        host_id: node.hostId,
        install_dir: node.installDir,
        role: node.role,
        pid: node.pids[0] || undefined,
        hazelcast_port: node.hazelcastPort > 0 ? node.hazelcastPort : undefined,
        api_port: node.apiPort > 0 ? node.apiPort : undefined,
      }));

      const data: CreateClusterRequest = {
        name: name.trim(),
        description: description.trim() || undefined,
        deployment_mode: deploymentMode,
        version: version.trim() || undefined,
        install_dir: selectedProcessInfo[0]?.installDir || undefined,
        nodes: nodes.length > 0 ? nodes : undefined,
      };

      const result = await services.cluster.createClusterSafe(data);

      if (result.success) {
        toast.success(t('cluster.createSuccess'));
        // Trigger initial config sync from master or hybrid node in background
        // 后台静默触发首次集群配置拉取，填充模板供后续配置管理查看
        if (result.data?.id) {
          const createdClusterId = result.data.id;
          const targetNode =
            selectedProcessInfo.find((p) => p.role === 'master' || p.role === 'hybrid') ||
            selectedProcessInfo[0];
          if (targetNode) {
            services.config
              .initClusterConfigsSafe(createdClusterId, targetNode.hostId, targetNode.installDir)
              .catch(() => {});
          }
        }
        resetForm();
        onSuccess();
      } else {
        toast.error(result.error || t('cluster.createError'));
      }
    } finally {
      setLoading(false);
    }
  };

  /**
   * Handle close
   * 处理关闭
   */
  const handleClose = (open: boolean) => {
    if (!open) {
      resetForm();
    }
    onOpenChange(open);
  };

  /**
   * Get process key for selection (includes hostId)
   * 获取进程选择的键（包含主机ID）
   */
  const getProcessKey = (hostId: number, proc: DiscoveredProcess) =>
    `${hostId}-${proc.pid}-${proc.install_dir}`;

  /**
   * Get host name by id
   * 根据ID获取主机名
   */
  const getHostName = (hostId: number) => {
    const host = availableHosts.find((h) => h.id === hostId);
    return host ? `${host.name} (${host.ip_address})` : `Host ${hostId}`;
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className='sm:max-w-[600px] max-h-[90vh] flex flex-col'>
        <DialogHeader>
          <DialogTitle>{t('cluster.registerCluster')}</DialogTitle>
          <DialogDescription>{t('cluster.registerClusterDescription')}</DialogDescription>
        </DialogHeader>

        <div className='flex-1 overflow-y-auto space-y-4 py-4 pr-2'>
          {/* Cluster Name / 集群名称 */}
          <div className='space-y-2'>
            <Label htmlFor='name'>
              {t('cluster.name')} <span className='text-destructive'>*</span>
            </Label>
            <Input
              id='name'
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('cluster.namePlaceholder')}
            />
          </div>

          {/* Description / 描述 */}
          <div className='space-y-2'>
            <Label htmlFor='description'>{t('cluster.descriptionLabel')}</Label>
            <Textarea
              id='description'
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder={t('cluster.descriptionPlaceholder')}
              rows={2}
            />
          </div>

          {/* Deployment Mode / 部署模式 */}
          <div className='space-y-2'>
            <Label htmlFor='deploymentMode'>
              {t('cluster.deploymentMode')} <span className='text-destructive'>*</span>
            </Label>
            <Select
              value={deploymentMode}
              onValueChange={(value) => setDeploymentMode(value as DeploymentMode)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value={DeploymentMode.HYBRID}>{t('cluster.modes.hybrid')}</SelectItem>
                <SelectItem value={DeploymentMode.SEPARATED}>
                  {t('cluster.modes.separated')}
                </SelectItem>
              </SelectContent>
            </Select>
            <p className='text-xs text-muted-foreground'>
              {deploymentMode === DeploymentMode.HYBRID
                ? t('cluster.hybridDescription')
                : t('cluster.separatedDescription')}
            </p>
          </div>

          {/* Version / 版本 */}
          <div className='space-y-2'>
            <Label htmlFor='version'>
              {t('cluster.version')} <span className='text-destructive'>*</span>
            </Label>
            <Input
              id='version'
              value={version}
              onChange={(e) => setVersion(e.target.value)}
              placeholder={t('cluster.versionPlaceholder')}
            />
          </div>

          {/* Divider / 分隔线 */}
          <div className='relative py-2'>
            <div className='absolute inset-0 flex items-center'>
              <span className='w-full border-t' />
            </div>
            <div className='relative flex justify-center text-xs uppercase'>
              <span className='bg-background px-2 text-muted-foreground'>
                {t('discovery.discoverFromHost')}
              </span>
            </div>
          </div>

          {/* Host Selection for Discovery / 用于发现的主机选择 */}
          <div className='space-y-2'>
            <div className='flex items-center justify-between'>
              <Label>{t('discovery.selectHostsToDiscover')}</Label>
              {availableHosts.length > 0 && (
                <div className='flex items-center gap-1'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-6 px-2 text-xs text-muted-foreground hover:text-foreground'
                    onClick={selectAllHosts}
                  >
                    {t('discovery.selectAllHosts')}
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-6 px-2 text-xs text-muted-foreground hover:text-foreground'
                    onClick={deselectAllHosts}
                  >
                    {t('discovery.deselectAllHosts')}
                  </Button>
                </div>
              )}
            </div>
            {loadingHosts ? (
              <div className='flex items-center gap-2 text-muted-foreground'>
                <Loader2 className='h-4 w-4 animate-spin' />
                {t('common.loading')}
              </div>
            ) : availableHosts.length === 0 ? (
              <div className='text-sm text-muted-foreground p-3 border rounded-md bg-muted/30'>
                {t('cluster.noAvailableHosts')}
              </div>
            ) : (
              <div className='space-y-2'>
                {/* Host list with checkboxes / 带复选框的主机列表 */}
                <div className='border rounded-md divide-y max-h-[150px] overflow-y-auto'>
                  {availableHosts.map((host) => {
                    const isSelected = selectedHostIds.has(host.id);
                    return (
                      <div
                        key={host.id}
                        className={`flex items-center gap-3 p-2 cursor-pointer hover:bg-muted/50 transition-colors ${
                          isSelected ? 'bg-primary/10' : ''
                        }`}
                        onClick={() => toggleHostSelection(host.id)}
                      >
                        <div
                          className={`w-4 h-4 rounded border flex items-center justify-center ${
                            isSelected ? 'bg-primary border-primary' : 'border-muted-foreground'
                          }`}
                        >
                          {isSelected && (
                            <CheckCircle2 className='h-3 w-3 text-primary-foreground' />
                          )}
                        </div>
                        <Server className='h-4 w-4 text-muted-foreground' />
                        <span className='font-medium'>{host.name}</span>
                        {host.ip_address && (
                          <span className='text-muted-foreground text-sm'>({host.ip_address})</span>
                        )}
                      </div>
                    );
                  })}
                </div>
                {/* Discover button / 发现按钮 */}
                <Button
                  variant='outline'
                  onClick={handleDiscoverProcesses}
                  disabled={selectedHostIds.size === 0 || discoveryLoading}
                  className='w-full'
                >
                  {discoveryLoading ? (
                    <Loader2 className='h-4 w-4 mr-2 animate-spin' />
                  ) : (
                    <Search className='h-4 w-4 mr-2' />
                  )}
                  {t('discovery.discoverProcessesOnHosts', {count: selectedHostIds.size})}
                </Button>
              </div>
            )}
            <p className='text-xs text-muted-foreground'>{t('discovery.discoverMultiHostHint')}</p>
          </div>

          {/* Discovered Processes by Host / 按主机分组的发现进程 */}
          {discoveredProcessesByHost.size > 0 && (
            <div className='space-y-2'>
              <div className='flex items-center justify-between'>
                <Label>{t('discovery.discoveredProcesses')}</Label>
                <div className='flex items-center gap-1'>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-6 px-2 text-xs text-muted-foreground hover:text-foreground'
                    onClick={selectAllProcesses}
                  >
                    {t('discovery.selectAllProcesses')}
                  </Button>
                  <Button
                    type='button'
                    variant='ghost'
                    size='sm'
                    className='h-6 px-2 text-xs text-muted-foreground hover:text-foreground'
                    onClick={deselectAllProcesses}
                  >
                    {t('discovery.deselectAllProcesses')}
                  </Button>
                </div>
              </div>
              <div className='border rounded-md max-h-[250px] overflow-y-auto'>
                {Array.from(discoveredProcessesByHost.entries()).map(([hostId, processes]) => (
                  <div key={hostId} className='border-b last:border-b-0'>
                    {/* Host header / 主机标题 */}
                    <div className='bg-muted/50 px-3 py-2 flex items-center justify-between sticky top-0 z-10'>
                      <div className='flex items-center gap-2'>
                        <Server className='h-4 w-4 text-muted-foreground' />
                        <span className='font-medium text-sm'>{getHostName(hostId)}</span>
                        <span className='text-xs text-muted-foreground'>
                          ({processes.length} {t('discovery.processes')})
                        </span>
                      </div>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        className='h-6 px-2 text-xs text-muted-foreground hover:text-foreground'
                        onClick={(e) => {
                          e.stopPropagation();
                          toggleHostProcesses(hostId);
                        }}
                      >
                        {t('discovery.selectAllOnHost')}
                      </Button>
                    </div>
                    {/* Processes / 进程列表 */}
                    <div className='divide-y'>
                      {processes.map((proc) => {
                        const key = getProcessKey(hostId, proc);
                        const isSelected = selectedProcesses.has(key);
                        return (
                          <div
                            key={key}
                            className={`flex items-center gap-3 p-3 cursor-pointer hover:bg-muted/50 transition-colors ${
                              isSelected ? 'bg-primary/10' : ''
                            }`}
                            onClick={() => toggleProcessSelection(key)}
                          >
                            <div
                              className={`w-5 h-5 rounded border flex items-center justify-center flex-shrink-0 ${
                                isSelected ? 'bg-primary border-primary' : 'border-muted-foreground'
                              }`}
                            >
                              {isSelected && (
                                <CheckCircle2 className='h-3 w-3 text-primary-foreground' />
                              )}
                            </div>
                            <Cpu className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                            <div className='flex-1 min-w-0'>
                              <div className='flex items-center gap-2 flex-wrap'>
                                <span className='font-medium'>PID: {proc.pid}</span>
                                <span
                                  className={`text-xs px-2 py-0.5 rounded-full ${
                                    proc.role === 'master'
                                      ? 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
                                      : proc.role === 'worker'
                                        ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
                                        : 'bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300'
                                  }`}
                                >
                                  {proc.role}
                                </span>
                                {proc.version && proc.version !== 'unknown' && (
                                  <span className='text-xs px-2 py-0.5 rounded-full bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-300'>
                                    v{proc.version}
                                  </span>
                                )}
                              </div>
                              <p className='text-xs text-muted-foreground truncate mt-1'>
                                📁 {proc.install_dir}
                              </p>
                              {/* Port info / 端口信息 */}
                              <div className='flex items-center gap-3 mt-1 text-xs text-muted-foreground'>
                                {proc.hazelcast_port > 0 && (
                                  <span className='flex items-center gap-1'>
                                    <span className='w-2 h-2 rounded-full bg-blue-500'></span>
                                    Hazelcast: {proc.hazelcast_port}
                                  </span>
                                )}
                                {proc.role !== 'worker' && proc.api_port > 0 && (
                                  <span className='flex items-center gap-1'>
                                    <span className='w-2 h-2 rounded-full bg-green-500'></span>
                                    API: {proc.api_port}
                                  </span>
                                )}
                              </div>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
              <p className='text-xs text-muted-foreground'>
                {t('discovery.selectedCount', {count: selectedProcesses.size})}
              </p>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant='outline' onClick={() => handleClose(false)} disabled={loading}>
            {t('common.cancel')}
          </Button>
          <Button onClick={handleSubmit} disabled={loading}>
            {loading && <Loader2 className='h-4 w-4 mr-2 animate-spin' />}
            {t('common.create')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
