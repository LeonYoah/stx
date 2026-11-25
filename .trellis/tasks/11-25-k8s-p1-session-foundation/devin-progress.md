# Devin 实施进度记录（工作笔记，非交付物）

> 用于上下文压缩后快速恢复现场。分支 `feat/k8s-p1-session-foundation`。

## 已完成
- go.mod 引入 k8s.io/client-go v0.37.1 + api/apimachinery v0.37.1（prometheus 保持 v0.316.0）
- `internal/utils/secretbox`（AES-256-GCM，enc:v1: 前缀，明文兼容）+ 单测全绿；`internal/config/config.go` init 中 `secretbox.SetSecret(Config.App.SessionSecret)`
- `internal/pkg/k8s` 全部实现并单测全绿：
  - types.go：InfraType/ManagementSource/AccessMode/JavaProxyMode 常量，SessionConfig（ApplyDefaults/Validate 端口契约 1-65535 且互不相同/ParseSessionConfig JSON 往返）
  - client_manager.go：Credentials/AuthMode(kubeconfig|token|in_cluster)、ClientManager（指纹缓存失效、NewClientManagerWithFactory 可注入 fake）、BuildRESTConfig、PrecheckRBAC（SelfSubjectAccessReview 覆盖 pods/sts/deploy/svc(含patch)/cm/secrets/jobs/pods/proxy/pods/log，纯只读）、ServerVersion、ListNamespaces
  - transport.go：Transport（ServiceBaseURL/PodBaseURL apiserver_proxy→`{host}/api/v1/namespaces/{ns}/services|pods/http:{name}:{port}/proxy`、in_cluster、external；RoundTripper apiserver_proxy 用 rest.TransportFor；HTTPClient）
  - discovery.go：Discoverer 配对 master/worker、labels/configmap/services 关联
  - session_render.go：seatunnel.yaml(http port=master_http_port)/hazelcast-master|worker.yaml(rest-api.enabled:false + kubernetes service-dns 发现)/hazelcast-client/jvm/log4j2 模板
  - session_deploy.go：SessionManager.Deploy（Namespace/RBAC/ConfigMap/Headless svc×2/Master+Worker StatefulSet PID1 直接 command、terminationGracePeriodSeconds 120、master readiness /overview、worker tcp 5801）幂等 upsert；Uninstall 按确定性命名删除、绝不删 PVC；ResourceNamesFor `<code>-master/-worker/-configs`
  - session_ops.go：SessionPods/ListPodsByLabel/Scale/GetReplicas/RolloutRestart/UpgradeImage/PatchProxyImage(strategic merge 按第一容器名)/ReadConfigMap/WriteConfigMapFile/PodLogs/GetWorkloadStatus
  - java_proxy.go：JavaProxyManager.EnsureProxy（jar CM binaryData+sha256 注解、resolveTargetPodTemplate 克隆、cloneProxyVolumes 白名单 `/opt/seatunnel/{connectors,plugins,lib,config,starter}`+checkpoint、跳过 RWO PVC、克隆 RWX/ROX PVC 与插件 initContainers、java command 直接 exec、readiness /healthz）/DeleteProxy/GetProxyStatus；ProxyJarLoader func 注入；ProxyResourceNames `stx-java-proxy-<code>` + `stx-java-proxy-jar-<code>`
- host model：新增 k8s_auth_mode/k8s_ca_cert/k8s_insecure_skip_tls 列；HostInfo 加 has_k8s_kubeconfig/has_k8s_token（无明文）；Create/Update request 对齐
- host service：创建加密落库、更新留空保留原值、非空重新加密（secretbox）

## 进行中（Step3 剩余）
- host handler：test-connection（纯只读 RBAC 预检+版本）、GET k8s/namespaces、GET k8s/discover 接口 + 凭据解密 loader 注入 K8sClientManager；host 需要构造全局 ClientManager 供各域复用（建议 router.go 组装，loader 读 host repo + secretbox.Decrypt）
- cluster model：infra_type/k8s_host_id/auto_registered/parent_cluster_id/source_job_instance_id + migrator + ToClusterInfo + scope 过滤（默认 managed 排除 auto_registered=true）
- cluster service/handler：K8s 创建（adopted/stx_deployed）+ 伴生 Proxy 自动拉起+healthz 探活、双校验健康、scale/rollout-restart/upgrade-image 接口、删除边界（adopted 只删 Proxy、stx_deployed 勾选才删）、WebUI 代理、Pod 投影节点

## 待办
- Step4 sync/monitoring/config/plugin 接入；Step5 CLI+前端；Step6 云端 e2e（minikube seatunnel 集群纳管 + seatunnel-stx-e2e 一键部署）；质量检查+推送 PR #208
- stx-java-proxy jar：需编译 tools/stx-java-proxy 得 v2/v3 jar 放 lib/（无 maven，需安装；e2e 前必须解决）
- 质量规范：新文件 ASF License 头、注释中英双语中文在前只写 why、不新增「中文 / English」拼接文案、CLI 对齐 internal/cmd、数组字段非 nil

## 关键事实
- minikube API: https://192.168.49.2:8443，kubeconfig 在 ~/.kube/config（minikube 上下文）
- 云端 seatunnel ns：sts seatunnel-master(1/1)+seatunnel-worker(2/2)，svc seatunnel(5801 headless)/seatunnel-master(8080 headless)/seatunnel-worker，cm seatunnel-configs，镜像 apache/seatunnel:3.1.0
- apiserver_proxy 已实测可用：GET /api/v1/namespaces/seatunnel/services/http:seatunnel-master:8080/proxy/overview 200
- 检查命令：go test ./... ; (cd agent && go test ./...) ; cd frontend && pnpm exec tsc --noEmit && pnpm test
- STX API：go run main.go api :17800，admin/admin123，前端 pnpm dev :3000

## 追加进度（第二轮）
- host/k8s.go：K8sCredentialsLoaderFromRepository + TestK8sConnection（版本+RBAC 预检，成功才刷新版本/在线）+ ListK8sNamespaces + DiscoverK8sSeaTunnelClusters；handler 注入 k8sManager
- router.go：k8sClientManager 唯一实例 + k8sJarLoader（lib/stx-java-proxy-{epoch}.jar）+ k8sHostTypeChecker；host 路由 test-connection/k8s/namespaces/k8s/discover；cluster 路由 POST /clusters/kubernetes + /:id/k8s/{scale,rollout-restart,upgrade-image,pods,pods/:pod/logs,java-proxy/retry,configmap(GET/PUT :file)}
- cluster model：InfraType/K8sHostID/AutoRegistered/ParentClusterID/SourceJobInstanceID 列 + IsKubernetes/K8sSessionConfig/effectiveInfraType + ClusterFilter.Scope（managed 默认排 auto_registered）；repository scope 过滤；handler Scope 绑定
- cluster/k8s.go：CreateKubernetes（stx_deployed 先 Deploy 含 CreateRBAC，失败回滚记录）+ EnsureK8sJavaProxy（90s healthz 宽限，失败带 PodEvents 不阻断）+ Delete/Uninstall/Scale/RolloutRestart/UpgradeImage（同步滚动 Proxy+更新 image 落库）+ Pods/PodLogs/ConfigMap 读写 + 全部 HTTP handler
- cluster Delete：K8s 边界（adopted 仅删 Proxy；stx_deployed 复用 forceRemoveInstallDir 作显式勾选才 Uninstall；失败 log 不阻断）
- sync engine_client：EngineEndpoint.Transport 字段 + clientForEndpoint（12 个 Do 调用点已换）+ resolver K8s 分支（resolveK8sEndpoints：ServiceBaseURL master service + RoundTripper）+ router 注入 SetK8sClientManager
- 全部编译通过；go test cluster/host 绿

## 待办（优先级序）
1. sync GetJobLogs K8s 分支（PodLogs 替代文件日志）+ 工作台拦截 K8s 集群 local 模式
2. monitoring /metrics 经 K8sTransport（参考 engine_client 同 pattern；availability_evaluator 跳过 agent_offline for k8s）
3. config 模块：K8s 集群走 ConfigMap（cluster 层已有读写接口，config 域接分支）
4. plugin 只读：K8s 集群拒绝 install/uninstall 写操作 + 列表走伴生 proxy
5. seatunnel/capabilities.go K8s 能力位
6. CLI internal/cmd（host kubernetes 参数 + cluster kubernetes 子命令）+ 单测
7. 前端（主机 K8s 表单/集群创建向导/详情适配/工作台隐藏 local）
8. 编译 stx-java-proxy jar → lib/（maven 需装；e2e 必须）
9. e2e AC-2/3/4 + 全量检查 + 推送
