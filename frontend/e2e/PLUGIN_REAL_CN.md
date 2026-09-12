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

# 插件管理 Real 场景说明

Spec：

- `frontend/e2e/plugin-real.spec.ts`

## 入口命令

```bash
cd frontend
pnpm exec bash ./scripts/e2e/run-real-plugin.sh
```

## 共享 real-runner 逻辑

所有 real 场景都复用了同一套 real-install 执行骨架。

### `run-real-installer.sh` 会做什么

`scripts/e2e/run-real-installer.sh` 会：

1. 在 `tmp/e2e/installer-real.*` 下创建临时工作目录
2. 生成临时 backend / agent 配置
3. 按需启动临时 MinIO
4. 在需要 MinIO 的场景下创建 checkpoint / IMAP bucket
5. 确保 `stx-java-proxy` jar 可用于安装后校验
6. 启动：
   - 临时 backend
   - 临时 agent supervisor
   - frontend dev server
7. 执行指定的 Playwright spec

## 当前覆盖内容

- 安装真实集群作为测试源
- 打开插件市场
- 选择 JDBC 的 MySQL profile
- 通过 API 下载真实插件资产
- 校验本地插件元数据与文件
- 将插件安装到真实集群
- 在集群详情中校验插件详情展示
- 下载并校验 `file-obs` 的 `lib` 依赖资产

## 执行流程图

```mermaid
flowchart TD
    A[安装源集群] --> B[打开插件市场]
    B --> C[搜索 JDBC]
    C --> D[选择 MySQL profile]
    D --> E[通过 API 下载插件]
    E --> F[校验本地插件元数据与文件]
    F --> G[安装到集群]
    G --> H[打开集群插件页]
    H --> I[校验已安装插件详情]
    I --> J[搜索 file-obs]
    J --> K[下载 file-obs]
    K --> L[校验 lib 目录依赖]
```

## 时序图

```mermaid
sequenceDiagram
    participant PW as Playwright
    participant FE as Frontend
    participant BE as Backend
    participant Store as 插件市场
    participant Agent as Agent
    participant Node as 集群节点

    PW->>FE: 打开插件市场并选择插件
    FE->>BE: 请求插件元数据
    BE->>Store: 获取 profile 元数据
    Store-->>BE: 返回元数据
    BE-->>FE: 展示插件详情

    PW->>FE: 下载插件
    FE->>BE: 发起下载请求
    BE->>Store: 拉取插件资产
    Store-->>BE: 返回压缩包与元数据
    BE-->>FE: 下载成功

    PW->>FE: 安装插件到集群
    FE->>BE: 安装请求
    BE->>Agent: 分发插件资产
    Agent->>Node: 落盘插件文件
    Node-->>Agent: 安装成功
    Agent-->>BE: 安装成功
    BE-->>FE: 展示已安装插件详情
```
