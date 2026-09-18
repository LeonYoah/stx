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

# 一键安装 Real 场景说明

Spec：

- `frontend/e2e/install-wizard-real.spec.ts`

## 入口命令

```bash
cd frontend
pnpm test:e2e:installer-real
# 或
pnpm exec bash ./scripts/e2e/run-real-installer.sh
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

## 当前覆盖的子场景

1. **本地 checkpoint 安装**
   - 执行 precheck
   - 完成一键安装
   - 校验生成的本地配置文件

2. **MinIO-backed checkpoint + IMAP 安装**
   - 执行 precheck
   - 在向导中校验 checkpoint 配置
   - 在向导中校验 IMAP 配置
   - 完成一键安装
   - 校验生成的 SeaTunnel / Hazelcast 配置
   - 安装后执行 `stx-java-proxy` checkpoint probe
   - 安装后执行 `stx-java-proxy` IMAP probe

## 执行流程图

```mermaid
flowchart TD
    A[打开安装向导实验页] --> B[执行 precheck]
    B --> C[填写安装配置]
    C --> D{存储模式}
    D -->|本地 checkpoint| E[直接安装]
    D -->|MinIO ck + IMAP| F[在向导中校验 checkpoint]
    F --> G[在向导中校验 IMAP]
    G --> E
    E --> H[等待安装成功]
    H --> I[校验生成的配置文件]
    I --> J{是否 MinIO 场景}
    J -->|是| K[执行 stx-java-proxy checkpoint probe]
    K --> L[执行 stx-java-proxy IMAP probe]
    J -->|否| M[结束]
    L --> M
```

## 时序图

```mermaid
sequenceDiagram
    participant PW as Playwright
    participant FE as Frontend
    participant BE as Backend
    participant Agent as Agent
    participant ST as 安装后的 SeaTunnel Home
    participant Proxy as stx-java-proxy
    participant MinIO as MinIO

    PW->>FE: 打开安装向导
    PW->>FE: 执行 precheck
    FE->>BE: precheck 请求
    BE->>Agent: 执行预检查
    Agent-->>BE: 返回预检查结果
    BE-->>FE: precheck 通过

    PW->>FE: 提交安装
    FE->>BE: 安装请求
    BE->>Agent: 安装 SeaTunnel
    Agent->>ST: 解压并生成运行时配置
    Agent->>MinIO: MinIO 场景下访问 bucket
    Agent-->>BE: 安装成功
    BE-->>FE: 成功状态

    PW->>ST: 校验磁盘上的配置文件
    PW->>Proxy: probe-once checkpoint
    Proxy->>MinIO: checkpoint 读写探测
    Proxy-->>PW: ok/readable/writable
    PW->>Proxy: probe-once imap
    Proxy->>MinIO: IMAP 读写探测
    Proxy-->>PW: ok/readable/writable
```
