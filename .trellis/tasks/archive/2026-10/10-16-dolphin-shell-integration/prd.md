# STX 通过 Shell 节点接入 DolphinScheduler

## 1. 目标

让 STX 同步任务能被 DolphinScheduler 调度，且停作业可靠。不写 Dolphin 插件。Dolphin 用自带 SHELL 节点执行 `stx` CLI。

用户价值：
- 批任务可以挂进已有 Dolphin 工作流，跟上游依赖、业务日期、失败重试走同一套调度。
- 在 Dolphin 停节点、在 STX 停作业、Worker 被 `kill -9`、机器宕机这几条路上，SeaTunnel 作业都会停或被接管，不会留下孤儿作业（守护进程 30 分钟连不上 STX 除外）。
- STX 任务发布新版本后不用改 Dolphin 节点：脚本按任务编号提交，总是跑最新发布版本。

本任务覆盖一期 CLI 和二期推送，一次交付。

设计原型：[STX × DolphinScheduler：Shell 节点集成原型](https://cursor.com/canvas/store-54b05534-483d-47b3-9a10-0a4f746873dd/41b506cc-8144-45f3-8b5d-29570222df00)

## 2. 已确认事实

- STX 任务配置用 `{{key}}` 在提交时替换；自定义变量存在 `task.definition.custom_variables`。
- `{{system.biz.date}}` 等内置时间变量的基准目前是 `time.Now()`，Dolphin 补数会对不上。
- `stx sync task submit --wait` 把 300 秒传给公共执行等待接口（上限 30 秒），被拒后打印提交结果、退出 0。`stx execution wait` 只读执行表，同步作业状态要有人打开作业页才回写。
- CLI 环境变量目前只读 `STX_USERNAME`；登录令牌 7 到 30 天过期，不适合放进 Dolphin 环境。
- CLI 禁止删除类命令。
- Dolphin 3.3 起停止节点：SIGINT → 约 10 秒 SIGTERM → 最后 SIGKILL。3.1 / 3.2 对进程树直接 `kill -9`。SIGKILL 捕获不到；SeaTunnel `-cj` 是 JVM 关闭钩子，遇到 SIGKILL 也不执行。
- Dolphin 工作流定义是整图快照。已上线工作流要先下线再改，定时会一起停。节点等待全部上游（AND）。
- 任务侧边栏已有内置分钟级 cron，文案建议外部调度器用 Dolphin / Airflow。

## 3. 需求

### 3.1 一期：CLI 能跑

1. **同步 / 异步提交**（对齐 `seatunnel.sh`）
   - 默认异步：提交成功返回作业 JSON，退出 0。
   - `--wait`：批任务等到终态（success / failed / canceled），实时任务等到 `running`。
   - `--timeout`：到点先停作业，退出 8。
   - `--wait` 必须轮询会刷新引擎状态的作业接口，不能用当前的公共执行等待。
2. **客户端退出时停作业**
   - `--close-job`，同步时默认开启；实时任务不适用。
   - 捕获 SIGINT / SIGTERM：发停止请求后退出。
   - SIGKILL：提交前拉起脱离进程树的守护进程，用管道感知 CLI 死亡；EOF 且没有 done 则按 `external_ref` 停作业并重试，最多 30 分钟。
   - 作业已是终态时不再发停止。
3. **外部引用**
   - `--external-ref` 记在作业上。
   - 同一 ref：运行中则接管并返回接管序号；停止中则等终态再提交；已终态则提交新作业。
   - 按 ref 停止必须带接管序号；旧序号的停止请求忽略。
4. **变量**
   - `--schedule-time` 作为内置时间变量基准。节点脚本默认传 Dolphin 的 `${system.datetime}`。
   - `--var k=v` 只覆盖本次 `custom_variables`，不写回任务。值仍是 `${…}` 时拒绝，退出 2。
   - 已绑定时 `{{system.workflow.*}}` / `{{system.project.*}}` / `{{system.task.*}}` 取绑定记录和 ref 里的实例 ID。
5. **认证**
   - 支持 `STX_SERVER`、`STX_TOKEN`。
   - 调度令牌：不过期、可吊销，只能运行、查询、停止已绑定 Dolphin 的任务。
6. **在 STX 停止作业**
   - 作业页停止和 `stx sync job cancel` 直接停 SeaTunnel。
   - `--wait` 看到 `canceled` 后写 done、退出 10，不重复发停止。不调 Dolphin 停止接口。
7. **复制节点脚本**
   - 任务页可复制生成好的 SHELL 脚本和 Worker 环境变量说明。用户可在 Dolphin 里手工建节点。

### 3.2 二期：能推

1. **推送只有两种方式**
   - 新建工作流：可勾选沿用 STX 内置定时，转成 Dolphin 定时并上线。
   - 复用已有工作流：可选上游、可选下游。勾下游会改变该节点触发条件（AND）；成环禁止推送。
2. **再推送只改节点**
   - 覆盖该 SHELL 节点的脚本和节点设置。连线、其他节点、工作流参数按读到的原样写回。
   - 不做漂移检测。节点已被删则停止，提示改用首次推送。
   - STX 任务发布新版本不用更新节点。
3. **绑定**
   - 只存项目 code、工作流 code、节点 code。上下游不记。
   - 推送后关闭 STX 内置定时。解除绑定后才能重新打开。
   - 解除绑定只删 STX 记录，不动 Dolphin 节点；之后该节点再跑返回 403。只在页面操作，CLI 不提供删除。
4. **CLI 推送**
   - `stx sync task dolphin push <id>`：`--new-workflow` 或 `--workflow --upstream --downstream`；已绑定则更新节点。支持 `--dry-run`。
   - 按 Dolphin 版本切换路径（3.3 起 process 改名 workflow）。
5. **任务侧边栏**
   - 展示绑定、最近由 Dolphin 触发的作业、打开 Dolphin、更新节点、解除绑定。内置定时开关在绑定期间禁用。

## 4. 验收标准

### 一期

- [ ] `stx sync task submit <id> --wait --confirm` 批任务等到终态；成功退出 0，失败或被停退出 10，超时退出 8。
- [ ] 现有错误的 `--wait`（300 秒打到 30 秒上限后当成功）不再出现；preview / recover 的同样问题一并修。
- [ ] `--wait` 轮询作业接口时能刷新引擎状态；在 STX 页面停止作业后，CLI 在下一轮查询看到 canceled 并退出 10。
- [ ] SIGINT / SIGTERM 下 CLI 会停作业（同步且 `--close-job` 开启时）。
- [ ] 守护进程在 CLI 被 SIGKILL 且未写 done 时按 `external_ref` 停作业；作业已终态则不再停。
- [ ] 同一 `external_ref` 有运行中作业时再次提交会接管，不重复提交；停止请求带旧接管序号会被忽略。
- [ ] `--var k=${unreplaced}` 拒绝提交，退出 2。
- [ ] `--schedule-time` 改变 `{{system.biz.date}}` 等的基准。
- [ ] `STX_SERVER` / `STX_TOKEN` 可用于非登录文件认证。
- [ ] 调度令牌不能跑未绑定任务；吊销后提交返回 401/403。
- [ ] 任务页可以复制节点脚本；脚本含 `--wait`、`--external-ref`、`--schedule-time`，没有要覆盖的自定义变量时不含 `--var`。
- [ ] 新增接口都有对应 CLI；CLI 无删除类命令。

### 二期

- [ ] 新建工作流：Dolphin 上出现只含该 SHELL 节点的工作流；勾选沿用定时则创建并上线对应 crontab，STX 内置定时关闭。
- [ ] 复用已有工作流：节点挂到勾选的上游和下游；成环时推送失败且不改图。
- [ ] 已上线工作流推送：先下线再写入再上线，定时恢复。
- [ ] 版本冲突：推送前版本号变化则停止，不覆盖别人的修改。
- [ ] 再次推送只改该节点脚本/设置，不改连线。
- [ ] 侧边栏展示绑定并可更新节点、解除绑定、打开 Dolphin。
- [ ] `stx sync task dolphin push` 覆盖新建、复用、更新节点和 `--dry-run`。
- [ ] 前端与 CLI 走同一套推送 API。

## 5. 不在范围

- 实现 Dolphin 自定义任务插件（对标 Dinky 的 `dolphinscheduler-task-dinky`）。
- 插入到连线、漂移检测。
- 调用 Dolphin 停止工作流实例接口。
- Airflow 或其他调度器。
- 实时任务在 CLI 退出后由 Dolphin 再停（实时任务 `--wait` 只等到 running）。
- 守护进程 30 分钟连不上 STX 时的自动恢复（作业继续跑，需在 STX 手工停）。

## 6. 约束

- 写命令需要 `--confirm`、幂等键、确认编号（现有 CLI 写契约）。
- stdout 单个 JSON，stderr NDJSON 事件。退出码沿用现有矩阵，作业失败/停止用 10，超时用 8，参数问题用 2，认证用 3，无权限用 4。
- 前后端新增/修改注释中英双语，中文在前。
- 前端遵循 `.trellis/spec/frontend/ui-conventions.md`（渐进披露、弹窗、密钥脱敏、单语言 i18n 文件双写）。
- 改 `.proto` 需重新生成 pb 文件（本任务预期不改 proto）。
