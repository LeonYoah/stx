
## 批次2 进度（跨上下文持久记录）
- 模块1 DONE 4d78cee7（运维Tab去guard+PUT 400）
- 模块2 DONE a2549bac（事件上报stx_callback_url+loopback+ConfigMap）
- 模块3 DONE a57382c8+style fixup（Proxy内存联动+enable_http切版本）
- 模块4 DONE 61ce5854（Runtime Storage K8s边界+ConfigMap/S3打通；卷克隆前缀/tmp/seatunnel修复）
- 模块5 DONE 后端7460ff37+前端本次提交（packages第3Tab+K8sImagePicker+升级弹窗picker+target_version同步）
- 模块6 进行中：后端一 DONE 6ccb9677（SessionConfig扩展+动态渲染）；后端二本次提交（precheck端点+CLI+operation）；剩余：hybrid deploy支持+前端向导改造（最大块）
- 模块7 未开始：全量测试+minikube E2E场景A-G
- 环境：/tmp/stx-api 二进制在:17800（后端改动后需重build+重启）；admin/admin123 cookie /tmp/ck.txt；前端pnpm dev:3000；集群seatunnel-adopted-demo id=10 host minikube-e2e id=1
- 测试命令：TZ=UTC go test -race ./internal/...；前端pnpm exec tsc --noEmit && pnpm test；route_baseline用 go run ./internal/operation/cmd/contract-report -write -root .

## 模块6 实施要点（供续接）
规格：废弃 K8sDeployClusterPanel 单页，统一用 ClusterDeployWizard 承载 K8s 6步 basic→hosts→config→precheck→deploy→complete（跳过plugins）
- Step1: infra切换(k8s/baremetal)+K8sImagePicker(components/common/k8s/K8sImagePicker.tsx 已就绪,onChange回{image,seatunnelVersion,pullPolicy,pullSecrets})
- Step2: 新建 wizard/K8sResourcesStep.tsx K8s主机+namespace+SEPARATED/HYBRID资源
- Step3: 隐藏包源/JDK；端口+JVM堆↔容器内存联动(Xmx超容器80%红字拦截)；Proxy配置卡；RuntimeAdvancedConfigCard复用；事件上报默认关；Checkpoint/IMAP共享UI+LOCAL_FILE警告
- Step4: 后端新端点 POST /api/v1/clusters/k8s/precheck（API连通/RBAC SelfSubjectAccessReview/资源冲突/pvc+secrets+sa/配比）+CLI
- Step5: 分步部署进度真实驱动
- 后端 SessionConfig 扩展 DeploymentMode/HybridReplicas/Resources.Hybrid/JVM/Runtime/Checkpoint/IMAP（可空兼容），session_render.go 动态渲染，session_deploy.go hybrid，全流程兼容HYBRID
- 现有参考文件：frontend/components/common/cluster/K8sDeployClusterPanel.tsx（将被废弃）、ClusterDeployWizard.tsx、wizard/ClusterConfigStep.tsx、internal/pkg/k8s/session_deploy.go、session_render.go、types.go
