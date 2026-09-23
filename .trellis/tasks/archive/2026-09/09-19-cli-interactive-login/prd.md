# CLI 登录支持交互输入用户名

## 目标

修正 `stx login` 只交互读取密码、不交互读取用户名的问题。在真实终端中，只指定服务地址时，应依次询问用户名和密码，而不是直接提示缺少 `--username`。

## 要求

- 用户名优先级保持为：`--username`、`STX_USERNAME`、终端交互输入。
- 真实 TTY 缺少用户名时，在 stderr 输出 `Username: ` 并读取一行；输入前后空白需要去除。
- 密码继续使用 TTY 隐藏输入，提示写 stderr，不得出现在 stdout、日志或进程参数中。
- 非交互环境缺少用户名时，继续返回稳定的用法错误，不得等待输入。
- 非交互环境缺少 `--password-stdin` 时，继续返回稳定的用法错误。
- `--username`、`STX_USERNAME` 和 `--password-stdin` 的原有脚本用法保持兼容。
- 使用最新 `dist/stx` 在真实伪终端中完成登录、查询当前用户和退出登录。

## 验收标准

- [x] `stx login --server http://127.0.0.1:17800` 在 TTY 中依次显示 `Username:` 和 `Password:`。
- [x] 交互输入用户名和密码后登录成功，stdout 只有最终 JSON。
- [x] 非 TTY 缺少用户名时不阻塞，返回 `usage_error`。
- [x] 显式用户名和环境变量用户名继续可用。
- [x] 密码不出现在输出和配置以外的位置。
- [x] 单元测试、全量测试、静态检查和真实二进制验证通过。

## 验证记录

- 最新二进制：`dist/stx`，SHA-256 `cf2b50f5e8b4973a892a5067f0c5460382b2cf8986e323619c71528970f654bd`。
- 真实伪终端执行 `stx login --server http://127.0.0.1:17800`，依次出现 `Username:`、`Password:`，登录成功且密码未回显。
- 登录后真实执行 `stx whoami` 成功，随后 `stx logout` 成功撤销令牌。
- 隔离配置文件权限为 `0600`。
- 非 TTY 缺少用户名返回 `usage_error` 和退出码 `2`；显式 `--username`、`--password-stdin` 登录仍成功。
- `go test ./...`、`go vet ./...` 和本任务文件差异检查通过。
- 全仓差异检查仍会报告 `docs/打包发布说明.md` 的既有行尾空格，该文件属于其他任务，本次未修改。

## Goal

在真实终端执行 stx login 时交互询问用户名和隐藏输入密码，保留脚本模式参数要求

## Requirements

- TBD

## Acceptance Criteria

- [ ] TBD

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
