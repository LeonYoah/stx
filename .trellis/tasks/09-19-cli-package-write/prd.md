# 安装包管理 CLI 写操作覆盖

## 目标

为安装包管理接口补齐 STX CLI 写操作，让用户和 AI Agent 能通过统一命令完成版本刷新、安装包上传、分片上传、下载、取消下载和删除，并在测试环境完成真实 HTTP 与异步流程验证。

## 范围

本批包含以下 7 个接口：

- `GET /api/v1/hosts/:id/install-command`
- `POST /api/v1/packages/versions/refresh`
- `POST /api/v1/packages/upload`
- `POST /api/v1/packages/upload/chunk`
- `DELETE /api/v1/packages/:version`
- `POST /api/v1/packages/download`
- `POST /api/v1/packages/download/:version/cancel`

CLI 命令必须继续使用操作登记表生成；需要文件流或 multipart 的上传命令可使用专用命令实现，但不能绕过统一认证、请求 ID、输出和错误处理。

## 需求

### 命令与交互

- 版本刷新和主机安装命令查询应支持普通生成式 CLI 的 JSON、YAML、table、raw 和 `--pick` 输出。
- 上传、分片上传、开始下载、取消下载、删除必须提供清晰的命令帮助、参数说明和影响提示。
- 写操作默认拒绝直接执行，用户必须显式使用 `--confirm`；命令需要保留幂等键入口，允许重复执行同一请求而不产生重复副作用。
- multipart 上传必须流式读取文件，不把完整安装包读入内存。
- 文件参数、版本、镜像、分片序号、分片总数、总大小和上传 ID 等参数要在 CLI 侧做基本校验，并将服务端错误转换为稳定退出码和 stderr 事件。
- CLI 请求必须使用专用客户端标识 Header，与前端请求区分，并继续发送认证、请求 ID和安全执行相关 Header。

### 服务端安全与异步语义

- 写操作不能只依赖 CLI 校验；服务端也必须校验确认信息、幂等键和请求归属。
- 同一个幂等键只能对应同一个请求摘要；相同请求重试应返回原结果或当前任务，不同请求复用同一幂等键必须拒绝。
- 下载取消只有在实际下载停止并清理临时文件后才能报告为 `cancelled`；不能只修改数据库或内存状态后立即宣称取消成功。
- 无法安全中断的阶段必须返回不可取消或已过晚等明确结果，不能伪造成功取消。
- 删除安装包需要明确提示不可恢复影响，并在服务端拒绝缺少确认的调用。

### 真实验证

- 使用最新构建的 `dist/stx` 和 `/Users/mac/.local/bin/stx` 软连接验证，不只测试命令帮助。
- 重启本地服务使新的 capability 生效；使用隔离 CLI 配置登录，避免覆盖日常配置。
- 真实调用本批全部命令，分别记录：请求是否成功、是否返回真实数据、是否为空结果、是否为预期错误、退出码及 stderr。
- 使用测试版本和可清理的临时文件验证上传、分片上传、查询和删除，不得删除现有 `2.3.13` 安装包。
- 使用不会造成完整大包长期落盘的方式验证下载与取消，并确认取消后后台不会继续写入临时文件。
- 验证未确认调用、幂等键重试、幂等键冲突、JSON/table/raw/`--pick`、稳定错误退出码和 capability。

## 约束

- 本批不加入插件变更、安装、升级和火焰图功能。
- 不修改当前工作区已有的安装发布文件：`.github/docker/Dockerfile.frontend`、`.github/workflows/release-on-tag.yml`、`.trellis/spec/infra/packaging-and-install.md`、`README.md`、`README_CN.md`、`docs/打包发布说明.md`、`scripts/download-bundle.sh`、`scripts/install-online.sh` 和 `deploy/`。
- 禁止执行 `git restore`；需要回撤时先说明并取得用户确认。
- 新增或修改的代码注释使用中英双语，中文在前。
- Git 提交信息使用中文。

## 验收标准

- [ ] 7 个接口都有可发现、可查看帮助并能真实发起请求的 CLI 命令。
- [ ] multipart 上传、分片上传、下载和取消使用统一客户端认证、Header、错误和输出协议。
- [ ] 写操作的服务端确认和幂等校验可通过直接 API 调用验证，不能只在 CLI 层生效。
- [ ] 下载取消不会出现“接口已取消但后台仍完成下载”的状态倒退。
- [ ] 单元测试覆盖命令参数、请求编码、multipart 流、确认 Header、幂等键和错误分类。
- [ ] 定向测试、`go test ./...`、`go vet ./...`、构建和 `git diff --check` 通过。
- [ ] 本地真实验证结果明确区分成功、有数据、空数据、预期错误和未覆盖场景。
