
## 批次2 进度（跨上下文持久记录）
- 模块1 DONE 4d78cee7（运维Tab去guard+PUT 400）
- 模块2 DONE a2549bac（事件上报stx_callback_url+loopback+ConfigMap）
- 模块3 DONE a57382c8+style fixup（Proxy内存联动+enable_http切版本）
- 模块4 DONE 61ce5854（Runtime Storage K8s边界+ConfigMap/S3打通；卷克隆前缀/tmp/seatunnel修复）
- 模块5 后端 DONE 7460ff37（container_images+REST/CLI/operation）；前端进行中：packages第3Tab+K8sImagePicker+升级弹窗换picker；注意 runtime_storage.go/list.go gofmt 已修待提交
- 模块6 未开始：统一6步创建向导（最大块）
- 模块7 未开始：全量测试+minikube E2E场景A-G
- 环境：/tmp/stx-api 二进制在:17800（后端改动后需重build+重启）；admin/admin123 cookie /tmp/ck.txt；前端pnpm dev:3000；集群seatunnel-adopted-demo id=10 host minikube-e2e id=1
- 测试命令：TZ=UTC go test -race ./internal/...；前端pnpm exec tsc --noEmit && pnpm test；route_baseline用 go run ./internal/operation/cmd/contract-report -write -root .
