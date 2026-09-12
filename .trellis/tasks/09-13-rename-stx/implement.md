# STX 改名执行计划

## 并行区域

### A. 后端、Agent 与 Java Proxy

- 修改根模块和 Agent 模块路径及全部 Go import。
- 修改后端与 Agent 的配置、服务名、目录、API、状态值和测试。
- 修改 protobuf 源文件与生成文件。
- 重命名 Java Proxy 目录、脚本、构件和 Java 类型，把 Maven 坐标与源码包迁到 STX 自有命名空间，并运行 Maven 测试。

### B. 前端

- 修改包元数据、锁文件、界面文案、国际化资源、服务常量和测试。
- 修改前端使用的项目链接、Cookie 测试值和 E2E 固定名称。
- 运行前端格式、类型、单元测试和生产构建检查。

### C. CI、发布、文档与运维脚本

- 修改 `.github/workflows`、Dockerfile、发布打包、安装、重启和可观测性脚本。
- 修改当前 README、开发文档、有效 OpenSpec 与 Trellis 规范。
- 清理 Trellis 中未加入项目专有内容的模板，更新索引、目录说明和当前任务引用。
- 保留历史归档和 `NOTICE` 来源说明。

### D. 主任务集成

- 处理公共文件和并行区域之间的引用。
- 重新生成 Swagger/protobuf，检查旧名称剩余项。
- 运行完整验证并修正发现的问题。

## 约束

- 不添加任何旧名称别名、回退读取或迁移分支。
- 不执行 `git commit`、`git push` 或 `git merge`。
- 不执行 `git restore`；如需回撤，必须先由用户确认。
- 内容修改使用 `apply_patch`，纯路径重命名可使用 `git mv`。
- 新增或修改的代码注释须遵循 `AGENTS.md`：中文在前、英文在后。
- 并行参与者只修改分配给自己的目录，看到其他人的改动时保留并适配。

## 检查命令

具体命令以仓库已有脚本和各模块配置为准，至少包含：

```bash
gofmt -w <changed-go-files>
go test ./...
(cd agent && go test ./...)
(cd frontend && pnpm exec tsc --noEmit && pnpm test && pnpm run pack:standalone)
mvn -f tools/stx-java-proxy/pom.xml test
bash -n scripts/*.sh support-files/release/*.sh
python3 scripts/check_license.py
```

生成检查：

```bash
bash scripts/swagger.sh
git diff --check
rg -n -i 'seatunnelx|seatunnel-x' <current-source-and-doc-paths>
```
