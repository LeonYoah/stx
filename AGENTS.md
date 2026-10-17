## 强制遵循
1. 使用中文进行git提交
2. **改动前查阅 Spec 规范**：进行任何前端、后端和 UI 改动前，必须先根据 `.trellis/spec/` 对应目录的 `index.md` 索引查阅相关 spec 文档，遵循既定架构契约与设计规范自检无误后再提交。

## 规范查阅与开发流程
- **前端与 UI 改动**：开发前必读 `.trellis/spec/frontend/index.md` 及其索引文档（特别是 `ui-conventions.md` 中的高信噪比渐进式披露、原子信息防被迫换行、零塌陷加载等通用美学准则）。
- **后端改动**：开发前必读 `.trellis/spec/backend/index.md` 及其索引文档（如目录结构、数据库规范、日志规范、错误处理与安全执行准则）。
- **跨层流转与契约**：必读 `.trellis/spec/guides/cross-layer-thinking-guide.md`，明确数据流边界、格式转换与空值兜底契约。
- **验证与提交**：开发完成后严格对照 spec 中的一致性检查项（Checklist）自检，验证通过后再进行 commit。

## 注释约定
- 关键方法/实现/设计及时添加注释，大多数其实都得加！
- **自己新增或修改的前后端代码注释，要求中英双语。**
- **默认顺序为“中文在前，英文在后”。** 两种语言应表达同一语义，不要写成两套不一致的说明。

## CLI约定
- 每次新增接口要补充 cli能力
- 每次修复或者优化功能需要检查 对应cli是否受影响


## Protobuf 变更说明

当修改 `.proto` 后，需要重新生成并确认以下文件更新：

- `internal/proto/agent/agent.pb.go`
- `internal/proto/agent/agent_grpc.pb.go`


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

- 需要 Go 1.24。基础镜像自带的 `/usr/bin/go` 是 1.22；环境把 Go 1.24.13 装到 `/usr/local/go` 并链接到 `/usr/local/bin/go`。确认 `go version` 输出 `go1.24.13`。
- 没有 `config.yaml` 时，从 `config.example.yaml` 复制后再启动。默认数据库是 SQLite，文件在 `data/stx.db`。在仓库根目录执行 `go run main.go server`（`api` 只是隐藏兼容别名）。健康检查是 `GET http://127.0.0.1:17800/api/v1/health`。默认登录 `admin` / `admin123`。
- Web UI 开发端口是 3000，不是安装包用的 17880：`cd frontend && pnpm dev --hostname 0.0.0.0 --port 3000`。前端包管理器钉在 pnpm 10.10.0。单测命令是 `pnpm test`。
- 同环境里的文档站在 sibling 仓库 `stx-website`，用 `pnpm start --host 0.0.0.0 --port 3001`，避免占用 Web UI 的 3000。
- `go test ./...` 和 `cd agent && go test ./...` 在本机没有 MySQL / PostgreSQL 时可以通过；连不上的多库用例会跳过。
