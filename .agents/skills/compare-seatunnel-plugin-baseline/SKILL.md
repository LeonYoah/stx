---
name: compare-seatunnel-plugin-baseline
description: "End-to-end procedural workflow for upgrading SeaTunnel versions across connector baseline seed, stx-java-proxy engine adapters, Go backend, and frontend UI."
---

# SeaTunnel Version Upgrade & Baseline Adaptation Workflow

Use this skill when upgrading SeaTunnel version support in SeaTunnelX (e.g. adapting from 2.3.x to 3.0.x or minor releases like 3.0.0 GA / 3.1.0).

Upgrading SeaTunnel support is **not** a simple script execution; upstream releases frequently introduce structural changes, dependency reshuffles, and engine protocol updates. This skill defines a **flexible, human-in-the-loop, multi-layer procedural workflow**.

---

## Architecture Scope & Upgrade Layers

When adapting a new SeaTunnel version, inspect and synchronize four interconnected layers:

```
┌────────────────────────────────────────────────────────────────────────┐
│ Layer 1: Connector Baseline Seed (seatunnel-plugins.json)              │
│ - Connector catalog detection (connectors-v2, plugin-mapping)         │
│ - Provided dependency extraction & JDBC dialect driver profiles       │
├────────────────────────────────────────────────────────────────────────┤
│ Layer 2: Java Proxy Sidecar (tools/stx-java-proxy)                     │
│ - EngineAdapter version policy & dynamic version routing               │
│ - Checkpoint storage deserialization & incremental state compatibility │
│ - WebUI DAG preview & SPI plugin classloader compatibility            │
├────────────────────────────────────────────────────────────────────────┤
│ Layer 3: Go Backend Engine Client (internal/apps/sync)                 │
│ - REST API versioning (Zeta REST V1 vs REST V2)                        │
│ - Multi-table DAG vertex array/map format compatibility               │
│ - Table-level metrics aggregation (TableSourceReceived/TableSinkWrite) │
├────────────────────────────────────────────────────────────────────────┤
│ Layer 4: Frontend WebUI & Sync Studio (frontend/components/.../sync)   │
│ - WebUiDagPreview vertex normalization & multi-table edge rendering    │
│ - Job runs summary metrics with multi-table capsule & DAG drawer       │
└────────────────────────────────────────────────────────────────────────┘
```

---

## Step-by-Step Procedure

### Phase 1: Target Version & Source Code Recon

1. **Locate Upstream SeaTunnel Source**:
   - Check local checkout (e.g. `/Users/mac/Documents/projects/seatunnel` or git submodule) or clone/fetch official tag:
     ```bash
     git -C /path/to/seatunnel fetch --tags
     git -C /path/to/seatunnel checkout <target-tag-or-branch>
     ```
2. **Confirm Baseline vs Target**:
   - Baseline: check the latest entry in `reviewed_versions` inside `internal/apps/plugin/seed/seatunnel-plugins.json`.
   - Target: the version being evaluated (e.g. `3.0.0-SNAPSHOT`, `3.0.0`, `3.1.0`).

---

### Phase 2: Layer 1 — Connector Catalog & Dependency Baseline

Do not use rigid static scripts. Perform dynamic comparison via shell / git commands:

1. **Connector Catalog Diff**:
   - Compare `seatunnel-connectors-v2` directories and `plugin-mapping.properties`:
     ```bash
     # List all connectors in target version
     ls -d /path/to/seatunnel/seatunnel-connectors-v2/connector-*
     # Inspect connector mapping definitions
     cat /path/to/seatunnel/plugin-mapping.properties
     ```
   - Identify:
     - **New connectors**: catalog names not present in `seatunnel-plugins.json` catalog list.
     - **Removed / renamed connectors**: check if any module was relocated or renamed.
2. **Dependency & `provided` Scope Inspection**:
   - For each new or modified connector, inspect its `pom.xml`:
     - **Only elevate `provided` dependencies to automatic review candidates.**
     - Treat `compile` / `runtime` dependencies as internal engine bundles or shaded jars (do NOT turn compile deps into mandatory baseline downloads unless official docs specifically require external jar placement).
     - Check companion connectors (e.g. `connector-http-base` for HTTP connectors, `connector-file-base` for file-based connectors).
3. **JDBC Dialect & Driver Profiles**:
   - Inspect `seatunnel-connectors-v2/connector-jdbc/pom.xml` and dialect source directories:
     ```bash
     ls /path/to/seatunnel/seatunnel-connectors-v2/connector-jdbc/src/main/java/org/apache/seatunnel/connectors/seatunnel/jdbc/internal/dialect/
     ```
   - Identify newly added dialects (e.g. `yashandb`, `oceanbase`, `dameng`).
   - Extract official driver maven coordinates (`groupId:artifactId:version`) and JDBC prefix from the dialect factory.
4. **Prepare Human Review Proposal for Baseline Seed**:
   - Structure differences into:
     - New connectors to append to `catalog`
     - Connectors classified as `default_not_required` (self-contained, no external driver needed)
     - Connectors with companion profiles (`default_companion_profiles`)
     - New JDBC database profiles under `connector-jdbc`
     - Version bumps to `reviewed_versions`

---

### Phase 3: Layer 2 — Java Proxy Sidecar (`stx-java-proxy`) Adaptation

SeaTunnelX uses `tools/stx-java-proxy` as an out-of-process Java sidecar to validate configs, preview DAGs, and inspect checkpoint / IMap storage. When SeaTunnel upgrades, audit `stx-java-proxy`:

1. **Dependency Audit (`tools/stx-java-proxy/pom.xml`)**:
   - Check if SeaTunnel core dependencies have breaking API changes:
     - `seatunnel-api`
     - `seatunnel-core-starter`
     - `seatunnel-engine-core`
     - `checkpoint-storage-api` / `imap-storage-api`
   - Check transitive shading: `seatunnel-hazelcast-shade`, `seatunnel-hadoop3-*-uber`.
2. **Engine Adapter & Version Routing (`io.github.leonyoah.stx.proxy.adapter`)**:
   - Review `EngineVersion.java`: verify version string parsing (`major.minor.patch`).
   - Review `EngineAdapterVersionPolicy.java`:
     - Does the version pattern (e.g. `3.0.*`, `3.1.*`) resolve to `ADAPTER_V30` or require a new adapter key?
     - Forward compatibility: does `3.x` fall back cleanly to `SeaTunnel30EngineAdapter`?
   - Review `SeaTunnelEngineAdapterRegistry.java`:
     - Ensure all available adapters are registered.
3. **Checkpoint Storage Deserialization (`SeaTunnel30EngineAdapter.java`)**:
   - SeaTunnel 3.0 introduced:
     - Incremental checkpoints (`incrementalStateFiles`)
     - Savepoint flags (`checkpointType`)
     - Multi-table checkpoint states (`getTableStates()`)
   - If target version introduces new checkpoint layout, update `deserializeCheckpoint`, `inspectSources`, and `inspectSinks` with reflection-safe extraction (`extractMapQuietly`).
4. **DAG Preview & SaveMode Compatibility (`WebUiDagPreviewService.java`)**:
   - Verify DAG topological sort handles 3.0 multi-table sources and split/merge transforms.
   - Verify `JobConfigSupportService` handles multi-table catalog path extraction.
5. **Java Proxy Verification**:
   - Run adapter unit tests:
     ```bash
     mvn test -Dtest=SeaTunnelEngineAdapterRegistryTest,EngineVersionTest,EngineAdapterVersionPolicyTest -f tools/stx-java-proxy/pom.xml
     ```

---

### Phase 4: Layer 3 & 4 — Go Backend & Frontend WebUI Sync

1. **Go Engine Client (`internal/apps/sync/engine_client.go`)**:
   - Check Zeta REST API format differences:
     - Vertex info format: Zeta 2.3 used Map (`map[string]interface{}`), Zeta 3.0 uses Array (`[]interface{}`) containing vertices with `vertexId` and `vertexName`.
     - Multi-table metric aggregation: Zeta 3.0 reports `TableSourceReceivedCount` and `TableSinkWriteCount` as comma-separated or integer values per table.
   - Run Go backend tests:
     ```bash
     go test -v ./internal/apps/plugin/... ./internal/apps/sync/...
     ```
2. **Frontend DAG & Metrics (`frontend/components/common/sync/`)**:
   - `sync-studio-utils.ts`: verify `extractJobMetricSummary` normalizes multi-table metrics and provides `tableCount` and `isMultiTable`.
   - `WebUiDagPreview.tsx`: verify numeric edge vertex ID parsing and multi-table path display.
   - `ConsolePanel.tsx`: verify multi-table capsule badge and DAG drawer trigger.
   - Run frontend checks:
     ```bash
     pnpm exec tsc --noEmit
     pnpm test
     ```
     *(Note: strictly avoid `pnpm build`)*

---

### Phase 5: Human Review Checkpoint

**Never apply version upgrades silently.** Always present a concise review table to the user:

```markdown
### SeaTunnel Version Upgrade Review (Baseline: <base> -> Target: <target>)

#### 1. Connector Catalog Changes
| Connector | Change Type | Mapping | Recommended Action |
| --- | --- | --- | --- |
| connector-xyz | Added | connector-xyz | Add to catalog, default_not_required |

#### 2. JDBC Profile Changes
| Database | Driver Coordinates | Dialect Supported | Recommended Action |
| --- | --- | --- | --- |
| yashandb | com.yashandb:yashandb-jdbc:1.10.7 | Yes | Add profile to connector-jdbc |

#### 3. Java Proxy (stx-java-proxy) Compatibility
| Component | Status | Required Change |
| --- | --- | --- |
| EngineAdapter | Compatible (3.0.x fallback) | None / Add mapping rule |
| Checkpoint Storage | Audited | Incremental & table-level state verified |
| DAG Preview | Verified | Multi-table vertex format supported |

#### 4. Backend & Frontend Compatibility
| Item | Verification Result |
| --- | --- |
| Go REST V2 Client | Verified with 3.0 array vertex & table metrics |
| WebUI DAG & Metrics | Verified with multi-table badge and DAG view |
```

Wait for explicit user approval before applying changes to `seatunnel-plugins.json` or proxy source code.

---

### Phase 6: Application & Post-Upgrade Quality Gate

Once approved by the user:

1. **Apply baseline edits** to `internal/apps/plugin/seed/seatunnel-plugins.json`.
2. **Apply proxy edits** in `tools/stx-java-proxy` if new adapter or version policy rules are needed.
3. **Run Full Verification**:
   - `mvn test -Dtest=SeaTunnelEngineAdapterRegistryTest,EngineVersionTest,EngineAdapterVersionPolicyTest -f tools/stx-java-proxy/pom.xml`
   - `go test -v ./internal/apps/plugin/... ./internal/apps/sync/...`
   - `pnpm exec tsc --noEmit`
   - `pnpm test`
4. **Git Commit Guidelines**:
   - Use Chinese for git commit messages.
   - Bilingual code comments: "中文在前，英文在后" (Chinese first, English second).
