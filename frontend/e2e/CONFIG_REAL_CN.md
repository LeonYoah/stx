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

# 配置管理 Real 场景说明

Spec：

- `frontend/e2e/config-real.spec.ts`

## 入口命令

```bash
cd frontend
pnpm exec bash ./scripts/e2e/run-real-config.sh
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

- 先安装一个真实集群作为测试源
- 从节点实时文件初始化模板配置
- 编辑模板内容
- 执行 smart repair
- 保存模板但不立即下发
- 同步模板到所有节点
- 校验版本历史与版本对比
- 回滚到历史版本
- 在节点文件被外部修改后，再次从节点导入

## 执行流程图

```mermaid
flowchart TD
    A[安装源集群] --> B[打开配置页]
    B --> C[从节点初始化模板]
    C --> D[编辑模板]
    D --> E[执行 smart repair]
    E --> F[保存模板]
    F --> G[同步到节点]
    G --> H[校验模板与节点文件一致]
    H --> I[查看版本历史]
    I --> J[版本对比]
    J --> K[执行回滚]
    K --> L[同步回滚结果]
    L --> M[外部修改节点文件]
    M --> N[再次从节点导入]
    N --> O[校验模板已刷新]
```

## 时序图

```mermaid
sequenceDiagram
    participant PW as Playwright
    participant FE as Frontend
    participant BE as Backend
    participant Agent as Agent
    participant Node as 集群节点

    PW->>FE: 打开配置页
    PW->>FE: 从节点初始化模板
    FE->>BE: 拉取实时配置
    BE->>Agent: 读取节点文件
    Agent->>Node: 加载配置文件
    Node-->>Agent: 返回文件内容
    Agent-->>BE: 返回实时配置
    BE-->>FE: 初始化模板

    PW->>FE: 编辑 + smart repair + 保存
    FE->>BE: 保存模板
    BE-->>FE: 创建版本成功

    PW->>FE: 同步模板
    FE->>BE: 同步请求
    BE->>Agent: 分发配置到节点
    Agent->>Node: 写入配置文件
    Node-->>Agent: 写入成功
    Agent-->>BE: 同步成功
    BE-->>FE: 返回同步结果
```
