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
 * Cluster Management Main Component
 * 集群管理主组件
 *
 * This component provides the main interface for cluster management,
 * including listing, searching, filtering, and CRUD operations.
 * 本组件提供集群管理的主界面，包括列表、搜索、过滤和 CRUD 操作。
 */

import {useState, useEffect, useCallback, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {toast} from 'sonner';
import {Plus, Search, Layers, Server, RefreshCw} from 'lucide-react';
import {WorkspaceHeader, StatPillsBar, TableLoadingBar, ModuleNavTabs} from '@/components/common/layout';
import {Pagination} from '@/components/ui/pagination';
import {motion} from 'motion/react';
import {easeOut} from 'motion';
import {useGSAP} from '@gsap/react';
import {animateGridCards} from '@/lib/animations/gsap-motion';
import services from '@/lib/services';
import {
  ClusterInfo,
  ClusterStatus,
  DeploymentMode,
  ListClustersRequest,
} from '@/lib/services/cluster/types';
import {ClusterCard} from './ClusterCard';
import {CreateClusterDialog} from './CreateClusterDialog';
import {EditClusterDialog} from './EditClusterDialog';
import {ClusterDeployWizard} from './ClusterDeployWizard';

const PAGE_SIZE = 12;

/**
 * Skeleton card for cluster grid loading state
 * 集群卡片骨架屏（防止网格布局塌陷）
 */
function ClusterCardSkeleton() {
  return (
    <div className='border rounded-xl bg-card/40 p-5 min-h-[320px] flex flex-col justify-between shadow-xs animate-pulse'>
      <div className='space-y-4'>
        <div className='flex items-start justify-between'>
          <div className='space-y-2 flex-1'>
            <div className='h-5 w-36 bg-muted rounded-md'></div>
            <div className='h-3.5 w-48 bg-muted/60 rounded-md'></div>
          </div>
          <div className='h-6 w-16 bg-muted rounded-full'></div>
        </div>
        <div className='space-y-2.5 pt-3'>
          <div className='h-3 w-full bg-muted/50 rounded-md'></div>
          <div className='h-3 w-4/5 bg-muted/50 rounded-md'></div>
          <div className='h-3 w-2/3 bg-muted/50 rounded-md'></div>
        </div>
      </div>
      <div className='pt-4 border-t border-border/40 flex items-center justify-between'>
        <div className='h-4 w-28 bg-muted/60 rounded-md'></div>
        <div className='h-8 w-20 bg-muted rounded-md'></div>
      </div>
    </div>
  );
}

/**
 * Cluster Management Main Component
 * 集群管理主组件
 */
export function ClusterMain() {
  const t = useTranslations();

  // Data state / 数据状态
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [currentPage, setCurrentPage] = useState(1);

  // Filter state / 过滤状态
  const [searchName, setSearchName] = useState('');
  const [filterStatus, setFilterStatus] = useState<string>('all');
  const [filterDeploymentMode, setFilterDeploymentMode] = useState<string>('all');

  // Dialog state / 对话框状态
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [isDeployWizardOpen, setIsDeployWizardOpen] = useState(false);
  const [isEditDialogOpen, setIsEditDialogOpen] = useState(false);
  const [selectedCluster, setSelectedCluster] = useState<ClusterInfo | null>(null);
  const gridContainerRef = useRef<HTMLDivElement>(null);

  // 集群数据更新后触发平滑交错入场动画
  // Trigger staggered card entrance animation when clusters data updates
  useGSAP(
    () => {
      if (clusters.length > 0 && !loading) {
        animateGridCards('.grid-card-animate');
      }
    },
    {dependencies: [clusters, loading], scope: gridContainerRef},
  );

  /**
   * Load clusters list
   * 加载集群列表
   */
  const loadClusters = useCallback(async () => {
    setLoading(true);
    try {
      const params: ListClustersRequest = {
        current: currentPage,
        size: PAGE_SIZE,
        name: searchName || undefined,
        status: filterStatus !== 'all' ? (filterStatus as ClusterStatus) : undefined,
        deployment_mode:
          filterDeploymentMode !== 'all'
            ? (filterDeploymentMode as DeploymentMode)
            : undefined,
      };

      const result = await services.cluster.getClustersSafe(params);

      if (result.success && result.data) {
        setClusters(result.data.clusters || []);
        setTotal(result.data.total || 0);
      } else {
        toast.error(result.error || t('cluster.loadError'));
        setClusters([]);
        setTotal(0);
      }
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('cluster.loadError'));
      setClusters([]);
      setTotal(0);
    } finally {
      setLoading(false);
    }
  }, [currentPage, searchName, filterStatus, filterDeploymentMode, t]);

  useEffect(() => {
    loadClusters();
  }, [loadClusters]);

  /**
   * Handle search
   * 处理搜索
   */
  const handleSearch = () => {
    setCurrentPage(1);
    loadClusters();
  };

  /**
   * Handle refresh
   * 处理刷新
   */
  const handleRefresh = () => {
    loadClusters();
  };

  /**
   * Handle page change
   * 处理页面变化
   */
  const handlePageChange = (page: number) => {
    setCurrentPage(page);
  };

  /**
   * Handle edit cluster
   * 处理编辑集群
   */
  const handleEdit = (cluster: ClusterInfo) => {
    setSelectedCluster(cluster);
    setIsEditDialogOpen(true);
  };

  /**
   * Handle delete cluster
   * 处理删除集群
   */
  const handleDelete = async (
    cluster: ClusterInfo,
    options?: { forceDelete?: boolean },
  ) => {
    const result = await services.cluster.deleteClusterSafe(cluster.id, options);
    if (result.success) {
      toast.success(t('cluster.deleteSuccess'));
      loadClusters();
    } else {
      toast.error(result.error || t('cluster.deleteError'));
    }
  };

  /**
   * Handle cluster created
   * 处理集群创建完成
   */
  const handleClusterCreated = () => {
    setIsCreateDialogOpen(false);
    loadClusters();
    toast.success(t('cluster.createSuccess'));
  };

  /**
   * Handle cluster updated
   * 处理集群更新完成
   */
  const handleClusterUpdated = () => {
    setIsEditDialogOpen(false);
    loadClusters();
    toast.success(t('cluster.updateSuccess'));
  };

  /**
   * Clear all filters
   * 清除所有过滤条件
   */
  const handleClearFilters = () => {
    setSearchName('');
    setFilterStatus('all');
    setFilterDeploymentMode('all');
    setCurrentPage(1);
  };

  const totalPages = Math.ceil(total / PAGE_SIZE);

  const containerVariants = {
    hidden: {opacity: 0},
    visible: {
      opacity: 1,
      transition: {
        duration: 0.5,
        staggerChildren: 0.1,
        ease: easeOut,
      },
    },
  };

  const itemVariants = {
    hidden: {opacity: 0, y: 20},
    visible: {
      opacity: 1,
      y: 0,
      transition: {duration: 0.6, ease: easeOut},
    },
  };

  return (
    <motion.div
      className='space-y-6'
      initial='hidden'
      animate='visible'
      variants={containerVariants}
    >
      {/* Header / 标题 */}
      <motion.div variants={itemVariants}>
        <WorkspaceHeader
          icon={<Layers />}
          title={t('cluster.title')}
          subtitle={t('cluster.description')}
          tabs={
            <ModuleNavTabs
              items={[
                {
                  key: 'clusters',
                  label: t('cluster.title'),
                  href: '/clusters',
                  icon: <Layers className='size-3.5' />,
                },
                {
                  key: 'hosts',
                  label: t('host.title'),
                  href: '/hosts',
                  icon: <Server className='size-3.5' />,
                },
              ]}
              activeKey='clusters'
            />
          }
          actions={
            <>
              <Button variant='outline' onClick={handleRefresh}>
                <RefreshCw className='h-4 w-4 mr-2' />
                {t('common.refresh')}
              </Button>
              <Button variant='outline' onClick={() => setIsCreateDialogOpen(true)}>
                <Plus className='h-4 w-4 mr-2' />
                {t('cluster.registerCluster')}
              </Button>
              <Button onClick={() => setIsDeployWizardOpen(true)}>
                <Plus className='h-4 w-4 mr-2' />
                {t('cluster.createCluster')}
              </Button>
            </>
          }
        />
      </motion.div>

      {/* Status Pills / 状态胶囊栏 */}
      <motion.div variants={itemVariants}>
        <StatPillsBar
          items={[
            {key: 'all', label: t('cluster.allStatuses')},
            {
              key: ClusterStatus.RUNNING,
              label: t('cluster.statuses.running'),
              variant: 'success',
            },
            {
              key: ClusterStatus.DEPLOYING,
              label: t('cluster.statuses.deploying'),
              variant: 'info',
            },
            {
              key: ClusterStatus.STOPPED,
              label: t('cluster.statuses.stopped'),
              variant: 'default',
            },
            {
              key: ClusterStatus.ERROR,
              label: t('cluster.statuses.error'),
              variant: 'danger',
            },
          ]}
          activeKey={filterStatus}
          onChange={(newStatus) => {
            setFilterStatus(newStatus);
            setCurrentPage(1);
          }}
        />
      </motion.div>

      {/* Filters / 过滤器 */}
      <motion.div
        className='flex flex-wrap gap-3 items-center'
        variants={itemVariants}
      >
        <div className='flex-1 min-w-[200px] max-w-sm'>
          <Input
            placeholder={t('cluster.searchPlaceholder')}
            value={searchName}
            onChange={(e) => setSearchName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
          />
        </div>

        <Select value={filterDeploymentMode} onValueChange={setFilterDeploymentMode}>
          <SelectTrigger className='w-[160px]'>
            <SelectValue placeholder={t('cluster.deploymentMode')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('cluster.allModes')}</SelectItem>
            <SelectItem value={DeploymentMode.HYBRID}>
              {t('cluster.modes.hybrid')}
            </SelectItem>
            <SelectItem value={DeploymentMode.SEPARATED}>
              {t('cluster.modes.separated')}
            </SelectItem>
          </SelectContent>
        </Select>

        <Button variant='outline' onClick={handleSearch} className='active:scale-[0.98]'>
          <Search className='h-4 w-4 mr-2' />
          {t('common.search')}
        </Button>

        {(searchName || filterDeploymentMode !== 'all' || filterStatus !== 'all') && (
          <Button variant='ghost' onClick={handleClearFilters} className='text-muted-foreground hover:text-foreground'>
            {t('common.clearFilters')}
          </Button>
        )}
      </motion.div>

      {/* Cluster Cards / 集群卡片 */}
      <motion.div variants={itemVariants} ref={gridContainerRef} className='space-y-3 relative'>
        <TableLoadingBar loading={loading} />
        {loading && clusters.length === 0 ? (
          <div className='grid grid-cols-[repeat(auto-fill,minmax(400px,400px))] gap-5'>
            {Array.from({length: 6}).map((_, i) => (
              <ClusterCardSkeleton key={i} />
            ))}
          </div>
        ) : clusters.length === 0 ? (
          <div className='text-center py-16 text-muted-foreground border border-dashed rounded-xl bg-card/20'>
            {t('cluster.noClusters')}
          </div>
        ) : (
          <div
            className={`grid grid-cols-[repeat(auto-fill,minmax(400px,400px))] gap-5 transition-opacity duration-200 ${
              loading ? 'opacity-60 pointer-events-none' : ''
            }`}
          >
            {clusters.map((cluster) => (
              <ClusterCard
                key={cluster.id}
                cluster={cluster}
                onEdit={handleEdit}
                onDelete={handleDelete}
                onRefresh={loadClusters}
              />
            ))}
          </div>
        )}
      </motion.div>

      {/* Pagination / 分页 */}
      {totalPages > 1 && (
        <motion.div variants={itemVariants}>
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            pageSize={PAGE_SIZE}
            totalItems={total}
            onPageChange={handlePageChange}
            showPageSizeSelector={false}
          />
        </motion.div>
      )}

      {/* Create Cluster Dialog / 创建集群对话框 */}
      <CreateClusterDialog
        open={isCreateDialogOpen}
        onOpenChange={setIsCreateDialogOpen}
        onSuccess={handleClusterCreated}
      />

      {/* Deploy Cluster Wizard / 部署集群向导 */}
      <ClusterDeployWizard
        open={isDeployWizardOpen}
        onOpenChange={setIsDeployWizardOpen}
        onComplete={() => {
          loadClusters();
          toast.success(t('cluster.wizard.deploySuccess'));
        }}
      />

      {/* Edit Cluster Dialog / 编辑集群对话框 */}
      {selectedCluster && (
        <EditClusterDialog
          open={isEditDialogOpen}
          onOpenChange={setIsEditDialogOpen}
          cluster={selectedCluster}
          onSuccess={handleClusterUpdated}
        />
      )}
    </motion.div>
  );
}
