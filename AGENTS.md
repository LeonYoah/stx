## 强制遵循
1. 使用中文进行git提交
2. **改动前查阅 Spec 规范**：进行任何前端、后端和 UI 改动前，必须先根据 `.trellis/spec/` 对应目录的 `index.md` 索引查阅相关 spec 文档，遵循既定架构契约与设计规范自检无误后再提交。

## 规范查阅与开发流程
- **前端与 UI 改动**：开发前必读 `.trellis/spec/frontend/index.md` 及其索引文档（特别是 `ui-conventions.md` 中的高信噪比渐进式披露、原子信息防被迫换行、零塌陷加载等通用美学准则）。
- **后端改动**：开发前必读 `.trellis/spec/backend/index.md` 及其索引文档（如目录结构、数据库规范、日志规范、错误处理与安全执行准则）。
- **跨层流转与契约**：必读 `.trellis/spec/guides/cross-layer-thinking-guide.md`，明确数据流边界、格式转换与空值兜底契约。
- **验证与提交**：开发完成后严格对照 spec 中的一致性检查项（Checklist）自检，验证通过后再进行 commit。

## 最小变更与范围纪律
- **先定范围再动手**：列出改动必须触达的所有位置（调用方、测试、夹具、配置、导出、CLI），并评估对用户的影响（会破坏或暴露什么数据、哪些调用方会失效）。这就是范围，范围之外不做。
- **规范内最小**：取能完整跑通且符合 spec 约定的最小方案。i18n、CLI 对齐、proto 再生成、License 头、E2E 覆盖等仓库约定属于范围的一部分，不是可砍的额外功能；模糊需求取核心功能的最小合规版本。
- **复用优先**：先找代码库已有的 helper / 组件 / 模式，其次标准库与平台能力，再次已安装依赖；不为几行代码新增依赖。
- **不投机**：不做没人要求的特性、选项、配置、抽象和"以后用得上"的代码；删除优于新增；保持代码库既有的分层、接口与约定。
- **修根因**：bug 修复先 grep 全部调用方，在共享代码处修一次；移动或合并的代码保留原有错误处理与校验。
- **不可砍**：信任边界的校验、防数据丢失的错误处理、安全、可访问性，以及用户明确要求的内容。
- **非平凡逻辑留验证**：新增分支、循环、解析器、涉及钱或安全的逻辑，补一个小测试或自检；琐碎改动不需要。
- 有已知限制的捷径用 `shortcut:` 前缀注释标明限制与升级时机（内容按下方注释约定双语）。

## 注释约定
- 注释只写代码表达不了的 why：约束、坑、业务规则来源。不复述代码逻辑，不记录"本次改了什么"，不写面向用户的解释。
- **自己新增或修改的注释，要求中英双语，中文在前，英文在后。** 两种语言表达同一语义；不要为了凑双语去注释显而易见的代码。
- 不主动新建或更新文档文件（README、CHANGELOG、说明页等），除非任务明确要求；`.trellis/` 按既定流程更新。

## CLI约定
- 每次新增接口要补充 cli能力
- 每次修复或者优化功能需要检查 对应cli是否受影响


## Protobuf 变更说明

当修改 `.proto` 后，需要重新生成并确认以下文件更新：

- `internal/proto/agent/agent.pb.go`
- `internal/proto/agent/agent_grpc.pb.go`

## Arc UI

工作台改用 [Arc](https://uiarc.dev/docs/ai) 时，先读 `.claude/skills/arc/SKILL.md`（`.agents/skills/arc/` 是同一份）。组件从 `frontend/` 用 shadcn CLI 安装，注册表已写在 `frontend/components.json` 的 `@uiarc`。不要手写或仿造 Pro 组件。界面文案仍走 next-intl。第一次真正装入组件时，再在根布局引入 `@/components/arc/foundation.css`。

<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex or another agent-capable tool, additional project-scoped helpers may live in:
- `.agents/skills/` — reusable Trellis skills
- `.codex/agents/` — optional custom subagents

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

## Cursor Cloud specific instructions

Cloud Agent 启动后，API 和 Web UI 由环境 start 拉起，一般不用再手工开进程。

- API：在仓库根目录执行 `go run main.go api`，监听 **17800**。`api` 会提示改用 `server`，进程仍提供接口。
- Web UI：`cd frontend && pnpm dev`，监听 **3000**。页面上的 `/api` 会代理到 `http://127.0.0.1:17800`。
- 配置：若没有 `config.yaml`，从 `config.example.yaml` 复制。默认 SQLite，文件在 `data/stx.db`。登录不需要 MySQL、PostgreSQL、Java 或 stx-java-proxy。
- 登录账号：`admin` / `admin123`。
- 工具链：Go **1.26.10**（以 `go.mod` 为准）。镜像里的 `/usr/bin/go` 更旧，环境会把 1.26.10 装到 `/usr/local/go` 并链接到 `/usr/local/bin/go`。前端使用 Node 22 与 pnpm **10.10.0**（`frontend/package.json` 的 `packageManager`）。
- 检查命令：`go test ./...`；`cd agent && go test ./...`；`cd frontend && pnpm test`；`cd frontend && pnpm exec tsc --noEmit`。`pnpm lint` 在当前 main 上会报已有的 ESLint 问题，这不是环境没装好。
