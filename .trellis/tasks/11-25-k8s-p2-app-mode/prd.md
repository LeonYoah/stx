# P2：K8s App 模式（STIP-41 原生实现）与临时集群治理

> 父任务：[11-25-k8s-integration](../11-25-k8s-integration/prd.md)
> 完整架构契约：[../11-25-k8s-integration/design.md](../11-25-k8s-integration/design.md)
> 前置依赖：**必须先完成并验收 `11-25-k8s-p1-session-foundation`**（依赖 `internal/pkg/k8s`、`K8sTransport`、`K8sJavaProxyManager` 与 `Cluster` 扩展列）。

## 1. 目标

在 P1 公共 K8s 底座之上，基于 Apache SeaTunnel 上游 STIP-41 协议（及 `LeonYoah/seatunnel#6` Master HTTP Service 发布机制），交付纯 Go 原生 **Kubernetes App 模式**，包含 App 模板（保存即拉起常驻伴生 Proxy）、自动登记临时集群实例（长跑实时任务进集群管理页运维 + 4 条防污染隔离边界）、双层融合对账、Savepoint & `restore-job-id` 恢复、以及双层定期清理与一键回收。

## 2. 核心需求

1. **App 模板（`kubernetes_application_template`）与运行时配置三选一**
   - 注册/保存模板时配置 K8s Host、Namespace、ServiceAccount、默认 SeaTunnel 镜像、Checkpoint PVC/远程存储、默认 Master/Worker 资源与 Slot、Hazelcast 端口（`master_port`，默认 `5801`）与可选 Master HTTP Service 发布端口（`master_http_port`，留空/`0` 不发布；填值须 `1–65535` 且 `!= master_port`，引擎实际监听仍完全由 Pod 内 `seatunnel.yaml` 决定，且校验 Master SA 具备 `services: patch` 权限）、临时集群保留策略（默认批 24h / 流 7d），保存即自动拉起常驻伴生 `stx-java-proxy`（支持 `reuse_cluster` 复用）。
   - **运行时配置文件来源支持三选一（`runtime_config.mode`）**：
     - `image_default`（默认，零配置）：直接沿用镜像内置 `/opt/seatunnel/config/*`；
     - `stx_managed`（STX 托管，推荐）：伴生 `stx-java-proxy` 暴露只读 `GET /api/v1/config/defaults` 读取当前镜像内真实默认文件作基线，用户按需定制 `seatunnel.yaml` / `jvm_options` / `log4j2.properties`（支持 `${VAR}` 占位符 + `secretKeyRef` 注入实现 ConfigMap 零明文）；保存模板时 STX 自动在目标 Namespace 维护专属 ConfigMap（`stx-app-cfg-<code>`），提交时仅对已修改文件按 `subPath` 挂载（不遮蔽镜像其余默认文件）；单作业特殊微调支持创建带 `OwnerReference -> Job` 的作业级临时 ConfigMap，结束自动 GC；
     - `external_ref`（引用外部 ConfigMap）：供 Helm/GitOps 团队复用已有 ConfigMap，STX 仅做提交前存在性/key 校验与只读预览，绝不修改外部对象。
2. **纯 Go STIP-41 原生提交器（`K8sApplicationRunner`）**
   - 原子顺序：校验引用 ConfigMap/SA → 建 `suspend: true, backoffLimit: 0, ttlSecondsAfterFinished` 的 owner Job（Pod 不预声明 HTTP port）→ 建 OwnerRef Headless Service（**初始仅声明 Hazelcast 端口 `5801`**）+ Application Secret（承载 `job.conf` 防凭据泄露与 96KB 限制）→ 解除挂起；任一步失败原地回滚补偿。
   - 序列化 `SEATUNNEL_APPLICATION_SPECIFICATION` 为 `Base64(UTF-8 Properties)`；当模板/任务配置了 `master_http_port` 时写入 `application.master.http-port`（绝不覆盖 `seatunnel.yaml`，Master 启动绑定 Jetty 后自行 JSON Patch Service 追加 `http` 端口并将 `port -> targetPort` 映射至真实监听端口）；完整持久化元数据到 `JobInstance.SubmitSpec`。
   - 支持工作台 `Task.Definition["k8s_application"]` 任务级参数覆盖（Worker 数、Slot、CPU/Mem、独立镜像、超时、NodeSelector、Tolerations、Env/SecretRef）。
3. **自动登记临时集群（`kubernetes_application_instance`, `auto_registered=true`）与 4 条硬隔离边界**
   - 每次提交 App 作业自动登记临时集群（`parent_cluster_id` 指向模板、`source_job_instance_id` 指向作业），共享父模板的常驻 Proxy；
   - `/clusters` 视图隔离：默认只看受管集群与模板，独立分段「App 运行实例」展示临时集群（优先运行中/实时任务）并支持进入 `/clusters/[id]` 完整控制台；
   - 工作台目标集群下拉框严格排除 `auto_registered=true` 实例；
   - `sync_job_instances.cluster_id` 永锚父 App 模板 ID，保证定期清理临时集群后历史作业/趋势/告警零断链；
   - 告警与大盘隔离：模板与批/终态临时实例不触发集群可用性误报。
4. **双层融合对账、停止/Savepoint、恢复、日志与双层 GC**
   - 按 `(k8s_host_id, namespace)` 批量 `ListJobs + ListPods` 权威状态对账；当 `master_http_port > 0` 时，在 `DEPLOYING` 阶段平滑容忍 Master 尚未自 Patch Service `http` 端口前的 404/503，就绪后经 `K8sTransport` 访问 `services/http:<appId>:<master_http_port>/proxy/job-info/{jobId}` 融合 DAG/指标/Checkpoint 与单作业 WebUI 代理；失败/卡住时自动采集 K8s Events 归因。
   - 普通取消删 owner Job + label 兜底清 Worker；Savepoint 停止（需开启 Master HTTP + 持久化 Checkpoint）先调 `POST /stop-job(savepoint=true)` 待 Job Complete 落 `savepoint_done`；**恢复必须生成新 `jobId` + 注入 `application.restore-job-id`**。
   - PodLogs 支持 Master/Worker 角色切换与增量拉取。
   - 后台 `EphemeralClusterCleaner` 按策略定期清理已终态临时集群记录 + 前端/CLI 一键清理残留 K8s 资源与过期临时记录。

## 3. 验收标准

- [ ] **AC-1**：STIP-41 Go 资源组装与 Properties 编码有 Golden Test 对齐 SeaTunnel 上游 `KubernetesApplicationResourcesTest` 契约。
- [ ] **AC-2**：在云端 Minikube `seatunnel-apps` Namespace 创建 App 模板，0 作业状态下伴生 Proxy 就绪，工作台测试连接/插件表单/校验可用；验证运行时配置三选一（`image_default`、`stx_managed` 经伴生 Proxy 载入镜像默认 `seatunnel.yaml` 并仅 `subPath` 挂载改动文件、`external_ref` 引用校验）。
- [ ] **AC-3**：提交批作业与长跑实时作业，自动登记临时集群且视图隔离生效；可从「App 运行实例」进控制台看 Pod、日志、K8s Events、Master HTTP 指标/WebUI。
- [ ] **AC-4**：验证 Savepoint 停止与 `restore-job-id` 恢复（新 `jobId` != 原 `jobId`）成功接续状态；验证定时/手动清理临时集群后 `sync_job_instances` 历史零断链。
- [ ] **AC-5**：CLI 与前端表单/列表/详情补齐，全量单测、`-race` 与 i18n 覆盖测试全绿。
