# P1 详细技术设计：K8s 底座、常驻容器模式与伴生 Proxy

> 所属任务：[prd.md](./prd.md) | 父设计：[../11-25-k8s-integration/design.md](../11-25-k8s-integration/design.md)

## 1. 模块与包结构

```text
internal/
├── utils/secretbox/              # AES-256-GCM 凭据加密（enc:v1: 前缀，兼容旧明文）
├── pkg/k8s/
│   ├── client_manager.go         # 按 HostID 缓存 Clientset/RESTConfig、凭据变更失效、SelfSubjectAccessReview
│   ├── transport.go              # K8sTransport: apiserver_proxy / in_cluster / external RoundTripper
│   ├── discovery.go              # 扫描 Namespace 中已有 SeaTunnel StatefulSet/Deployment/Service/CM
│   ├── session_deploy.go         # 一键部署标准 StatefulSet + Headless Svc + ConfigMap + 可选 RBAC + 安全卸载
│   ├── session_ops.go            # Pod 投影 NodeInfo、Scale、RolloutRestart、UpgradeImage、ConfigMap 读写、PodLogs
│   ├── java_proxy.go             # 272KB ConfigMap.binaryData + 同镜像伴生 Deployment/Service 托管与智能卷克隆
│   └── types.go                  # K8s 集群配置解析结构体（与 Cluster.Config["kubernetes"] 一一对应）
├── apps/host/                    # 完善 HostTypeKubernetes 加密/脱敏/TestConnection/Namespaces/Discover
├── apps/cluster/                 # InfraType 模型扩展、K8s 节点投影、健康双校验、WebUI/Proxy/Scale/Rollout/UpgradeImage
├── apps/sync/                    # K8sRuntimeResolver + K8sTransport EngineClient + PodLogs 分支
├── apps/monitoring/              # K8s 8080 /metrics 抓取目标 + K8sTransport + 跳过 agent_offline
├── apps/config/                  # K8s ConfigMap 配置适配（拉取/编辑/版本/推送+可选滚动重启）
└── apps/plugin/                  # K8s 集群只读镜像插件视图（经伴生 stx-java-proxy /api/v1/plugin/list）
```

## 2. 关键接口与边界契约

### 2.1 `K8sTransport` URL 与代理契约
- `apiserver_proxy` 模式：
  - 访问指定 Pod：`{apiServer}/api/v1/namespaces/{ns}/pods/http:{podName}:{port}/proxy`
  - 访问 Service：`{apiServer}/api/v1/namespaces/{ns}/services/http:{svcName}:{port}/proxy`
  - 使用 `rest.TransportFor(restConfig)` 构造带认证与 TLS 的 `http.RoundTripper`，由 `SeaTunnelEngineClient`、`ConfigToolClient`、`embedts` 抓取器和 `ProxyWebUI` 复用。
- `in_cluster` 模式：直连 `http://{podIP}:{port}` 或 `http://{svcName}.{ns}.svc.cluster.local:{port}`。
- `external` 模式：直连用户配置的 `external_base_url`。

### 2.2 伴生 `stx-java-proxy` 智能卷克隆规则
克隆目标 Master/Worker Workload 的 `volumes` & `volumeMounts` 时遵循白名单 + 安全过滤：
- **克隆**：`ConfigMap`、`Secret`、挂载路径命中 `/opt/seatunnel/{connectors,plugins,lib,config}` 或 Checkpoint 目录的 `emptyDir`（连带对应插件 `initContainers`）及 `accessModes` 包含 `ReadWriteMany` / `ReadOnlyMany` 的 PVC。
- **跳过**：StatefulSet `volumeClaimTemplates` 生成的单 Pod `ReadWriteOnce` 数据卷（如 `/tmp/seatunnel`），避免跨节点挂卷死锁。
- **注入**：挂载只读 ConfigMap `stx-java-proxy-jar-<code >` 到 `/opt/stx-proxy/stx-java-proxy.jar`，启动命令固定：
  `java <jvm_opts> -Dstx.java.proxy.port=18080 -DSEATUNNEL_HOME=/opt/seatunnel -Dstx.java.proxy.seatunnel.home=/opt/seatunnel -cp /opt/seatunnel/lib/*:/opt/seatunnel/starter/seatunnel-starter.jar:/opt/stx-proxy/stx-java-proxy.jar io.github.leonyoah.stx.proxy.StxJavaProxyApplication`。

### 2.3 双校验集群健康（解决休眠假死 Pod 问题）
- `evaluateClusterHealth` 遇到 `kubernetes_session` 时：
  1. 先查 K8s Master/Worker Pods 副本数与 `Ready` 条件；
  2. 对 `Ready` 的 Master Pod，经 `K8sTransport` 探测 `/overview`（复用 `engineProbeCache` 10s 缓存防雪崩）；
  3. 若 `Pod.Ready == true` 但 `/overview` 超时/失败，标为 `unhealthy` 并输出明确原因（避免假死 JVM 漏报）。

### 2.4 跨层兼容契约
- `Cluster.ToClusterInfo()` 新增 `infra_type`、`k8s_host_id`、`auto_registered`、`parent_cluster_id`、`source_job_instance_id`，所有数组字段保证非 `nil`（序列化为 `[]`）。
- `GET /api/v1/clusters` 新增可选查询参数 `scope`（默认 `managed`，过滤 `auto_registered=false`；为 P2 预留 `app_instances` 与 `all`）。
- `internal/apps/host/handler.go` 响应强制走脱敏转换，绝不输出 `k8s_kubeconfig` / `k8s_token` 原文。
