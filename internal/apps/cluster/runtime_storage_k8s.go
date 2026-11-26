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

package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
)

// k8sLocalFileWarning K8s LOCAL_FILE 的统一边界提示：容器内文件控制面不可达且重建即丢。
// k8sLocalFileWarning is the uniform boundary note for LOCAL_FILE on Kubernetes:
// in-container files are unreachable from the control plane and lost on pod recreation.
const k8sLocalFileWarning = "Kubernetes 本地文件存储（LOCAL_FILE）位于 Pod 容器内，不支持跨 Pod 容量统计与文件清理，且 Pod 重建后数据会丢失；生产环境建议切换为 S3 / OSS / HDFS 外部共享存储"

// k8sLocalFileUnsupported 操作级拒绝文案（浏览/预览/反序列化/清理）。
// k8sLocalFileUnsupported is the operation-level rejection for browse/preview/inspect/cleanup.
const k8sLocalFileUnsupported = "Kubernetes 本地文件存储不支持在控制面浏览、预览、反序列化或清理容器内文件，请配置 S3 / OSS / HDFS 外部存储"

// readK8sRuntimeConfigFiles 读取集群 ConfigMap 中的 seatunnel.yaml 与 hazelcast 配置内容。
// readK8sRuntimeConfigFiles reads seatunnel.yaml and the hazelcast config from the cluster ConfigMap.
func (s *Service) readK8sRuntimeConfigFiles(ctx context.Context, clusterObj *Cluster) (seatunnelYAML, hazelcastYAML string, err error) {
	cfg, err := s.requireK8sSession(clusterObj)
	if err != nil {
		return "", "", err
	}
	ops := k8s.NewSessionManager(s.k8sManager)
	files, err := ops.ReadConfigMap(ctx, cfg)
	if err != nil {
		return "", "", err
	}
	seatunnelYAML = files["seatunnel.yaml"]
	// hybrid 模式只有单一 hazelcast.yaml；separated 模式成员配置在 hazelcast-master.yaml。
	// Hybrid mode ships a single hazelcast.yaml while separated mode keeps member config
	// in hazelcast-master.yaml.
	hazelcastYAML = files["hazelcast.yaml"]
	if strings.TrimSpace(hazelcastYAML) == "" {
		hazelcastYAML = files["hazelcast-master.yaml"]
	}
	return seatunnelYAML, hazelcastYAML, nil
}

// applyK8sLocalFileBoundary LOCAL_FILE 规格附加 K8s 边界标志与警告。
// applyK8sLocalFileBoundary marks a LOCAL_FILE spec with the Kubernetes boundary flags.
func applyK8sLocalFileBoundary(spec *RuntimeStorageSpec) {
	if spec == nil || !spec.Enabled || !strings.EqualFold(spec.StorageType, "LOCAL_FILE") {
		return
	}
	spec.SizeAvailable = false
	spec.CleanupSupported = false
	spec.Nodes = nil
	spec.TotalSizeBytes = 0
	if spec.Warning == "" {
		spec.Warning = k8sLocalFileWarning
	}
}

// getK8sRuntimeStorageDetails 从 ConfigMap 解析 K8s 集群的运行时存储详情。
// getK8sRuntimeStorageDetails parses runtime storage details of a Kubernetes cluster
// from its ConfigMap.
func (s *Service) getK8sRuntimeStorageDetails(ctx context.Context, clusterObj *Cluster) (*RuntimeStorageDetails, error) {
	seatunnelYAML, hazelcastYAML, err := s.readK8sRuntimeConfigFiles(ctx, clusterObj)
	if err != nil {
		return nil, err
	}
	details := &RuntimeStorageDetails{ClusterID: clusterObj.ID, ConfigSource: "k8s_configmap"}
	checkpoint := parseCheckpointStorageFromYAML(seatunnelYAML)
	imap := parseIMAPStorageFromYAML(hazelcastYAML)
	if checkpoint == nil {
		checkpoint = &RuntimeStorageSpec{Kind: "checkpoint"}
	}
	if imap == nil {
		imap = &RuntimeStorageSpec{Kind: "imap", StorageType: "DISABLED"}
	}
	applyK8sLocalFileBoundary(checkpoint)
	applyK8sLocalFileBoundary(imap)

	// 外部 S3 存储可以直接用解析出的连接配置做容量统计；其他外部类型暂无控制面统计通道。
	// External S3 storage can be sized directly with the parsed connection config; other
	// external types have no control-plane stat channel yet.
	for _, spec := range []*RuntimeStorageSpec{checkpoint, imap} {
		if spec == nil || !spec.Enabled || !strings.EqualFold(spec.StorageType, "S3") {
			continue
		}
		var resolved *runtimeStorageResolvedConfig
		if spec.Kind == "imap" {
			resolved = parseIMAPResolvedConfigFromYAML(hazelcastYAML)
		} else {
			resolved = parseCheckpointResolvedConfigFromYAML(seatunnelYAML)
		}
		if resolved == nil {
			continue
		}
		if total, statErr := statS3RuntimeStorage(ctx, resolved); statErr == nil {
			spec.TotalSizeBytes = total
			spec.SizeAvailable = true
		} else if spec.Warning == "" {
			spec.Warning = statErr.Error()
		}
	}
	details.Checkpoint = checkpoint
	details.IMAP = imap
	details.ConfiguredCheckpoint = checkpoint
	details.ConfiguredIMAP = imap
	return details, nil
}

// resolveK8sRuntimeStorageResolved 解析 K8s 集群指定存储类型的连接配置（直读 ConfigMap）。
// resolveK8sRuntimeStorageResolved resolves the connection config of one storage kind
// for a Kubernetes cluster straight from the ConfigMap.
func (s *Service) resolveK8sRuntimeStorageResolved(ctx context.Context, clusterObj *Cluster, kind string) (*runtimeStorageResolvedConfig, error) {
	seatunnelYAML, hazelcastYAML, err := s.readK8sRuntimeConfigFiles(ctx, clusterObj)
	if err != nil {
		return nil, err
	}
	if kind == "imap" {
		return parseIMAPResolvedConfigFromYAML(hazelcastYAML), nil
	}
	return parseCheckpointResolvedConfigFromYAML(seatunnelYAML), nil
}

// k8sRuntimeStorageBoundaryError 按存储类型给出 K8s 操作边界错误；S3 返回 nil 表示可直连。
// k8sRuntimeStorageBoundaryError maps a storage type to its Kubernetes operation boundary;
// S3 yields nil because direct access works.
func k8sRuntimeStorageBoundaryError(storageType string) error {
	switch strings.ToUpper(strings.TrimSpace(storageType)) {
	case "", "LOCAL_FILE", "DISABLED":
		return errors.New(k8sLocalFileUnsupported)
	case "S3":
		return nil
	default:
		return errors.New("Kubernetes 集群当前暂不支持通过 HTTP 代理浏览/反序列化该外部存储类型（S3 支持浏览与预览）")
	}
}

// k8sProxyHTTPProbe 经 K8sTransport 调用伴生 Proxy 的 HTTP 探活接口。
// 返回 (success, message)；Proxy 不可用时返回明确错误。
// k8sProxyHTTPProbe calls the companion proxy's HTTP probe endpoint through K8sTransport.
// It returns (success, message); an unavailable proxy yields an explicit error.
func (s *Service) k8sProxyHTTPProbe(ctx context.Context, clusterObj *Cluster, kind string, probeConfig map[string]interface{}) (bool, string, error) {
	cfg, err := s.requireK8sSession(clusterObj)
	if err != nil {
		return false, "", err
	}
	mode := cfg.JavaProxy.Mode
	if mode == k8s.JavaProxyModeDisabled {
		return false, "", errors.New("STX Java Proxy 已禁用，无法校验外部存储连通性")
	}
	targetNs := cfg.Namespace
	_, serviceName, _ := k8s.ProxyResourceNames(clusterObj.EffectiveClusterCode())
	port := cfg.JavaProxy.Port
	if port <= 0 {
		port = 18080
	}
	if mode == k8s.JavaProxyModeReuseCluster && cfg.JavaProxy.ReuseClusterID > 0 {
		if reuse, getErr := s.repo.GetByID(ctx, cfg.JavaProxy.ReuseClusterID, false); getErr == nil && reuse != nil {
			_, reuseSvc, _ := k8s.ProxyResourceNames(reuse.EffectiveClusterCode())
			serviceName = reuseSvc
			if reuseCfg := reuse.K8sSessionConfig(); reuseCfg != nil {
				targetNs = reuseCfg.Namespace
			}
		}
	}
	transport := k8s.NewTransport(s.k8sManager, cfg.HostID, targetNs, cfg.AccessMode, cfg.ExternalBaseURL)
	base, err := transport.ServiceBaseURL(ctx, serviceName, port)
	if err != nil {
		return false, "", fmt.Errorf("STX Java Proxy 不可达，无法校验外部存储连通性: %w", err)
	}
	client, err := transport.HTTPClient(ctx, 20*time.Second)
	if err != nil {
		return false, "", err
	}
	body, _ := json.Marshal(map[string]interface{}{"config": probeConfig, "probe": true})
	resp, err := client.Post(strings.TrimRight(base, "/")+"/api/v1/storage/"+kind+"/probe", "application/json", bytes.NewReader(body))
	if err != nil {
		return false, "", fmt.Errorf("STX Java Proxy 探活请求失败: %w", err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(resp.Body)
	var parsed struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return false, "", fmt.Errorf("STX Java Proxy 探活响应解析失败: %w", err)
	}
	if parsed.Error != "" && parsed.Message == "" {
		parsed.Message = parsed.Error
	}
	return parsed.Success, parsed.Message, nil
}

// validateK8sRuntimeStorage K8s 集群的存储校验：本地文件友好结构化返回，外部存储经 Proxy 探活。
// validateK8sRuntimeStorage validates runtime storage for a Kubernetes cluster: local
// file storage returns a friendly structured outcome, external storage is probed
// through the companion proxy.
func (s *Service) validateK8sRuntimeStorage(ctx context.Context, clusterObj *Cluster, kind installerapp.RuntimeStorageValidationKind) (*installerapp.RuntimeStorageValidationResult, error) {
	result := &installerapp.RuntimeStorageValidationResult{
		Kind:  kind,
		Hosts: []*installerapp.RuntimeStorageValidationHostResult{},
	}
	resolved, err := s.resolveK8sRuntimeStorageResolved(ctx, clusterObj, string(kind))
	if err != nil {
		return nil, err
	}
	storageType := ""
	if resolved != nil {
		storageType = strings.ToUpper(strings.TrimSpace(resolved.StorageType))
	}
	host := &installerapp.RuntimeStorageValidationHostResult{HostID: 0, HostName: "kubernetes"}
	switch storageType {
	case "", "LOCAL_FILE":
		host.Success = true
		host.Message = "Kubernetes 本地文件存储位于 Pod 容器内，无需连通性校验；注意 Pod 重建后数据会丢失"
		result.Success = true
		result.Warning = k8sLocalFileWarning
	case "DISABLED":
		host.Success = true
		host.Message = "IMAP 外部存储已禁用"
		result.Success = true
	default:
		probeConfig := map[string]interface{}{
			"storage.type": storageType,
			"namespace":    resolved.Namespace,
			"endpoint":     resolved.Endpoint,
			"bucket":       resolved.Bucket,
			"access.key":   resolved.AccessKey,
			"secret.key":   resolved.SecretKey,
		}
		success, message, probeErr := s.k8sProxyHTTPProbe(ctx, clusterObj, string(kind), probeConfig)
		if probeErr != nil {
			host.Success = false
			host.Message = probeErr.Error()
			result.Success = false
			result.Warning = probeErr.Error()
		} else {
			host.Success = success
			host.Message = message
			result.Success = success
		}
	}
	result.Hosts = append(result.Hosts, host)
	return result, nil
}

// applyK8sRuntimeStorage K8s 集群的运行时存储应用：读 ConfigMap、外部存储先探活、patch 后写回。
// applyK8sRuntimeStorage applies runtime storage for a Kubernetes cluster: read the
// ConfigMap, probe external storage first, patch the YAML, and write it back.
func (s *Service) applyK8sRuntimeStorage(ctx context.Context, clusterObj *Cluster, kind installerapp.RuntimeStorageValidationKind, req *ApplyRuntimeStorageRequest) (*ApplyRuntimeStorageResult, error) {
	seatunnelYAML, hazelcastYAML, err := s.readK8sRuntimeConfigFiles(ctx, clusterObj)
	if err != nil {
		return nil, err
	}
	targetFile := "seatunnel.yaml"
	content := seatunnelYAML
	if kind == installerapp.RuntimeStorageValidationIMAP {
		if strings.TrimSpace(hazelcastYAML) == "" {
			return nil, errors.New("集群 ConfigMap 中缺少 hazelcast 配置，无法写入 IMAP 存储")
		}
		content = hazelcastYAML
		// hybrid 用 hazelcast.yaml，separated 用 hazelcast-master.yaml：按 ConfigMap 实有文件回写。
		// Hybrid uses hazelcast.yaml while separated uses hazelcast-master.yaml; write back
		// whichever actually exists in the ConfigMap.
		targetFile = "hazelcast-master.yaml"
		if cfg, sessErr := s.requireK8sSession(clusterObj); sessErr == nil {
			if files, readErr := k8s.NewSessionManager(s.k8sManager).ReadConfigMap(ctx, cfg); readErr == nil {
				if strings.TrimSpace(files["hazelcast.yaml"]) != "" {
					targetFile = "hazelcast.yaml"
				}
			}
		}
	}

	// 外部存储先探活：失败直接返回，不写回 ConfigMap；LOCAL_FILE/DISABLED 无需探活。
	// External storage is probed first: a failure returns early without writing back;
	// LOCAL_FILE/DISABLED need no probe.
	storageType := strings.ToUpper(strings.TrimSpace(req.StorageType))
	if req.Enabled && storageType != "" && storageType != "LOCAL_FILE" && storageType != string(installerapp.IMAPStorageDisabled) {
		probeConfig := map[string]interface{}{
			"storage.type": storageType,
			"namespace":    req.Namespace,
			"endpoint":     req.Endpoint,
			"bucket":       req.Bucket,
			"access.key":   req.AccessKey,
			"secret.key":   req.SecretKey,
		}
		success, message, probeErr := s.k8sProxyHTTPProbe(ctx, clusterObj, string(kind), probeConfig)
		if probeErr != nil {
			return nil, probeErr
		}
		if !success {
			return nil, fmt.Errorf("外部存储连通性校验失败，未写入配置: %s", message)
		}
	}

	patched, err := patchRuntimeStorageYAML(content, kind, req)
	if err != nil {
		return nil, err
	}
	if err := s.WriteK8sConfigMapFileByID(ctx, clusterObj.ID, targetFile, patched); err != nil {
		return nil, err
	}
	result := &ApplyRuntimeStorageResult{
		Saved:           true,
		RestartRequired: true,
		Message:         "配置已写入集群 ConfigMap，滚动重启后生效",
	}
	return result, nil
}
