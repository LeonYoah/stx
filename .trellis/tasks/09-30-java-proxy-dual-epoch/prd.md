# stx-java-proxy 双代际 v2/v3 jar

## Goal
同一套 proxy 源码按 SeaTunnel 主版本分别编译打包，使 2.x / 3.x 集群各自使用编译依赖对齐的 thin jar。

## Scope
- Maven `epoch-v2` / `epoch-v3` profile：分别对 SeaTunnel 2.3.13 / 3.0.0 编译，产出 `stx-java-proxy-v2.jar` / `stx-java-proxy-v3.jar`
- `ProxyEpochForVersion`：3.x → `v3`
- 发布/安装脚本同时携带并部署两份 jar
- 下载解析支持直接使用代际标签（`v2`/`v3`）

## Acceptance Criteria
- [x] `mvn -Pepoch-v2 package` 与 `mvn -P'epoch-v3,!epoch-v2' package` 均可成功
- [x] 产物文件名分别为 `stx-java-proxy-v2.jar`、`stx-java-proxy-v3.jar`
- [x] SeaTunnel `3.0.0` / `3.x` 解析到代际 `v3`；`2.3.x` 仍为 `v2`
- [x] `package-release.sh` 构建并暂存两份 jar
- [x] 相关 Go 单测通过
- [x] 3.x 缺 v3 jar 时可回退 v2（滚动升级）
