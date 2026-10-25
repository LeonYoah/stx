# 上游 STIP-41（Zeta Application Mode）现状

> 调研时间：2026-10-24。上游代码基线：apache/seatunnel dev `9ab97e1`（`3.0.1-SNAPSHOT`）。

## 时间线与状态

| 项 | 状态 |
| --- | --- |
| STIP-41 issue #12457 | 2026-10-18 随 #12460 合入关闭 |
| #12460 | **只实现 Kubernetes provider**（173 个文件）；YARN 在 PR 描述中写明"拆到后续 PR"，同时删除了旧的 `thirdparty/yarn/YarnResourceManager` 占位 |
| YARN provider | 截至调研日没有任何 open PR |
| umbrella #11857 | 仍为 open |
| #12898（open） | CLI 修复：`submit` 必须带 `-a`；`--wait` 只有 `FAILED` 才非零退出；cancel 之后 `status` 文档改为 `UNKNOWN` |
| #12943（open） | K8s HA：2 个 master + 1 个同步备份，固定 worker |
| #12930（closed） | K8s Session Cluster，未合入 |
| 已发布版本 | 最新为 3.0.0（2026-09-29），**不含** Application Mode；最早要到 3.0.1 / 3.1.0 |

评审历史：评审人（DanielLeens、SEZ9）一直要求 v1 先做 YARN，最终合入的却是 K8s。LeonYoah 提出参考 `spark.yarn.archive` 预置发行包，被记为 v1 non-goal，留待单独的 STIP（需要覆盖：不可变版本标识、完整性校验、HDFS 权限/Kerberos、缓存淘汰、connector/classloader 升级语义，并且不能混用两个发行版的 jar）。

## 对 STX 有约束的契约（附源码证据）

1. **REST 被强制关闭**
   `seatunnel-engine-common/.../config/SeatunnelApplicationConfig.java` 约 309-312 行：`http.setEnabled(false); http.setEnableHttps(false);`
   同一处还会强制设置：`ConnectorJarStorage` 关闭、静态 slot、`backupCount=0`、TCP 加入方式、master/worker 角色。
2. **一个作业对应一个 application**：1 个 master + 固定数量的 worker；worker 全部注册后才执行作业；结束后释放资源；checkpoint 存储不会被删除。
3. **状态由平台映射**：`CREATED/DEPLOYING/RUNNING/SUCCEEDED/FAILED/CANCELED/UNKNOWN`。
   K8s：cancel 会删除 owner Job，之后查询得到 `UNKNOWN`；超过 `kubernetes.finished-job-retention-seconds`（默认 86400）后也是 `UNKNOWN`。
4. **job ID**：提交方可以用 `--job-id` 预先指定，master 沿用该 ID，不会另外生成。
5. **恢复**：提交一个新的 application，带 `--restore-job-id`（即 `application.restore-job-id`），并访问同一个 checkpoint 存储。
6. **访问方式**：
   - 平台 API：status/cancel，不需要 master 在线；
   - `ClusterDescriptor.retrieve(id)`：拿到的是原生 Hazelcast `SeaTunnelClient`（Java），不是 HTTP 客户端。
7. **CLI**：`bin/seatunnel-application.sh submit|status|cancel -t <target> -c job.conf -a application.config [-i k=v] [--wait]`。输出格式和退出码仍在变化（见 #12898）。
8. **YARN v1（设计稿）**：每次提交上传一个自包含的 `yarn.distribution` 本地归档；Kerberos 不在范围内。
9. **K8s 制品限制**：`kubeclient/factory/AbstractWorkloadFactory.java` 中 `command()` 直接拼出 `java ... -cp <home> <Main>`，**覆盖镜像 ENTRYPOINT**。
   全部 `kubernetes.*` 选项中没有 pod template、init 容器或额外 volume，只有：image、env、configmap/secret 挂载（ConfigMap 上限 1MiB）、checkpoint-pvc、labels/annotations、node-selector/tolerations。
   → **在 K8s 上"按作业只带所需连接器"，没有上游改动时只能按依赖组合构建镜像。**
10. **K8s 标签**：支持 `kubernetes.master.labels` / `worker.labels`，STX 可以靠它批量列出自己的 application。YARN 是否支持 tags 尚不可知（provider 未出）。

## 公开接口（Java）

- `ClusterDescriptor<ID>`：`deployApplication(spec)` / `getApplicationStatus(id)` / `cancelApplication(id)` / `retrieve(id)` / `close()`
- `ApplicationClusterDeployer(clientServiceLoader).run(target, options, specification)`
- 平台 provider 在 `seatunnel-resource-managers/<platform>`，发行包中位于 `resource-managers/<platform>`，按 `-t` 只加载选中的那一个。

## 来源

- https://github.com/apache/seatunnel/issues/12457
- https://github.com/apache/seatunnel/pull/12460
- https://github.com/apache/seatunnel/issues/11857
- 上游文档：`docs/zh/getting-started/application/{cli.md,kubernetes/*}`
