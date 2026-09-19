# 集群模块 CLI 设计

## 命令范围

第一批完整覆盖以下命令：

- 集群：`list`、`get`、`create`、`update`、`delete`、`status get`、`start`、`stop`、`restart`。
- 节点：`node list`、`node add`、`node add-batch`、`node update`、`node remove`、`node precheck`、`node start`、`node stop`、`node restart`、`node logs`。
- Java Proxy：`java-proxy status`、`java-proxy logs`、`java-proxy start`、`java-proxy stop`、`java-proxy restart`。

运行时存储接口包含不同资源类型和较复杂的正文，本批先不混入，后续作为独立资源命令处理。

## 实现方式

- 无正文 GET 和无正文 R0 POST 继续使用 `internal/cli/command` 生成器。
- 集群、节点的 JSON 写请求放入 `internal/cmd/cluster.go`，复用 `prepareSecureWrite`、`handleSecureWriteError` 和 `renderWriteResult`。
- 复杂 `config`、`overrides`、批量节点正文支持 JSON 文件参数，避免为嵌套结构增加大量不稳定参数。
- 简单常用字段保留独立 flag，更新命令只发送用户显式设置的字段。
- 所有可能修改资源或进程状态的命令要求 `--confirm`，使用统一幂等请求头。
- 现有服务端若尚未接入公共执行安全约定，本批在真实验证中明确记录，不把“服务端忽略确认或幂等”当作已完成。

## 真实验证

- 先查询本地服务、主机、安装包和集群状态。
- 创建带时间后缀的临时集群，完成更新、节点新增、节点更新、预检查和节点删除。
- 只在不会碰到现有运行集群的临时资源上测试进程操作；如果缺少已安装目录或端口条件，保留真实错误和退出码。
- 删除临时集群，确认列表中不存在残留。
