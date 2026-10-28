# 作业计划 DAG（运行作业的表结构与建表计划）

> 适用于 `internal/apps/sync/plan_dag.go` 与 `GET /api/v1/sync/jobs/:id/plan-dag`。前端消费方式见
> [前端：作业详情与 Flink 风格拓扑](../frontend/api-and-services.md#scenario-作业详情与-flink-风格拓扑)。

## 1. 范围 / 触发条件

- 引擎 `job-info` 返回的运行时 `job_dag`（存于 `result_preview.job_dag`）每个节点只有 `vertexId`、`type`、`vertexName`、
  `tablePaths`，**没有** `tableColumns` / `tableSchemas` / `saveModePreviews`。作业详情要展示列信息和建表计划，只能用
  作业提交时的配置重新推导。
- 修改提交内容的保存位置（`submit_spec.submitted_content` / `submitted_format`）、config tool 的 WebUI DAG 接口，或这个接口的原因码时，必须同步本文。

## 2. 签名

```go
// GET /api/v1/sync/jobs/:id/plan-dag    权限：作业可见（getJobInstanceForActor）   风险：R0
func (s *Service) GetJobPlanDAGForActor(ctx context.Context, actor executionapp.Actor, id uint) (*JobPlanDAGResult, error)

type JobPlanDAGResult struct {
    JobID     uint     `json:"job_id"`
    Available bool     `json:"available"`
    Reason    string   `json:"reason"`              // available=true 时为空
    Message   string   `json:"message,omitempty"`   // 只在 inspect_failed 时出现，已脱敏、按字符截断 500
    WebUIJob  JSONMap  `json:"webui_job,omitempty"` // 与 POST /tasks/:id/dag 的 webui_job 同构
    Warnings  []string `json:"warnings"`            // 永远是数组
}
```

CLI：`stx sync job plan-dag <id>`（`internal/operation/registry_sync.go` 登记 `sync.job.plan-dag`，生成式命令）。

## 3. 契约

- 输入只取作业自己的记录：`submit_spec.submitted_content`、`submitted_format`（缺省 `format`，经 `normalizeSubmitFormat`）、
  集群取 `clusterIDForJob`；任务定义只用于 `proxy_base_url` 覆盖，**不**读任务当前草稿。
- 调用 `configToolClient.InspectWebUIDAG`，结果用 `structToJSONMap` 原样放进 `webui_job`（WebUI 兼容 `jobDag`），前端与编辑器 DAG 共用一套解析。
- 不缓存、不写回 `result_preview`：提交内容不可变，线上实测 config tool 调用约 0.2s；`result_preview` 同时被对账器写，回写会产生并发覆盖。前端按作业在会话内缓存并提供刷新。
- 语义：表结构按**当前**库表解析（config tool 连库读 catalog）；建表计划同样对照**当前**数据库状态推导——作业建过表之后再看，
  `CREATE_SCHEMA_WHEN_NOT_EXIST` 会变成 `INFO_ONLY`（无动作）。界面必须标注“只读、按当前库表状态推导，可能与作业启动时不同”。
- 引擎侧事实（SeaTunnel 3.0.0 PG CDC，2026-10-28 实测）：删列直接让作业失败（SE-07，只支持 ADD COLUMN）；运行中新增列未被识别时，
  新列的值会被静默丢弃。所以运行中作业的真实结构以「事件」页签的 schema change 记录为准，计划 DAG 只能作参考。
- 节点对应：线上作业 #26 实测运行时与计划 DAG 的 `vertexId`、`tablePaths` 一致，但 `pipelineEdges` 的 pipeline 键不同（"1" vs "0"），所以按顶点合并，不按 pipeline 键。

## 4. 校验与原因对应表

| 情况 | 结果 |
|---|---|
| 作业不存在 / 不可见 | 404 / 403（`getStatusCodeForError`） |
| `submitSpecExecutionMode == local` | 200，`reason=local_mode` |
| `submitted_content` 为空（旧作业、提交前失败） | 200，`reason=no_submitted_content` |
| config tool 未注入，或 `ResolveConfigToolEndpoint` 失败（含集群已删除） | 200，`reason=config_tool_unavailable` |
| `InspectWebUIDAG` 报错或返回 nil | 200，`reason=inspect_failed`，`message` = `audit.RedactText` 后截断 |
| 成功 | 200，`available=true`，`webui_job` 有值，config tool 的 warnings 透传 |

## 5. Good / Base / Bad

- Good：CDC 作业运行中打开详情，抽屉“表结构”显示 4 列与主键，Sink“建表计划”显示 SaveMode 动作或“不支持”的说明。
- Base：较早的作业没有 `submitted_content`，拓扑与指标照常显示，抽屉一行说明“无法解析表结构”。
- Bad：直接用任务当前草稿推导——任务改过版本后，详情会显示与该作业实际执行不一致的表结构。

## 6. 必须有的测试

- `plan_dag_test.go`：成功时请求带提交内容与格式、`webui_job.jobDag.vertexInfoMap` 保留 `tableColumns`、warnings 透传；
  四个原因码各一例；`warnings` 永不为 nil；inspect 错误里的 `password=secret` 被屏蔽。
- `operation/validate_test.go` 生成式命令集合含 `sync.job.plan-dag`；路由基线 `route_baseline.json` 与 Swagger 同步（`swag init` + `go run ./internal/operation/cmd/contract-report -write`）。

## 7. 错误与正确示例

#### 错误
```go
// 失败直接返回 500：前端只能弹错误，拓扑也跟着不可用；错误原文还可能带出连接密码。
if err != nil {
    return nil, err
}
```

#### 正确
```go
// 失败用原因码表达，拓扑照常可用；错误文本先脱敏再截断。
if err != nil || resp == nil {
    result.Reason = PlanDAGReasonInspectFailed
    if err != nil {
        result.Message = truncatePlanDAGMessage(audit.RedactText(strings.TrimSpace(err.Error())))
    }
    return result, nil
}
```
