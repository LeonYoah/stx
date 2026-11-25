# Review 修复进度（第二轮 — P0/P1/P2）

## 用户最新指令（2026-11-25 18:12）
修复 PR #208 Review 清单全部 P0/P1/P2 + 补齐 P1 收尾，云端 Minikube 复验推送。远端已同步（设计文档更新：Master HTTP 自 Patch 为 Open Discussion、Session SA 不再授 services:patch、InfraType 上移公共层 size:64、extra_mounts 预留）。

## 已完成（未提交）
- P0-1: Uninstall 彻底移除 Namespace 删除逻辑（保留打标但不再使用）✅
- P0-3: RBACRequiredRules 按真实模型拆填 Group/Resource/Subresource/Verb（apps/batch 组、pods/proxy+pods/log+services/proxy 子资源、proxy 加 create、services 去 patch）✅ 单测待补
- P1-8a: Session SA Role 移除 services:patch ✅
- P1-8c: upsertService 更新保留已有 ClusterIP + 同步 PublishNotReadyAddresses ✅
- P1-8d: CreateRBAC 仅当 cfg.ServiceAccount=="" ✅
- P0-2: 新建 internal/pkg/infra 叶子包（Type 常量 + IsK8s/IsK8sSession/Effective，含 legacy "kubernetes" 兼容）+ infra_test.go ✅；pkg/k8s/types.go InfraType 块已删；model.go size:64 + IsKubernetes 用 infra.IsK8s；k8s.go:214 写 infra.TypeK8sSession；SessionConfig.ExtraMounts（ExtraMount: Name/Kind(configmap|secret)/Source/MountPath/ReadOnly/Env）已加 ✅
- 前端：ClusterPlugins infra_type 兼容 kubernetes_* 前缀；types.ts 联合类型拓宽 ✅
- P0-5: K8sClusterCreateDialog 对齐 DiscoveredCluster 真实结构（base_name/master.workload对象/services[]/configmap_name/label_selector/master_http_port/hazelcast_port），pickDiscovered 派生 master svc（含 master 字串）与 cluster svc ✅
- P0-4a: GetK8sPodLogs 越权校验（SessionPods 名单或伴生 Proxy 前缀，否则拒绝）✅

## 进行中（下一个动作）
- P0-4b: 消除 HostTransportRegistry 进程级全局注册表 + sync/config_tool_client.go:567 包级全局变量
  方案已定：
  1. sync 包新增 ConfigToolEndpoint struct{BaseURL string; Transport http.RoundTripper}
  2. ConfigToolResolver 接口与 DefaultConfigToolResolver 返回 *ConfigToolEndpoint（2 处调用：plan_dag.go:98、plugin_template.go:372）
  3. ConfigToolClient 接口 11 个方法 endpoint string → *ConfigToolEndpoint（约 17 个调用点，机械替换）；DefaultConfigToolClient.postJSON 内部用 endpoint.BaseURL 拼 URL，按 endpoint.Transport 选 client（nil 用默认）
  4. DefaultConfigToolResolver 结构体加 k8sManager 字段 + SetK8sManager 方法注入；删包级 k8sConfigToolClientManager 变量与 SetK8sConfigToolClientManager（router.go 调用点改）
  5. pkg/k8s/transport.go 删除 HostTransportRegistry/DefaultHostTransportRegistry/DelegatingRoundTripper 全部及 sync import；NewDefaultConfigToolClient 不再用 DelegatingRoundTripper
  注意：字符串 endpoint 来源（task override proxy_base_url）包装为 &ConfigToolEndpoint{BaseURL: s}

## 第二轮已完成（已提交 45ddf97a + 679e6f15）
- P0 全部 + P0 单测（RBAC 规则断言、cloneProxyVolumes 悬空挂载）
- P0-4b ConfigToolEndpoint 重构完成：registry 全删、resolver 结构体注入、17 调用点机械迁移、测试全绿
- P1-6/7/8abcde/9 后端完成：探活 18s、JavaProxy 四模式分支、StatusCode 检查、cluster.Status 按健康回写、health.go K8s 双校验、webui K8s 代理（proxyK8sWebUI + endpoints 分支）、availability 跳过 agent_offline、seatunnel.InfraCapabilitiesFor、config 域 K8sConfigMapSource 可选接口合成只读视图 + router 接线

## 待办（按序）
- P0 补单测：RBAC 规则逐条断言 Group/Resource/Subresource/Verb
- P1-6: cloneProxyVolumes 悬空 VolumeMount 修复（按 clonedVolumeNames 反过滤 mounts）+ RWO 跳过单测
- P1-7: sync 漏点——service.go:1685 submitNewTask K8s local 拦截（已做？验证）；engine_client.go:910 删 /*UNRESOLVED*/ 用 clientForEndpoint；engine_client_realtime.go:178 同改
- P1-8e: CreateKubernetes 探活改 15-20s 短等待不阻断+proxy_status/PodEvents+cluster.Status 按实际更新；JavaProxyMode 全分支（ReuseCluster/External/Disabled 覆盖 EnsureK8sJavaProxy/UpgradeK8sImage/ListK8sImageConnectors）；ListK8sImageConnectors 查 StatusCode
- P1-8f: 前端 ClusterPlugins K8s 只读彻底隐藏安装/卸载/复选框+不可变镜像提示；K8sClusterCreateDialog 用 Arc Select+i18n 端口标签+DirtyCloseDialog+proxy_status 不健康展示 PodEvents+一键重试
- P1-9 后端: health.go 双校验（Pod Ready + /overview 探活）、webui_master.go K8s 代理、availability_evaluator 跳过 agent_offline、config 域 K8s ConfigMap 分支、capabilities.go K8s 能力位、CLI 新操作单测
- P1-9 前端: EditHostDialog K8s 字段+留空保留提示、test-connection 按钮+RBAC 展示、集群详情 Pod 列表/扩缩容/滚动重启/镜像升级/ConfigMap UI、工作台隐藏 local
- P2: gofmt+import 分组、host/k8s.go:155,184 HTTP 错误码、删本文件、复验 AC-2/3/4（确认卸载不删 Namespace 且 PVC 完好）、全绿推送

## 环境事实
- API server 需重启加载新代码：go run main.go api 端口 17800（杀进程按 ss -tlnp 找 pid）
- minikube host id=1、stx-cookie.txt 在 /tmp
- 原 seatunnel ns 集群完好；seatunnel-stx-e2e ns 已删（可复用名）
