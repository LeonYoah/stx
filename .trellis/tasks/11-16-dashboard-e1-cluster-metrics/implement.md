# E1 执行计划：集群指标细化分组

1. [ ] `internal/apps/dashboard/cluster_detail.go`：五组面板查询表 + 事件差分检测 + handler。
2. [ ] 路由 `/dashboard/home/clusters/detail`、operation `dashboard.home.cluster-detail`、swagger、基线。
3. [ ] `home.service.ts` getClustersDetail + 类型；ClusterTab 细节区（组切换 + 面板网格 + 事件条）；i18n zh/en。
4. [ ] 测试：Go（分组查询拼装 / 事件差分 / 非法 group / 降级）、前端（懒加载 / 面板渲染 / 空态）。

## 实施记录

（待填）
