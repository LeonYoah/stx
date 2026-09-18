# Secondary Development

Developer-facing notes for extending STX. Product overview and user install live in [README.md](../README.md) / [README_CN.md](../README_CN.md).

Chinese short version: [二次开发.md](./二次开发.md).

## Stack

- Backend: Go 1.24, Gin, GORM, OpenTelemetry, Swagger
- Frontend: Next.js 15, React 19, TypeScript, Tailwind CSS, Shadcn UI
- Agent: Go process on target hosts, talks to the control plane over gRPC
- Database: SQLite (default) / MySQL / PostgreSQL

## Local development

```bash
cp config.example.yaml config.yaml
go mod tidy
go run main.go api

cd frontend
pnpm install
pnpm dev
```

Default UI: `http://localhost:3000`  
Default login: `admin` / `admin123`

```bash
go test ./...
cd frontend && pnpm test
```

## Protobuf / gRPC codegen

After changing `.proto` files, regenerate:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

protoc --proto_path=. \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  internal/proto/agent/agent.proto

cp internal/proto/agent/agent.pb.go agent/agent.pb.go
```

Confirm updates:

- `internal/proto/agent/agent.pb.go`
- `internal/proto/agent/agent_grpc.pb.go`

### Windows (PowerShell)

```powershell
$protocVersion = "28.3"
$protocZip = "protoc-$protocVersion-win64.zip"
$protocUrl = "https://github.com/protocolbuffers/protobuf/releases/download/v$protocVersion/$protocZip"
$protocDir = "D:\protoc"

if (!(Test-Path $protocDir)) {
  New-Item -ItemType Directory -Path $protocDir -Force
}
Invoke-WebRequest -Uri $protocUrl -OutFile "$protocDir\$protocZip"
Expand-Archive -Path "$protocDir\$protocZip" -DestinationPath $protocDir -Force
$env:PATH = "$protocDir\bin;$env:USERPROFILE\go\bin;$env:PATH"

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

protoc --proto_path=. `
  --go_out=. --go_opt=paths=source_relative `
  --go-grpc_out=. --go-grpc_opt=paths=source_relative `
  internal/proto/agent/agent.proto

Copy-Item -Path "internal\proto\agent\agent.pb.go" -Destination "agent\agent.pb.go" -Force
```

## Agent cross-compile

```bash
cd agent
GOOS=linux GOARCH=amd64 go build -o stx-agent ./cmd
GOOS=linux GOARCH=arm64 go build -o stx-agent-arm64 ./cmd
cp stx-agent ../lib/agent/stx-agent-linux-amd64
cp stx-agent-arm64 ../lib/agent/stx-agent-linux-arm64
```

PowerShell:

```powershell
cd agent
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o stx-agent ./cmd
$env:GOOS="linux"; $env:GOARCH="arm64"; go build -o stx-agent-arm64 ./cmd
Copy-Item stx-agent ../lib/agent/stx-agent-linux-amd64 -Force
Copy-Item stx-agent-arm64 ../lib/agent/stx-agent-linux-arm64 -Force
```

## Related docs

- [Quick start (users, CN)](./00-快速开始.md)
- [Packaging & release (CN)](./打包发布说明.md)
- [Observability one-click integration (CN)](./可观测性三件套一键接入说明.md)
