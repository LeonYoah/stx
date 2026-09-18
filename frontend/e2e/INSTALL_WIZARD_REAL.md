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

# Real Installer Scenario

Spec:

- `frontend/e2e/install-wizard-real.spec.ts`

## Entry commands

```bash
cd frontend
pnpm test:e2e:installer-real
# or
pnpm exec bash ./scripts/e2e/run-real-installer.sh
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

## Covered sub-scenarios

1. **Local checkpoint install**
   - run precheck
   - complete one-click installation
   - verify generated local config files

2. **MinIO-backed checkpoint + IMAP install**
   - run precheck
   - validate checkpoint configuration in the wizard
   - validate IMAP configuration in the wizard
   - complete one-click installation
   - verify generated SeaTunnel and Hazelcast configs
   - run post-install `stx-java-proxy` checkpoint probe
   - run post-install `stx-java-proxy` IMAP probe

## Execution flow

```mermaid
flowchart TD
    A[Open install wizard lab page] --> B[Run precheck]
    B --> C[Fill installer config]
    C --> D{Storage mode}
    D -->|Local checkpoint| E[Install directly]
    D -->|MinIO ck + IMAP| F[Validate checkpoint in wizard]
    F --> G[Validate IMAP in wizard]
    G --> E
    E --> H[Wait for installation success]
    H --> I[Assert generated config files]
    I --> J{MinIO scenario?}
    J -->|Yes| K[Run stx-java-proxy checkpoint probe]
    K --> L[Run stx-java-proxy IMAP probe]
    J -->|No| M[Done]
    L --> M
```

## Sequence

```mermaid
sequenceDiagram
    participant PW as Playwright
    participant FE as Frontend
    participant BE as Backend
    participant Agent as Agent
    participant ST as Installed SeaTunnel Home
    participant Proxy as stx-java-proxy
    participant MinIO as MinIO

    PW->>FE: open installer lab
    PW->>FE: run precheck
    FE->>BE: precheck request
    BE->>Agent: execute precheck
    Agent-->>BE: precheck result
    BE-->>FE: precheck passed

    PW->>FE: submit install
    FE->>BE: install request
    BE->>Agent: install SeaTunnel
    Agent->>ST: extract and configure runtime
    Agent->>MinIO: validate/use buckets when MinIO scenario
    Agent-->>BE: installation success
    BE-->>FE: success status

    PW->>ST: verify config files on disk
    PW->>Proxy: probe-once checkpoint
    Proxy->>MinIO: checkpoint read/write probe
    Proxy-->>PW: ok/readable/writable
    PW->>Proxy: probe-once imap
    Proxy->>MinIO: IMAP read/write probe
    Proxy-->>PW: ok/readable/writable
```
