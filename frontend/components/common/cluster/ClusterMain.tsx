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

import {useState, useEffect, useCallback} from 'react';
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
import {Separator} from '@/components/ui/separator';
import {toast} from 'sonner';
import {Plus, Search, Network, RefreshCw} from 'lucide-react';
import {WorkspaceHeader} from '@/components/common/layout';
import {motion} from 'motion/react';
import {easeOut} from 'motion';
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
          icon={<Network />}
          title={t('cluster.title')}
          subtitle={t('cluster.description')}
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

      <Separator />

      {/* Filters / 过滤器 - 左对齐 */}
      <motion.div
        className='flex flex-wrap gap-4 items-end'
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

        <Select value={filterStatus} onValueChange={setFilterStatus}>
          <SelectTrigger className='w-[150px]'>
            <SelectValue placeholder={t('cluster.status')} />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>{t('cluster.allStatuses')}</SelectItem>
            <SelectItem value={ClusterStatus.CREATED}>
              {t('cluster.statuses.created')}
            </SelectItem>
            <SelectItem value={ClusterStatus.DEPLOYING}>
              {t('cluster.statuses.deploying')}
            </SelectItem>
            <SelectItem value={ClusterStatus.RUNNING}>
              {t('cluster.statuses.running')}
            </SelectItem>
            <SelectItem value={ClusterStatus.STOPPED}>
              {t('cluster.statuses.stopped')}
            </SelectItem>
            <SelectItem value={ClusterStatus.ERROR}>
              {t('cluster.statuses.error')}
            </SelectItem>
          </SelectContent>
        </Select>

        <Select value={filterDeploymentMode} onValueChange={setFilterDeploymentMode}>
          <SelectTrigger className='w-[150px]'>
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

        <Button variant='outline' onClick={handleSearch}>
          <Search className='h-4 w-4 mr-2' />
          {t('common.search')}
        </Button>

        <Button variant='ghost' onClick={handleClearFilters}>
          {t('common.clearFilters')}
        </Button>
      </motion.div>

      {/* Cluster Cards / 集群卡片 */}
      <motion.div variants={itemVariants}>
        {loading ? (
          <div className='text-center py-12 text-muted-foreground'>
            {t('common.loading')}
          </div>
        ) : clusters.length === 0 ? (
          <div className='text-center py-12 text-muted-foreground'>
            {t('cluster.noClusters')}
          </div>
        ) : (
          <div className='grid grid-cols-[repeat(auto-fill,minmax(400px,400px))] gap-5'>
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
        <motion.div
          className='flex items-center justify-between'
          variants={itemVariants}
        >
          <div className='text-sm text-muted-foreground'>
            {t('common.totalItems', {total})}
          </div>
          <div className='flex gap-2'>
            <Button
              variant='outline'
              size='sm'
              disabled={currentPage === 1}
              onClick={() => handlePageChange(currentPage - 1)}
            >
              {t('common.previous')}
            </Button>
            <span className='flex items-center px-4 text-sm'>
              {currentPage} / {totalPages}
            </span>
            <Button
              variant='outline'
              size='sm'
              disabled={currentPage === totalPages}
              onClick={() => handlePageChange(currentPage + 1)}
            >
              {t('common.next')}
            </Button>
          </div>
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
