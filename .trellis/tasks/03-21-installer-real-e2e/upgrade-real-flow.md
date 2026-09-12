# Upgrade Real E2E Flow

## Purpose

This document captures the real end-to-end upgrade verification flow added for `upgrade-real`.
It is intended to explain:

- what the test actually provisions
- which control-plane and agent APIs are exercised
- which upgrade behaviors are verified
- how incremental CI decides whether to run the suite

## Scope

The `upgrade-real` suite verifies a **real single-node SeaTunnel cluster upgrade**:

- source version: `2.3.12`
- target version: `2.3.13`
- package source: online download
- execution path: Playwright -> Control Plane -> Agent -> SeaTunnel install dir
- final validation: real generated files on disk

## Sequence Diagram

```mermaid
sequenceDiagram
    autonumber
    participant GH as GitHub Actions / E2E Workflow
    participant Harness as run-real-upgrade.sh
    participant FE as Playwright Browser
    participant CP as STX Control Plane
    participant AG as Agent
    participant FS as Install Dir / Config Files
    participant PKG as Package Cache

    GH->>Harness: start upgrade-real suite
    Harness->>CP: boot temporary backend
    Harness->>AG: boot temporary agent
    Harness->>FE: start frontend + Playwright

    FE->>CP: login as admin
    FE->>CP: GET /api/v1/hosts
    CP-->>FE: online host fixture

    FE->>CP: POST /api/v1/clusters
    CP-->>FE: cluster_id
    FE->>CP: POST /api/v1/clusters/:id/nodes
    CP-->>FE: node created

    FE->>CP: POST /api/v1/hosts/:hostId/install (source 2.3.12)
    CP->>PKG: resolve/download source package if needed
    CP->>AG: install source version
    AG->>FS: extract source package
    AG->>FS: write seatunnel.yaml / hazelcast.yaml / hazelcast-client.yaml / log4j2.properties
    AG->>CP: report install status
    CP-->>FE: source install success

    FE->>CP: GET /api/v1/packages/2.3.13
    alt target package missing locally
        FE->>CP: POST /api/v1/packages/download
        CP->>PKG: download target package 2.3.13
        CP-->>FE: package ready
    end

    FE->>CP: open /clusters/:id/upgrade/prepare
    FE->>CP: POST /api/v1/st-upgrade/precheck
    CP->>CP: build node targets
    CP->>CP: build connector/lib/plugins manifests
    CP-->>FE: precheck result

    alt config merge requires manual choice
        FE->>CP: open /clusters/:id/upgrade/config
        FE->>FE: apply old/new file decisions for each config
    end

    FE->>CP: POST /api/v1/st-upgrade/plan
    CP-->>FE: plan_id
    FE->>CP: POST /api/v1/st-upgrade/execute
    CP->>AG: execute upgrade steps

    AG->>FS: sync lib manifest
    AG->>FS: sync connectors manifest
    AG->>FS: sync plugins manifest
    AG->>FS: merge config files
    AG->>FS: switch install dir/version
    AG->>FS: restart cluster
    AG->>FS: run smoke test
    AG->>CP: report task step status
    CP-->>FE: upgrade task success

    FE->>FS: read target seatunnel.yaml
    FE->>FS: read target hazelcast.yaml
    FE->>FS: read target hazelcast-client.yaml
    FE->>FS: read target log4j2.properties
    FE->>FE: assert target version + generated config content
```

## What the test verifies

### 1. Source cluster creation is real

The suite does **not** mock an upgraded cluster.
It really:

- creates a cluster record
- adds a node
- installs source version `2.3.12`
- waits for host install success

### 2. Target package preparation is real

Before upgrade execution, the suite ensures target package `2.3.13` exists locally.
If absent, it triggers package download through the normal package API.

### 3. Upgrade prepare/config/execute pages are real

The browser goes through:

- `/clusters/:id/upgrade/prepare`
- `/clusters/:id/upgrade/config`
- `/clusters/:id/upgrade/execute`

No page-level mocks are used for the core upgrade flow.

### 4. Upgrade execution is real

The suite waits for a successful upgrade task and requires the expected step chain to appear, including:

- `SWITCH_VERSION`
- `START_CLUSTER`
- `HEALTH_CHECK`
- `SMOKE_TEST`
- `COMPLETE`

### 5. Final file assertions are real

After upgrade succeeds, the test reads the actual target install dir and asserts key content in:

- `config/seatunnel.yaml`
- `config/hazelcast.yaml`
- `config/hazelcast-client.yaml`
- `config/log4j2.properties`

## Config assertions currently covered

### seatunnel.yaml

The suite verifies target runtime config such as:

- HTTP enabled
- HTTP port migrated correctly
- checkpoint namespace retained
- `fs.defaultFS: file:///`

### hazelcast.yaml

The suite verifies IMAP state for the test scenario (currently disabled for this flow).

### hazelcast-client.yaml

The suite verifies cluster members are written using the **real online host IP** and target Hazelcast port.

### log4j2.properties

The suite verifies the expected log mode mapping after upgrade.

## Incremental CI trigger design

`upgrade-real` should run only when relevant upgrade/install/config paths change.

Current trigger buckets include:

- `internal/apps/stupgrade/**`
- `internal/apps/plugin/**`
- `internal/apps/config/**`
- `internal/apps/cluster/**`
- `internal/seatunnel/**`
- `agent/internal/installer/**`
- `frontend/components/common/cluster/upgrade/**`
- `frontend/e2e/upgrade-real.spec.ts`
- `frontend/e2e/helpers/upgrade-real.ts`
- `frontend/scripts/e2e/run-real-upgrade.sh`
- `frontend/scripts/e2e/run-real-installer.sh`
- `config.e2e.installer-real.yaml`
- `config.e2e.agent-real.yaml`

This keeps the suite:

- **enabled in remote CI**
- **incremental for pull requests**
- isolated from ordinary smoke selection

## Resource notes

### CI

In GitHub Actions, this suite is allowed to create and discard temporary resources because the runner is ephemeral.
Local destructive cleanup is intentionally not required for CI success.

### Local development

This suite is heavy for small machines.
For low-memory environments, the preferred mode is:

- let remote CI run it
- avoid repeated local execution unless actively debugging the flow

## Known maintenance rules

1. Do not commit downloaded package cache under `frontend/lib/packages/`.
2. Keep `upgrade-real` excluded from ordinary smoke selection.
3. Always assert `hazelcast-client.yaml` using the real resolved host IP, not a hard-coded loopback value.
4. If config merge introduces conflicts, the suite must explicitly resolve them before creating an upgrade plan.
