<!--
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

## Context

我们已经把 `tools/stx-java-proxy` 引入到 STX 仓库，并新增了 `probe-once` CLI 模式。CLI 支持通过 request/response JSON 文件一次性执行 checkpoint 或 IMAP 探测，无需维护常驻 HTTP 服务。

当前安装相关链路有两个明显约束：

- 真实 runtime probe 依赖已解压的 `SEATUNNEL_HOME`，因为 classpath 需要复用 `${SEATUNNEL_HOME}/starter`、`${SEATUNNEL_HOME}/lib`、`${SEATUNNEL_HOME}/connectors` 和 `${SEATUNNEL_HOME}/plugins`；
- `io.github.leonyoah.stx:stx-java-proxy:2.3.13` 当前并不在 Maven Central，不能假设远端 Agent 可随时从中央仓库拉取该 jar。

因此，这一阶段不去强行改造安装前页面上的 runtime validate，而是优先把真实探测放到安装链路内部：安装包已解压、运行时依赖已就位，此时再执行 one-shot probe，成功率和语义都更准确。

## Goals / Non-Goals

**Goals:**

- 在 Agent 安装链路中对远端 checkpoint / IMAP 执行真实运行时探测。
- 探测失败只生成 warning，不阻塞安装配置写入与后续启动。
- 将 warning 透传到控制面和前端安装进度中。
- 复用已有 `stx-java-proxy` one-shot CLI，不新增常驻服务。

**Non-Goals:**

- 本阶段不把安装前的 `/installer/runtime-storage/validate` 改造成真实 runtime probe。
- 本阶段不把 probe 结果设计成安装硬门槛。

## Decisions

### Decision 1: 真实 runtime probe 只放进安装链路，不抢先替换安装前校验接口

- 选择：`extract` 完成后，在 Agent 本地执行 one-shot probe。
- 原因：此时 `SEATUNNEL_HOME` 已存在，探测依赖的 SeaTunnel 类路径最完整；同时无需先解决“安装前如何把 proxy 分发到所有主机”这个更大的问题。
- 备选方案：立即替换 `/installer/runtime-storage/validate` 为真实 probe。
- 不选原因：该接口当前既没有安装目录上下文，也没有稳定的 proxy 资产分发方案，强改会制造新的误报和环境依赖。

### Decision 2: Probe failure is warning-only

- 选择：checkpoint / IMAP runtime probe 失败后，记录 step warning 并继续执行 `configure_checkpoint` / `configure_imap`。
- 原因：运行时存储问题是重要提示，但不应阻断安装主链路；用户仍然可以完成安装、查看 warning、再决定是否修正存储配置。
- 备选方案：探测失败直接让步骤失败。
- 不选原因：会使安装在外部存储波动、权限暂时异常、proxy 工具缺失等情况下完全不可用，体验过于激进。

### Decision 3: 使用非 `-bin` jar，通过脚本拼装 SeaTunnel runtime classpath

- 选择：优先使用普通 `stx-java-proxy-2.3.13-2.12.15.jar`，配合 `stx-java-proxy.sh`。
- 原因：普通 jar 与脚本的职责边界更清晰，真正需要的依赖仍然来自 `SEATUNNEL_HOME`；`-bin.jar` 会引入更多重复依赖，但对存储插件/Hadoop 组合问题并不能彻底兜底。
- 备选方案：统一改用 `-bin.jar`。
- 不选原因：更重，也更容易与真实安装目录下的运行时依赖重复或冲突。

### Decision 4: 通过控制面发布包与 Agent 安装脚本统一分发 proxy 资产，并预留多版本 jar 选择能力

- 选择：控制面发布包统一内置 `lib/stx-java-proxy-{seatunnelVersion}.jar` 与 `scripts/stx-java-proxy.sh`；Agent 安装脚本默认下载 `2.3.13` 版本 jar，并在运行时优先按 SeaTunnel 集群版本选择对应 jar，找不到时回退到 `2.3.13`。
- 原因：runtime probe 最终是在目标主机上的 Agent 进程内执行，只改控制面本地工作区路径无法真正闭环；同时平台会长期支持多个 SeaTunnel 版本，需要让 proxy jar 的选择规则可扩展，而不是把单一 jar 名写死。
- 备选方案：仅靠 Agent 本地“搜索工作区 / tools 目录”发现资产。
- 不选原因：这只适用于开发环境，无法覆盖正式 Agent 安装。

### Decision 5: Agent 侧仍保留“资产发现 + graceful fallback”作为兼容兜底

- 选择：Agent 在本地搜索 proxy 脚本和普通 jar；若二者齐备则执行探测，否则记录 warning 并跳过。
- 原因：即使正式分发链路已经建立，开发环境、源码运行和历史安装仍可能依赖旧路径；保留兼容搜索可以降低切换成本。
- 备选方案：要求缺失资产直接失败。
- 不选原因：不符合 warning-only 目标，也不利于渐进落地。

### Decision 6: 控制面安装状态新增 `warnings` 聚合字段

- 选择：控制面在轮询安装进度时从步骤消息聚合 warning，并持久保存在 `InstallationStatus.warnings` 中；前端用独立黄色提示区域展示。
- 原因：如果只把 warning 混在 step message 里，最终完成态容易被淹没，用户感知不够强。
- 备选方案：只在步骤 message 中写 `Warning: ...`。
- 不选原因：信息不够聚合，完成态下不够显眼。

## Risks / Trade-offs

- [风险] 控制面发布包或 Agent 安装脚本漏带 proxy 资产。  
  → 缓解：发布脚本、控制面分发接口、Agent 安装脚本同时纳入改造；若运行时仍缺失，安装步骤继续 warning-only。

- [风险] 真实 probe 本身依赖外部存储/Hadoop 版本组合，报错信息较长。  
  → 缓解：控制面保留原始 warning message，前端做折叠显示；日志中打印完整错误。

- [风险] 控制面 warning 聚合若只靠字符串前缀识别，容易被误判。  
  → 缓解：Agent 统一使用固定前缀 `Warning:` 报告非阻塞问题，控制面仅识别该前缀。

- [风险] 当前页面上的 runtime-storage validate 仍是 endpoint-only，用户可能以为两者等价。  
  → 缓解：第一阶段先在文案中说明安装时还会执行真实 runtime probe；第二阶段再统一改造该页面。

## Migration Plan

1. 新增 OpenSpec 变更文档，明确第一阶段范围。
2. 在 Agent 中新增 runtime storage probe helper，并接入 checkpoint / IMAP 安装步骤。
3. 控制面安装状态新增 `warnings` 字段，并在进度轮询时聚合 warning。
4. 前端安装进度展示 warnings。
5. 在控制面发布包、控制面 Agent 分发接口和 Agent 安装脚本中补齐 proxy 资产分发。
6. 后续单独变更安装前 runtime validate 页面升级。

## Open Questions

- 第二阶段是否要在安装前页面增加“需要已准备 runtime probe 资产”的环境提示，再切换到真实 probe。
