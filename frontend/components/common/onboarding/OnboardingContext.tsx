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

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import {usePathname} from 'next/navigation';
import {useAuth} from '@/hooks/use-auth';
import services from '@/lib/services';
import {NodeStatus} from '@/lib/services/cluster/types';
import {getSyncJobClusterId} from '@/components/common/sync/sync-studio-utils';
import {
  completedIdsFromResults,
  evaluateAllSteps,
  isHostAgentReady,
  isPathFullyComplete,
  resolveOnboardingTargetCluster,
  shouldBindOnboardingTargetCluster,
  shouldEvaluateOnboardingProgress,
  stepsForPath,
  type JobSnapshot,
} from '@/lib/onboarding/completion';
import {
  classifyOnboardingFetchError,
  fetchAllPages,
} from '@/lib/onboarding/fetch-all';
import {
  ONBOARDING_RESOURCES_CHANGED_EVENT,
} from '@/lib/onboarding/events';
import {
  loadOnboardingState,
  saveOnboardingState,
} from '@/lib/onboarding/storage';
import {
  DEFAULT_ONBOARDING_STATE,
  type OnboardingDisplayMode,
  type OnboardingPath,
  type OnboardingPersistedState,
  type StepCheckResult,
} from '@/lib/onboarding/types';

/** Poll while guide is active; keep ≤5s freshness for resource returns (AC5). */
const POLL_MS = 5000;

type OnboardingContextValue = {
  ready: boolean;
  state: OnboardingPersistedState;
  stepResults: StepCheckResult[];
  completedIds: string[];
  checking: boolean;
  lastError: string | null;
  selectPath: (path: OnboardingPath) => void;
  setDisplayMode: (mode: OnboardingDisplayMode) => void;
  resumeGuide: () => void;
  /** Record cluster the user actually opened/created / 记录用户实际打开或创建的集群 */
  bindTargetCluster: (clusterId: number) => void;
  clearTargetCluster: () => void;
  refresh: () => Promise<void>;
};

const OnboardingContext = createContext<OnboardingContextValue | null>(null);

/**
 * Provides per-user onboarding state, persistence, and real resource aggregation.
 * 提供按用户上手状态、持久化与真实资源聚合。
 */
export function OnboardingProvider({children}: {children: ReactNode}) {
  const {user, isAuthenticated, isLoading} = useAuth();
  const userId = user?.id;
  const pathname = usePathname();
  const [state, setState] = useState<OnboardingPersistedState>({
    ...DEFAULT_ONBOARDING_STATE,
  });
  const [ready, setReady] = useState(false);
  const [stepResults, setStepResults] = useState<StepCheckResult[]>([]);
  const [checking, setChecking] = useState(false);
  const [lastError, setLastError] = useState<string | null>(null);
  const stateRef = useRef(state);
  stateRef.current = state;
  /** Monotonic generation so stale async refresh cannot overwrite newer results. */
  const refreshGenRef = useRef(0);

  useEffect(() => {
    if (isLoading) {
      return;
    }
    if (!isAuthenticated || !userId) {
      setState({...DEFAULT_ONBOARDING_STATE});
      setReady(false);
      return;
    }
    setState(loadOnboardingState(userId));
    setReady(true);
  }, [isAuthenticated, isLoading, userId]);

  const persist = useCallback(
    (next: OnboardingPersistedState) => {
      const cleaned: OnboardingPersistedState = {
        version: 2,
        path: next.path,
        displayMode: next.displayMode,
        targetClusterId: next.targetClusterId,
        finished: next.finished,
        completedStepIds: next.completedStepIds.filter((id) => id !== 'proxy'),
      };
      setState(cleaned);
      stateRef.current = cleaned;
      if (userId) {
        saveOnboardingState(userId, cleaned);
      }
    },
    [userId],
  );

  const patch = useCallback(
    (partial: Partial<OnboardingPersistedState>) => {
      const current = stateRef.current;
      persist({...current, ...partial, version: 2});
    },
    [persist],
  );

  /**
   * Stable binder: skip no-op writes so ClusterDetail effect cannot loop.
   * 稳定绑定函数：跳过无变更写入，避免 ClusterDetail effect 死循环。
   */
  const bindTargetCluster = useCallback(
    (clusterId: number) => {
      if (
        !shouldBindOnboardingTargetCluster(
          stateRef.current.targetClusterId,
          clusterId,
        )
      ) {
        return;
      }
      // Only record clusters the user actually opened/created.
      // 仅记录用户实际打开或创建的集群。
      patch({targetClusterId: clusterId});
    },
    [patch],
  );

  const refresh = useCallback(async () => {
    const current = stateRef.current;
    // AC4 finished latch: skip re-evaluation so offline/scale-out cannot roll back.
    // AC4 完成闩锁：跳过重评，避免离线/扩容回退完成态。
    if (!ready || !shouldEvaluateOnboardingProgress(current)) {
      setStepResults([]);
      return;
    }
    const gen = ++refreshGenRef.current;
    setChecking(true);
    setLastError(null);
    try {
      let hostsFetchFailed = false;
      let clusterStatusFailed = false;
      let packagesFetchFailed = false;
      let pluginsFetchFailed = false;

      const packagesResult = await services.installer
        .listPackages()
        .then((data) => ({ok: true as const, data}))
        .catch((error: unknown) => ({
          ok: false as const,
          error: classifyOnboardingFetchError(error, 'packages_fetch_failed'),
        }));

      const localPluginsResult = await services.plugin
        .listLocalPlugins()
        .then((data) => ({ok: true as const, data}))
        .catch((error: unknown) => ({
          ok: false as const,
          error: classifyOnboardingFetchError(error, 'plugins_fetch_failed'),
          data: [] as Awaited<
            ReturnType<typeof services.plugin.listLocalPlugins>
          >,
        }));

      const hostsResult = await fetchAllPages((page) =>
        services.host
          .getHosts({current: page.current, size: page.size})
          .then((data) => ({
            total: data.total ?? 0,
            items: data.hosts || [],
          })),
      )
        .then((items) => ({ok: true as const, items}))
        .catch((error: unknown) => ({
          ok: false as const,
          error: classifyOnboardingFetchError(error, 'hosts_fetch_failed'),
          items: [] as Awaited<
            ReturnType<typeof services.host.getHosts>
          >['hosts'],
        }));

      const clustersResult = await fetchAllPages((page) =>
        services.cluster
          .getClusters({current: page.current, size: page.size})
          .then((data) => ({
            total: data.total ?? 0,
            items: data.clusters || [],
          })),
      )
        .then((items) => ({ok: true as const, items}))
        .catch((error: unknown) => ({
          ok: false as const,
          error: classifyOnboardingFetchError(error, 'clusters_fetch_failed'),
          items: [] as {id: number; name: string}[],
        }));

      // Drop stale response after newer refresh started / 丢弃被更新一轮抢先的结果
      if (gen !== refreshGenRef.current) {
        return;
      }

      if (!packagesResult.ok) {
        packagesFetchFailed = true;
        setLastError(packagesResult.error);
      }
      if (!localPluginsResult.ok) {
        pluginsFetchFailed = true;
        setLastError(localPluginsResult.error);
      }
      if (!hostsResult.ok) {
        hostsFetchFailed = true;
        setLastError(hostsResult.error);
      }
      if (!clustersResult.ok) {
        setLastError(clustersResult.error);
      }

      const packages = packagesResult.ok ? packagesResult.data : {local_packages: []};
      const localPlugins = localPluginsResult.ok ? localPluginsResult.data : [];
      const hosts = (hostsResult.ok ? hostsResult.items : []).map((h) => ({
        id: h.id,
        name: h.name,
        isOnline: !!h.is_online,
        agentReady: isHostAgentReady(h),
      }));
      const clusters = (clustersResult.ok ? clustersResult.items : []).map(
        (c) => ({id: c.id, name: c.name}),
      );

      const resolved = resolveOnboardingTargetCluster(current, clusters);
      if (resolved.clearStaleBinding && current.targetClusterId != null) {
        // Clear stale id without inventing a multi-cluster pick.
        // 清理失效绑定，不在多集群场景擅自写入。
        patch({
          targetClusterId:
            resolved.clusterId != null && clusters.length === 1
              ? resolved.clusterId
              : null,
        });
      }

      let targetNodes: {
        hostId: number;
        hostName: string;
        isOnline: boolean;
        processRunning: boolean;
      }[] = [];
      let clusterHealthyEnough = false;
      let jobs: JobSnapshot[] = [];

      if (resolved.clusterId != null) {
        try {
          const status = await services.cluster.getClusterStatus(
            resolved.clusterId,
          );
          if (gen !== refreshGenRef.current) {
            return;
          }
          targetNodes = (status.nodes || []).map((n) => ({
            hostId: n.host_id,
            hostName: n.host_name,
            isOnline: !!n.is_online,
            processRunning: n.status === NodeStatus.RUNNING,
          }));
          const healthy = String(status.health_status) === 'healthy';
          const allRunning =
            targetNodes.length > 0 &&
            targetNodes.every((n) => n.processRunning && n.isOnline);
          clusterHealthyEnough = healthy || allRunning;
        } catch (error) {
          clusterStatusFailed = true;
          targetNodes = [];
          clusterHealthyEnough = false;
          if (gen === refreshGenRef.current) {
            setLastError(
              classifyOnboardingFetchError(error, 'cluster_status_failed'),
            );
          }
        }

        try {
          const jobList = await services.sync.listJobs({
            current: 1,
            size: 50,
          });
          if (gen !== refreshGenRef.current) {
            return;
          }
          jobs = (jobList.items || []).map((job) => ({
            id: job.id,
            clusterId: getSyncJobClusterId(job),
            status: String(job.status || ''),
          }));
        } catch {
          jobs = [];
        }
      }

      if (gen !== refreshGenRef.current) {
        return;
      }

      const localPackages = (packages.local_packages || []).map((p) => ({
        version: p.version,
        isLocal: !!p.is_local,
      }));

      const latest = stateRef.current;
      // Refresh entry already requires a path; re-check after awaits (race-safe).
      // 入口已要求有 path；await 后再确认一次，避免竞态下 path 被清空。
      const path = latest.path || current.path;
      if (!path) {
        setStepResults([]);
        return;
      }
      const results = evaluateAllSteps({
        path,
        state: latest,
        localPackages,
        localPlugins: (localPlugins || []).map((p) => ({
          name: p.name,
          version: p.version,
          selectedProfileKeys: p.selected_profile_keys,
        })),
        hosts,
        clusters,
        targetNodes,
        resolvedTargetClusterId: resolved.clusterId,
        targetAmbiguous: resolved.ambiguous,
        clusterHealthyEnough,
        jobs,
        hostsFetchFailed,
        clusterStatusFailed,
        packagesFetchFailed,
        pluginsFetchFailed,
      });

      setStepResults(results);
      const completed = completedIdsFromResults(results);

      if (isPathFullyComplete(results) && !latest.finished) {
        persist({
          ...latest,
          finished: true,
          displayMode: 'hidden',
          completedStepIds: completed,
          // Persist the resolved target only when evaluation used a concrete id.
          // 仅在评估使用了明确目标时写入。
          targetClusterId:
            latest.targetClusterId ?? resolved.clusterId ?? null,
        });
      } else if (
        !latest.finished &&
        latest.completedStepIds.join('|') !== completed.join('|')
      ) {
        persist({
          ...latest,
          completedStepIds: completed,
        });
      }
    } catch (error) {
      if (gen === refreshGenRef.current) {
        setLastError(classifyOnboardingFetchError(error, 'generic'));
      }
    } finally {
      if (gen === refreshGenRef.current) {
        setChecking(false);
      }
    }
  }, [persist, patch, ready]);

  // Poll + re-check on route return / focus / resource-changed (AC5).
  // 轮询；从业务页返回、获焦、资源变更时重查（AC5）。
  useEffect(() => {
    // AC4: finished guide must not poll resources / AC4：已完成引导不得再轮询资源
    if (
      !ready ||
      !shouldEvaluateOnboardingProgress({
        path: state.path,
        finished: state.finished,
      })
    ) {
      return;
    }
    void refresh();
    const timer = window.setInterval(() => {
      void refresh();
    }, POLL_MS);
    const onVisible = () => {
      if (document.visibilityState === 'visible') {
        void refresh();
      }
    };
    const onFocus = () => {
      void refresh();
    };
    const onResourcesChanged = () => {
      void refresh();
    };
    document.addEventListener('visibilitychange', onVisible);
    window.addEventListener('focus', onFocus);
    window.addEventListener(
      ONBOARDING_RESOURCES_CHANGED_EVENT,
      onResourcesChanged,
    );
    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', onVisible);
      window.removeEventListener('focus', onFocus);
      window.removeEventListener(
        ONBOARDING_RESOURCES_CHANGED_EVENT,
        onResourcesChanged,
      );
    };
  }, [ready, state.path, state.finished, state.targetClusterId, pathname, refresh]);

  const value = useMemo<OnboardingContextValue>(
    () => ({
      ready,
      state,
      stepResults,
      completedIds: completedIdsFromResults(stepResults),
      checking,
      lastError,
      selectPath: (path) =>
        patch({
          path,
          displayMode: 'expanded',
          finished: false,
          completedStepIds: [],
          targetClusterId: null,
        }),
      setDisplayMode: (mode) => {
        // finished 闩锁禁止因资源轮询自动弹开；用户仍可主动收起/隐藏/由 Profile 恢复展开。
        // Finished latch blocks auto-reopen from resource polling; user may still collapse/hide/resume via Profile.
        patch({displayMode: mode});
      },
      resumeGuide: () => {
        patch({displayMode: 'expanded'});
      },
      bindTargetCluster,
      clearTargetCluster: () => patch({targetClusterId: null}),
      refresh,
    }),
    [
      ready,
      state,
      stepResults,
      checking,
      lastError,
      patch,
      bindTargetCluster,
      refresh,
    ],
  );

  return (
    <OnboardingContext.Provider value={value}>
      {children}
    </OnboardingContext.Provider>
  );
}

/**
 * Access onboarding context; throws outside provider.
 * 读取上手上下文；在 Provider 外抛错。
 */
export function useOnboarding(): OnboardingContextValue {
  const ctx = useContext(OnboardingContext);
  if (!ctx) {
    throw new Error('useOnboarding must be used within OnboardingProvider');
  }
  return ctx;
}

/**
 * Optional hook when provider may be absent (e.g. login page).
 * Provider 可能不存在时的可选钩子（如登录页）。
 */
export function useOnboardingOptional(): OnboardingContextValue | null {
  return useContext(OnboardingContext);
}

export function useOnboardingStepOrder(path: OnboardingPath | null) {
  return useMemo(() => (path ? stepsForPath(path) : []), [path]);
}
