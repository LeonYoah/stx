# 实现清单

按一期 → 二期顺序。每一段结束后跑对应测试，再进入下一段。

## 0. Review gate

- [x] PRD / Design 已根据画布方案写完（一期 CLI + 二期推送）
- [x] `python3 ./.trellis/scripts/task.py start 10-16-dolphin-shell-integration`（本文件评审后执行）
- [x] 开发分支 `cursor/dolphin-shell-integration-0e25`，base `main`

## 一期 · CLI 能跑

### 1. 提交契约与 `--wait`

- [x] `POST /api/v1/sync/tasks/:id/submit` 接受 `variables` / `external_ref` / `schedule_time`
- [x] `GetJob` 刷新引擎状态；CLI `--wait` 轮询该接口，不再打 300 秒到 execution wait
- [x] 批任务等终态；实时任务等 running（作业 JSON 的 `mode` 从 `submit_spec` 回填）
- [x] `--timeout` 到期 cancel 后退出 8
- [x] 修正 preview / recover 的同样 `--wait` 错误成功
- [x] 登记表补字段；`stx sync task submit --help` 与 CLI 测试更新
- [x] 验证：`go test ./internal/cmd/ ./internal/apps/sync/ ./internal/cli/...`

### 2. `--var` 与 `--schedule-time`

- [x] 覆盖本次变量，不写回任务
- [x] 值仍是 `${…}` → 400 / 退出 2
- [x] `buildTaskVariableRuntime` 使用 `schedule_time` 作 `ReferenceTime`
- [x] 验证：`go test ./internal/apps/sync/ -run Variable`

### 3. `external_ref` 接管与按 ref 停止

- [x] 作业表字段 + 迁移
- [x] 提交时按运行中 / 停止中 / 终态分流
- [x] `POST /api/v1/sync/jobs/cancel-by-ref` + `attach_generation`
- [x] CLI `--external-ref`；`stx sync job cancel` 保持按 id
- [x] 验证：服务层单测覆盖接管、旧 generation 忽略、终态重新提交

### 4. `--close-job` 与守护进程

- [x] 同步默认 `--close-job`；SIGINT/SIGTERM 停作业
- [x] 提交前 double-fork + setsid 守护进程 + 管道
- [x] EOF 无 done → cancel-by-ref 重试 30 分钟
- [x] 作业已终态不再停；409/404 停止重试
- [x] 实时任务不启用 close-job
- [x] 验证：守护进程与信号处理单测（可在 Linux CI 跑 fork 路径）

### 5. 认证

- [x] CLI 读 `STX_SERVER` / `STX_TOKEN`
- [x] 调度令牌 kind、可空过期、bound_task_id；能力收窄
- [x] 页面发放/吊销（可与绑定同屏，二期完善 UI）
- [x] 验证：`go test ./internal/apps/auth/ ./internal/cli/...`

### 6. 复制节点脚本

- [x] `GET /api/v1/sync/tasks/:id/dolphin/script` 生成脚本（无绑定时 ref 模板仍用 Dolphin 占位符）
- [x] 任务调度侧边栏「复制节点脚本」
- [x] `stx sync task dolphin script <id>`
- [x] i18n zh/en
- [x] 验证：脚本生成单测；前端复制按钮有文案

## 二期 · 能推

### 7. Dolphin 客户端与连接

- [x] 调度器连接存储（URL、token 脱敏）
- [x] 客户端：按版本切换 process/workflow 路径；gen-task-codes、GET/PUT 定义、release、schedule online
- [x] 用 httptest 模拟 3.2 与 3.3 路径

### 8. 绑定与推送 API

- [x] bindings 表
- [x] push new / reuse / update；reuse 环检测；version 冲突 409
- [x] 推送成功关内置定时
- [x] unbind 仅页面
- [x] `stx sync task dolphin push`
- [x] 验证：`go test` 覆盖环、冲突、update 不改边、dry-run

### 9. 推送对话框与侧边栏

- [x] 对话框：新建 / 复用、上下游、覆盖变量折叠、成环禁止
- [x] 已绑定：脚本 Diff、只更新节点
- [x] 侧边栏：绑定信息、最近 Dolphin 触发作业、打开 Dolphin、更新、解绑、内置定时禁用
- [x] 对照 `ui-conventions.md`：弹窗尺寸、footer、零塌陷、密钥脱敏
- [x] i18n zh/en
- [ ] 浏览器验证主路径：打开对话框 → 复用勾选上下游 → dry-run/确认推送（无真 Dolphin 时用 mock / 脚本生成）——本环境无运行中的 STX 控制台与 Dolphin，改用 httptest + tsc + vitest（含翻译覆盖）代替

## 全量验证

```text
go test ./internal/cmd/ ./internal/cli/... ./internal/apps/sync/ ./internal/apps/auth/ ./internal/operation/
cd frontend && pnpm exec tsc --noEmit
```

有浏览器工具时验证：复制脚本、推送对话框两种模式、已绑定更新、侧边栏解绑。无真 Dolphin 时服务端用 httptest，前端至少走到对话框与脚本预览。

对照 `.trellis/spec/guides/cross-layer-thinking-guide.md`：submit 正文、作业 status、CLI 退出码、绑定字段前后端一致。

## 回滚点

- 一期只合 CLI / 作业字段时：前端复制脚本可先不上；旧节点没有 `--wait` 修复则行为与现在一样不可靠。
- 二期推送失败应在 Dolphin 下线后、PUT 前因 version 冲突停止；PUT 后上线失败要在错误里写出工作流已下线，避免静默停定时。
