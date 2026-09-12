# STX

Apache SeaTunnel 数据集成平台的运维管理工具

## 项目简介

STX 是为 Apache SeaTunnel 数据集成引擎打造的一站式运维管理工具，提供**主机管理、集群与节点管理、Agent 运维、安装包与插件管理**等功能。

> 本项目基于 [linux-do/cdk](https://github.com/linux-do/cdk) 项目改造，原项目采用 MIT 协议开源。

### 主要特性

- **多种认证方式** - 支持用户名密码登录和 OAuth（GitHub、Google）登录
- **主机与 Agent 管理** - 主机注册、Agent 安装与心跳、在线状态与资源监控
- **集群与节点管理** - SeaTunnel 集群创建、节点部署、启停与状态展示
- **安装包与插件** - 安装包管理、插件安装/卸载、多版本 SeaTunnel 支持
- **多数据库支持** - 支持 SQLite（默认）、MySQL、PostgreSQL
- **国际化支持** - 内置中英文切换
- **轻量化部署** - 默认使用内存会话，无需 Redis
- **现代化界面** - 基于 Next.js 15 和 React 19 的响应式设计

### 界面展示

#### 登录

![登录页](docs/screenshots/00-login.png)

#### 控制台

![控制台](docs/screenshots/01-dashboard.png)

#### 主机管理

![主机管理](docs/screenshots/02-hosts.png)

#### 集群管理

![集群管理](docs/screenshots/03-clusters.png)

#### 安装包管理

![安装包](docs/screenshots/04-packages.png)

#### 插件管理

![插件](docs/screenshots/05-plugins.png)

#### 审计日志

![审计日志](docs/screenshots/06-audit-logs.png)

#### 集群节点管理

![集群节点](docs/screenshots/07-cluster-nodes.png)

## 架构概览

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│   Frontend      │    │    Backend      │    │   Database      │
│   (Next.js)     │◄──►│     (Go)        │◄──►│ (SQLite/MySQL)  │
│                 │    │                 │    │                 │
│ • React 19      │    │ • Gin Framework │    │ • SQLite 默认   │
│ • TypeScript    │    │ • GORM          │    │ • MySQL 可选    │
│ • Tailwind CSS  │    │ • OpenTelemetry │    │ • PostgreSQL    │
│ • Shadcn UI     │    │ • Swagger API   │    │ • SQLite 默认   │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

## 技术栈

### 后端
- **Go 1.24** - 主要开发语言
- **Gin** - Web 框架
- **GORM** - ORM 框架
- **SQLite/MySQL/PostgreSQL** - 数据库
- **内存会话** - 默认内置会话存储，无需额外缓存中间件

### 前端
- **Next.js 15** - React 框架
- **React 19** - UI 库
- **TypeScript** - 类型安全
- **Tailwind CSS 4** - 样式框架
- **Shadcn UI** - 组件库

## 环境要求

- **Go** >= 1.24
- **Node.js** >= 18.0
- **pnpm** >= 8.0 (推荐)

## 快速开始

### 1. 克隆项目

```bash
git clone https://github.com/LeonYoah/stx.git
cd stx
```

### 2. 配置环境

```bash
cp config.example.yaml config.yaml
```

默认配置使用 SQLite 数据库，无需额外配置即可启动。

### 3. 启动后端

```bash
go mod tidy
go run main.go api
```

### 4. 启动前端

```bash
cd frontend
pnpm install
pnpm dev
```

### 5. 访问应用

- **前端界面**: http://localhost:3000
- **默认账号**: admin / admin123

## 离线部署 STX

### 1. 打包 CentOS 7 兼容离线包

```bash
scripts/package-release.sh \
  --arch amd64 \
  --bundle-observability without \
  --node-major 18 \
  --node-variant glibc217
```

### 2. 解压并安装

```bash
tar -xzf stx-<version>-linux-amd64-node18-glibc217-without-observability.tar.gz
cd stx-<version>-linux-amd64-node18-glibc217-without-observability
sudo ./install.sh
```

安装包只会携带 `config.example.yaml`，安装时会自动生成 `config.yaml`。

### 3. 端口配置

- **后端 HTTP / gRPC 端口**：修改 `config.yaml`
  - `app.addr`
  - `grpc.port`
- **前端端口 / 监听地址**：启动时通过环境变量覆盖
  - `FRONTEND_PORT`
  - `FRONTEND_HOST`
  - `NEXT_PUBLIC_BACKEND_BASE_URL`

示例：

```bash
CONFIG_PATH=/opt/stx/config.yaml \
FRONTEND_PORT=8080 \
FRONTEND_HOST=0.0.0.0 \
NEXT_PUBLIC_BACKEND_BASE_URL=http://127.0.0.1:8000 \
/opt/stx/bin/start.sh
```

## ⚙️ 配置说明

### 主要配置项

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `auth.default_admin_username` | 默认管理员用户名 | `admin` |
| `auth.default_admin_password` | 默认管理员密码 | `admin123` |
| `database.type` | 数据库类型 | `sqlite` |

### OAuth 登录配置（可选）

平台支持 GitHub 和 Google OAuth 登录作为备选登录方式。

#### 获取 GitHub OAuth 凭证

1. 登录 GitHub，访问 [Developer Settings](https://github.com/settings/developers)
2. 点击 **"New OAuth App"**
3. 填写应用信息：
   - **Application name**: `STX`
   - **Homepage URL**: `http://localhost:3000`
   - **Authorization callback URL**: `http://localhost:3000/callback`
4. 创建后获取 **Client ID** 和 **Client Secret**

> 详细教程：[GitHub OAuth2 配置指南](https://apifox.com/apiskills/how-to-use-github-oauth2/)

#### 获取 Google OAuth 凭证

1. 访问 [Google Cloud Console](https://console.cloud.google.com/)
2. APIs & Services → Credentials → Create Credentials → OAuth client ID
3. 添加 Authorized redirect URIs: `http://localhost:3000/callback`

> 详细教程：[Google OAuth2 配置指南](https://apifox.com/apiskills/how-to-use-google-oauth2/)

#### 配置 OAuth 凭证

```yaml
oauth_providers:
  github:
    enabled: true
    client_id: "你的 GitHub Client ID"
    client_secret: "你的 GitHub Client Secret"
    redirect_uri: "http://localhost:3000/callback"
  google:
    enabled: true
    client_id: "你的 Google Client ID"
    client_secret: "你的 Google Client Secret"
    redirect_uri: "http://localhost:3000/callback"
```

## 测试

```bash
# 后端测试
go test ./...

# 前端测试
cd frontend && pnpm test
```

## 二次开发指南

### Protocol Buffers 代码生成

本项目使用 gRPC 进行 Agent 与 Control Plane 之间的通信。如果修改了 `.proto` 文件，需要重新生成 Go 代码。

#### 前置条件

1. **安装 Go protoc 插件**（Linux/macOS/Windows 通用）

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

#### Linux / macOS

1. **安装 protoc 编译器**

```bash
# macOS
brew install protobuf

# Ubuntu/Debian
sudo apt-get install protobuf-compiler

# CentOS/RHEL
sudo yum install protobuf-compiler
```

2. **生成代码**

```bash

# 或手动执行
protoc --proto_path=. \
    --go_out=. --go_opt=paths=source_relative \
    --go-grpc_out=. --go-grpc_opt=paths=source_relative \
    internal/proto/agent/agent.proto
```

#### Windows (PowerShell)

1. **下载并安装 protoc 编译器**

```powershell
# 一键下载并配置 protoc（安装到 D:\protoc 目录）
$protocVersion = "28.3"
$protocZip = "protoc-$protocVersion-win64.zip"
$protocUrl = "https://github.com/protocolbuffers/protobuf/releases/download/v$protocVersion/$protocZip"
$protocDir = "D:\protoc"

if (!(Test-Path $protocDir)) { 
    New-Item -ItemType Directory -Path $protocDir -Force 
}
Invoke-WebRequest -Uri $protocUrl -OutFile "$protocDir\$protocZip"
Expand-Archive -Path "$protocDir\$protocZip" -DestinationPath $protocDir -Force
$env:PATH = "$protocDir\bin;$env:PATH"

# 验证安装
protoc --version
```

2. **生成代码**

```powershell
# 设置环境变量（每次新开 PowerShell 需要执行）
$protocDir = "D:\protoc"
$env:PATH = "$protocDir\bin;$env:USERPROFILE\go\bin;$env:PATH"


go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
# 生成 protobuf 代码
protoc --proto_path=. `
    --go_out=. --go_opt=paths=source_relative `
    --go-grpc_out=. --go-grpc_opt=paths=source_relative `
    internal/proto/agent/agent.proto
```

> 提示：为了避免每次都设置环境变量，建议将 `D:\protoc\bin` 添加到系统 PATH 环境变量：
> - 右键 "此电脑" → "属性" → "高级系统设置" → "环境变量"
> - 在 "系统变量" 中找到 `Path`，点击 "编辑"
> - 添加新条目：`D:\protoc\bin`
> - 点击 "确定" 保存，重启 PowerShell 即可全局使用

#### 验证生成结果

生成成功后，以下文件会被更新：
- `internal/proto/agent/agent.pb.go` - Protobuf 消息定义
- `internal/proto/agent/agent_grpc.pb.go` - gRPC 服务定义

```bash
# 运行测试验证生成的代码
go test ./internal/proto/agent/...
```

### Agent 打包

Agent 是部署在目标主机上的守护进程，需要交叉编译为 Linux 二进制文件。

#### Linux / macOS

```bash
cd agent

# 打包 Linux amd64
GOOS=linux GOARCH=amd64 go build -o stx-agent ./cmd

# 打包 Linux arm64
GOOS=linux GOARCH=arm64 go build -o stx-agent-arm64 ./cmd
```

#### Windows (PowerShell)

```powershell
cd agent

# 打包 Linux amd64
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o stx-agent ./cmd

# 打包 Linux arm64
$env:GOOS="linux"; $env:GOARCH="arm64"; go build -o stx-agent-arm64 ./cmd

# 恢复环境变量（可选）
Remove-Item Env:GOOS; Remove-Item Env:GOARCH
```

#### 部署 Agent 二进制

打包完成后，将 `stx-agent` 复制到 `lib/agent/` 目录：

```bash
# Linux/macOS
cp agent/stx-agent lib/agent/stx-agent-linux-amd64
cp agent/stx-agent-arm64 lib/agent/stx-agent-linux-arm64

# Windows PowerShell
Copy-Item agent/stx-agent lib/agent/stx-agent-linux-amd64
Copy-Item agent/stx-agent-arm64 lib/agent/stx-agent-linux-arm64

# Windows PowerShell 一键操作

cd agent; $env:GOOS="linux"; $env:GOARCH="amd64"; go build -o stx-agent ./cmd; cd ..; Copy-Item agent/stx-agent lib/agent/stx-agent-linux-amd64 -Force

```

### gRPC Proto 代码生成

本项目使用 gRPC 进行 Control Plane 与 Agent 之间的通信。修改 `.proto` 文件后需要重新生成 Go 代码。

#### 一键生成（推荐）

```powershell
# Windows PowerShell
protoc --proto_path=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/proto/agent/agent.proto; `
Copy-Item -Path "internal\proto\agent\agent.pb.go" -Destination "agent\agent.pb.go" -Force
```

```bash
# Linux/macOS
protoc --proto_path=. --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative internal/proto/agent/agent.proto && \
cp internal/proto/agent/agent.pb.go agent/agent.pb.go
```

#### 命令说明

| 参数 | 说明 |
|------|------|
| `--proto_path=.` | 指定 proto 文件搜索路径为当前目录 |
| `--go_out=.` | 生成 Go 代码到当前目录 |
| `--go_opt=paths=source_relative` | 生成到 proto 文件同级目录（关键！） |
| `--go-grpc_out=.` | 生成 gRPC 代码 |
| `Copy-Item` | 同步到 agent 模块（agent 独立编译需要） |

#### 验证

```bash
go build ./...           # 编译主项目
cd agent && go build ./... # 编译 Agent
```

## 部署

### Docker 部署

```bash
docker build -f .github/docker/Dockerfile.backend -t stx-backend .
docker run -d -p 8000:8000 -p 9000:9000 stx-backend
```

## 📄 许可证

本项目基于 [Apache License 2.0](LICENSE) 开源。

## 🔗 相关链接

- [Apache SeaTunnel](https://seatunnel.apache.org/)
- [STX GitHub](https://github.com/LeonYoah/stx)
- [原项目 linux-do/cdk](https://github.com/linux-do/cdk)
