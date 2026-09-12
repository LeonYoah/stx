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

# Real Plugin Scenario

Spec:

- `frontend/e2e/plugin-real.spec.ts`

## Entry command

```bash
cd frontend
pnpm exec bash ./scripts/e2e/run-real-plugin.sh
```

## Shared real-runner logic

All real suites reuse the same real-install harness.

### What the harness prepares

`scripts/e2e/run-real-installer.sh`:

1. creates a temporary working directory under `tmp/e2e/installer-real.*`
2. generates temporary backend and agent config files
3. starts a temporary MinIO container when needed
4. creates checkpoint and IMAP buckets for MinIO-backed flows when needed
5. ensures `stx-java-proxy` jar is available for post-install checks
6. starts:
   - temporary backend
   - temporary agent supervisor
   - frontend dev server
7. runs the selected Playwright spec

## Covered behaviors

- install a real cluster as test source
- browse plugin marketplace
- choose JDBC MySQL profile
- download real plugin assets through API
- verify downloaded local assets and metadata
- install plugin to real cluster
- verify plugin details in cluster plugin view
- download and verify `file-obs` assets including `lib` dependencies

## Execution flow

```mermaid
flowchart TD
    A[Install source cluster] --> B[Open plugin marketplace]
    B --> C[Search JDBC]
    C --> D[Select MySQL profile]
    D --> E[Download plugin via API]
    E --> F[Verify local plugin metadata and files]
    F --> G[Install plugin to cluster]
    G --> H[Open cluster plugin tab]
    H --> I[Verify installed plugin detail]
    I --> J[Search file-obs]
    J --> K[Download file-obs]
    K --> L[Verify lib-target dependencies]
```

## Sequence

```mermaid
sequenceDiagram
    participant PW as Playwright
    participant FE as Frontend
    participant BE as Backend
    participant Store as Plugin Marketplace
    participant Agent as Agent
    participant Node as Cluster Node

    PW->>FE: open marketplace and select plugin
    FE->>BE: request plugin metadata
    BE->>Store: fetch profile metadata
    Store-->>BE: metadata
    BE-->>FE: show plugin detail

    PW->>FE: download plugin
    FE->>BE: download request
    BE->>Store: fetch plugin assets
    Store-->>BE: archives and metadata
    BE-->>FE: download success

    PW->>FE: install plugin to cluster
    FE->>BE: install request
    BE->>Agent: distribute plugin assets
    Agent->>Node: place plugin files
    Node-->>Agent: install success
    Agent-->>BE: install success
    BE-->>FE: render installed plugin detail
```
