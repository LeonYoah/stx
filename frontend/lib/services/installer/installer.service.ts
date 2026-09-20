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

/**
 * SeaTunnel Installer Service
 * SeaTunnel 安装管理服务
 */

import apiClient from '../core/api-client';
import {localizeBackendText} from '@/lib/i18n/localize-text';
import type {
  AvailableVersions,
  PackageInfo,
  PrecheckRequest,
  PrecheckResult,
  InstallationRequest,
  InstallationStatus,
  ListPackagesResponse,
  GetPackageInfoResponse,
  UploadChunkResponse,
  DeletePackageResponse,
  PrecheckResponse,
  InstallResponse,
  DownloadTask,
  DownloadRequest,
  DownloadResponse,
  DownloadListResponse,
  MirrorSource,
  RuntimeStorageValidationRequest,
  RuntimeStorageValidationResponse,
  RuntimeStorageValidationResult,
} from './types';

const API_PREFIX = '';
const DEFAULT_UPLOAD_CHUNK_SIZE = 8 * 1024 * 1024; // 8MB

// ==================== Package Management 安装包管理 ====================

/**
 * List available packages and versions
 * 获取可用安装包和版本列表
 */
export async function listPackages(): Promise<AvailableVersions> {
  const response = await apiClient.get<ListPackagesResponse>(`${API_PREFIX}/packages`);
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Get package info by version
 * 根据版本获取安装包信息
 */
export async function getPackageInfo(version: string): Promise<PackageInfo> {
  const response = await apiClient.get<GetPackageInfoResponse>(
    `${API_PREFIX}/packages/${version}`
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Upload offline package
 * 上传离线安装包
 */
export async function uploadPackage(
  file: File,
  version: string,
  sourceFile?: File,
  onProgress?: (percent: number) => void,
): Promise<PackageInfo> {
  if (!file || file.size <= 0) {
    throw new Error('上传文件不能为空 / Upload file is empty');
  }

  const totalSize = file.size;
  const totalChunks = Math.ceil(totalSize / DEFAULT_UPLOAD_CHUNK_SIZE);
  const uploadID = createChunkUploadID(version);
  let finalPackage: PackageInfo | null = null;

  onProgress?.(0);

  for (let chunkIndex = 0; chunkIndex < totalChunks; chunkIndex += 1) {
    const start = chunkIndex * DEFAULT_UPLOAD_CHUNK_SIZE;
    const end = Math.min(start + DEFAULT_UPLOAD_CHUNK_SIZE, totalSize);
    const chunkBlob = file.slice(start, end);

    const formData = new FormData();
    formData.append('file', chunkBlob, file.name);
    formData.append('version', version);
    formData.append('upload_id', uploadID);
    formData.append('chunk_index', String(chunkIndex));
    formData.append('total_chunks', String(totalChunks));
    formData.append('total_size', String(totalSize));
    formData.append('file_name', file.name);

    const response = await apiClient.post<UploadChunkResponse>(
      `${API_PREFIX}/packages/upload/chunk`,
      formData,
      {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      },
    );
    if (response.data.error_msg) {
      throw new Error(response.data.error_msg);
    }

    const result = response.data.data;
    if (!result) {
      throw new Error('分片上传返回为空 / Empty chunk upload response');
    }

    const isLastChunk = chunkIndex === totalChunks - 1;
    if (isLastChunk && result.completed && result.package) {
      finalPackage = result.package;
      onProgress?.(100);
      continue;
    }

    const percent = Math.floor(((chunkIndex + 1) / totalChunks) * 100);
    onProgress?.(Math.min(99, percent));
  }

  if (!finalPackage) {
    throw new Error('分片上传未完成，请重试 / Chunk upload did not complete');
  }

  if (sourceFile) {
    finalPackage = await uploadSourcePackage(version, sourceFile);
  }

  return finalPackage;
}

function createChunkUploadID(version: string): string {
  const sanitizedVersion = version.replace(/[^0-9A-Za-z_-]/g, '_');
  const randomPart = Math.random().toString(36).slice(2, 10);
  return `upload_${Date.now()}_${sanitizedVersion}_${randomPart}`.slice(0, 120);
}

/** Delete a local runtime package through the web console. / 通过网页控制台删除本地运行包。 */
export async function deletePackage(version: string): Promise<void> {
  const response = await apiClient.delete<DeletePackageResponse>(`${API_PREFIX}/packages/${version}`);
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
}

/** Upload or replace the source archive associated with a runtime package. / 上传或替换运行包关联的源码包。 */
export async function uploadSourcePackage(version: string, sourceFile: File): Promise<PackageInfo> {
  const formData = new FormData();
  formData.append('source_file', sourceFile, sourceFile.name);
  const response = await apiClient.post<GetPackageInfoResponse>(
    `${API_PREFIX}/packages/${version}/source/upload`,
    formData,
    {headers: {'Content-Type': 'multipart/form-data'}},
  );
  if (response.data.error_msg || !response.data.data) {
    throw new Error(response.data.error_msg || '源码包上传返回为空 / Empty source upload response');
  }
  return response.data.data;
}

/** Fetch source from a remote mirror. / 从远端镜像补充源码包。 */
export async function fetchSourcePackage(version: string, mirror: MirrorSource = 'apache'): Promise<PackageInfo> {
  const response = await apiClient.post<GetPackageInfoResponse>(
    `${API_PREFIX}/packages/${version}/source/fetch`,
    {mirror},
  );
  if (response.data.error_msg || !response.data.data) {
    throw new Error(response.data.error_msg || '源码包下载返回为空 / Empty source fetch response');
  }
  return response.data.data;
}

/** Download source through the authenticated STX API. / 通过已认证的 STX API 下载源码包。 */
export async function downloadSourcePackage(version: string): Promise<void> {
  const response = await apiClient.get<Blob>(`${API_PREFIX}/packages/${version}/source/download`, {responseType: 'blob'});
  const url = URL.createObjectURL(response.data);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = `apache-seatunnel-${version}-src.tar.gz`;
  anchor.click();
  URL.revokeObjectURL(url);
}

// ==================== Precheck 预检查 ====================

/**
 * Run precheck on a host
 * 在主机上运行预检查
 */
export async function runPrecheck(
  hostId: number | string,
  options?: PrecheckRequest
): Promise<PrecheckResult> {
  const response = await apiClient.post<PrecheckResponse>(
    `${API_PREFIX}/hosts/${hostId}/precheck`,
    options || {}
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Validate runtime storage connectivity for checkpoint or IMAP.
 * 校验 checkpoint 或 IMAP 的运行时存储连通性。
 */
export async function validateRuntimeStorage(
  request: RuntimeStorageValidationRequest,
): Promise<RuntimeStorageValidationResult> {
  const response = await apiClient.post<RuntimeStorageValidationResponse>(
    `${API_PREFIX}/installer/runtime-storage/validate`,
    request,
  );
  if (response.data.error_msg) {
    throw new Error(localizeBackendText(response.data.error_msg));
  }
  const result = response.data.data!;
  return {
    ...result,
    warning: localizeBackendText(result.warning),
    hosts: Array.isArray(result.hosts)
      ? result.hosts.map((host) => ({
          ...host,
          message: localizeBackendText(host.message),
        }))
      : [],
  };
}

// ==================== Installation 安装 ====================

/**
 * Start installation on a host
 * 在主机上开始安装
 */
export async function startInstallation(
  hostId: number | string,
  request: Omit<InstallationRequest, 'host_id'>
): Promise<InstallationStatus> {
  const response = await apiClient.post<InstallResponse>(
    `${API_PREFIX}/hosts/${hostId}/install`,
    request
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Get installation status
 * 获取安装状态
 */
export async function getInstallationStatus(hostId: number | string): Promise<InstallationStatus> {
  const response = await apiClient.get<InstallResponse>(
    `${API_PREFIX}/hosts/${hostId}/install/status`
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Retry a failed installation step
 * 重试失败的安装步骤
 */
export async function retryStep(
  hostId: number | string,
  step: string
): Promise<InstallationStatus> {
  const response = await apiClient.post<InstallResponse>(
    `${API_PREFIX}/hosts/${hostId}/install/retry`,
    { step }
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Cancel ongoing installation
 * 取消正在进行的安装
 */
export async function cancelInstallation(hostId: number | string): Promise<InstallationStatus> {
  const response = await apiClient.post<InstallResponse>(
    `${API_PREFIX}/hosts/${hostId}/install/cancel`,
    {}
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

// ==================== Package Download 安装包下载 ====================

/**
 * Start downloading a package to server
 * 开始下载安装包到服务器
 */
export async function startDownload(version: string, mirror?: MirrorSource, withSource = true): Promise<DownloadTask> {
  const request: DownloadRequest = { version, mirror, with_source: withSource };
  const response = await apiClient.post<DownloadResponse>(
    `${API_PREFIX}/packages/download`,
    request
  );
  if (response.data.error_msg && !response.data.data) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Get download status for a version
 * 获取某版本的下载状态
 */
export async function getDownloadStatus(version: string): Promise<DownloadTask> {
  const response = await apiClient.get<DownloadResponse>(
    `${API_PREFIX}/packages/download/${version}`
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * Cancel a download
 * 取消下载
 */
export async function cancelDownload(version: string): Promise<DownloadTask> {
  const response = await apiClient.post<DownloadResponse>(
    `${API_PREFIX}/packages/download/${version}/cancel`,
    {}
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data!;
}

/**
 * List all download tasks
 * 获取所有下载任务
 */
export async function listDownloads(): Promise<DownloadTask[]> {
  const response = await apiClient.get<DownloadListResponse>(
    `${API_PREFIX}/packages/downloads`
  );
  if (response.data.error_msg) {
    throw new Error(response.data.error_msg);
  }
  return response.data.data || [];
}

// ==================== Version Management 版本管理 ====================

interface RefreshVersionsResponse {
  error_msg?: string;
  data: string[];
}

/**
 * Refresh version list from Apache Archive
 * 从 Apache Archive 刷新版本列表
 */
export async function refreshVersions(): Promise<{ versions: string[]; warning?: string }> {
  const response = await apiClient.post<RefreshVersionsResponse>(
    `${API_PREFIX}/packages/versions/refresh`,
    {}
  );
  return {
    versions: response.data.data || [],
    warning: response.data.error_msg,
  };
}

// ==================== Export all functions 导出所有函数 ====================

export const installerService = {
  // Package management / 安装包管理
  listPackages,
  getPackageInfo,
  uploadPackage,
  deletePackage,
  uploadSourcePackage,
  fetchSourcePackage,
  downloadSourcePackage,
  // Package download / 安装包下载
  startDownload,
  getDownloadStatus,
  cancelDownload,
  listDownloads,
  // Version management / 版本管理
  refreshVersions,
  // Precheck / 预检查
  runPrecheck,
  validateRuntimeStorage,
  // Installation / 安装
  startInstallation,
  getInstallationStatus,
  retryStep,
  cancelInstallation,
};
