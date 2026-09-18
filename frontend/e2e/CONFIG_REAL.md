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

# Real Config Scenario

Spec:

- `frontend/e2e/config-real.spec.ts`

## Entry command

```bash
cd frontend
pnpm exec bash ./scripts/e2e/run-real-config.sh
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
- initialize template configs from live node files
- edit template content
- run smart repair
- save template without immediate sync
- sync template to all nodes
- verify version history and compare
- rollback to a previous version
- import config from node again after out-of-band file modification

## Execution flow

```mermaid
flowchart TD
    A[Install source cluster] --> B[Open cluster config tab]
    B --> C[Init template from node]
    C --> D[Edit template]
    D --> E[Run smart repair]
    E --> F[Save template]
    F --> G[Sync template to nodes]
    G --> H[Verify file + template match]
    H --> I[Open version history]
    I --> J[Compare versions]
    J --> K[Rollback]
    K --> L[Sync rollback result]
    L --> M[Modify live file out of band]
    M --> N[Init from node again]
    N --> O[Verify template refreshed from live node]
```

## Sequence

```mermaid
sequenceDiagram
    participant PW as Playwright
    participant FE as Frontend
    participant BE as Backend
    participant Agent as Agent
    participant Node as Cluster Node

    PW->>FE: open config tab
    PW->>FE: initialize template from node
    FE->>BE: fetch live config
    BE->>Agent: read node file
    Agent->>Node: load config file
    Node-->>Agent: file content
    Agent-->>BE: live config
    BE-->>FE: template initialized

    PW->>FE: edit + smart repair + save
    FE->>BE: save template
    BE-->>FE: version created

    PW->>FE: sync template
    FE->>BE: sync request
    BE->>Agent: distribute config to nodes
    Agent->>Node: write config file
    Node-->>Agent: write success
    Agent-->>BE: sync success
    BE-->>FE: sync result
```
