# E2 执行计划：每日 / 每月 / 指定时间读写量

1. [x] `embedts.OldestSampleTime()` + 单测（blocks + head 取最小，空返回 false）。
2. [x] `internal/apps/dashboard/volume.go`：VolumeService（桶切分 / totals / split 名称解析）+ VolumeHandler + 测试。
3. [x] 路由 `GET /dashboard/home/volume`、operation `dashboard.home.volume`、swagger、路由基线。
4. [x] 前端 `home.service.ts`：`getVolume` + 类型。
5. [x] `HomeDashboard` 拉 volume 喂 `DashboardKpiRow` 第 7 张卡；`JobsTab` 加「数据量」面板（指标 / 粒度 / 拆分三开关 + beUI BarChart stacked）；i18n zh/en。
6. [x] `go test ./internal/...`、`cd frontend && pnpm exec tsc --noEmit && pnpm test`（相关测试）；spec 自检；提交。

## 验证

- 本地起服务跑演示作业后核对：`stx dashboard home volume --granularity day` 桶值与作业「累计写入」差值一致（抽样）。
- 窗口起点早于最早样本：前端显示「数据自 … 起」而非 0。
- 短批作业（<15s）计入日 / 月统计（依赖内嵌 P2 终值补写，验收核对）。

## 实施记录

- 2026-11-16：全链路实现完成。
  - 后端：`embedts.OldestSampleTime`（blocks.Meta().MinTime 与 head.MinTime 取最小）；`volume.go` 每桶一条 `increase(counter[Ns])` 即时查询（design.md 记录了为何不用 step=1d 的 range query：显示时区跨 DST 时日长 ≠86400s）；`sum by (cluster_id|task_id)` 拆分；totals 恒全局聚合；失败桶记 `partial_errors` 不拖垮整包。
  - 权限：`split=task` 走新增的 `sync.ResolveTaskNamesForActor`（`CanUserView` 过滤），无权限任务名不下发、前端回退 `#id`，与 Top N 的 viewable 口径一致；聚合总量保持全局口径。
  - CLI：`stx dashboard home volume --granularity day --split cluster` 由 operation 注册生成；route_baseline + swagger 已重生成（`make swagger` + `contract-report -write`），validate_test 注册数 278→279。
  - 前端：KPI 第 7 卡（窗口写入行数 + 「今日 X · 本月 Y」，月起早于最早样本时显「自 M/D 起」）；JobsTab 数据量面板独立拉取，粒度 / 拆分变化重新请求、指标四选一本地切换；partial 桶标 `*` 脚注；自定义窗口不轮询。
  - 测试：Go 10+ 用例（日 / 月 / DST 切桶、PromQL 拼装、拆分命名、降级、partial_errors、非法参数、OldestSampleTime）；前端 622 用例全过（新增 KPI 与 volume 面板断言）。`internal/apps/installer` 两个上传用例因沙箱跨设备 rename 失败，为环境既有问题，与本次无关。
