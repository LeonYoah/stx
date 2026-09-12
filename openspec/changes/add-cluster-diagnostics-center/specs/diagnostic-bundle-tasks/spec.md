## ADDED Requirements

### Requirement: The system SHALL create diagnostic tasks from multiple trigger sources

The system SHALL allow operators to create a diagnostic bundle task manually or from an error group, inspection finding, or alert context. Each diagnostic task MUST persist its trigger source, source reference, cluster scope, selected nodes, and execution status.

#### Scenario: Create manual diagnostic task

- **WHEN** a user manually creates a diagnostic bundle for a cluster
- **THEN** the system SHALL persist a diagnostic task with trigger source `manual`

#### Scenario: Create diagnostic task from an inspection finding

- **WHEN** a user chooses "generate diagnostic bundle" from an inspection finding
- **THEN** the system SHALL create the task with a source reference to that finding and prefill the relevant cluster and nodes

### Requirement: Diagnostic bundle execution SHALL stream step and node progress

The system SHALL execute diagnostic bundle collection as a multi-step task with persisted task, step, node execution, and step log records. The UI MUST provide real-time progress updates and execution logs for each step and node.

#### Scenario: Observe running diagnostic task

- **WHEN** a diagnostic task is running
- **THEN** the system SHALL stream step status, node execution updates, and appended execution logs to the task detail view

#### Scenario: Handle failed step

- **WHEN** one collection step fails on one or more nodes
- **THEN** the task view SHALL surface the failed step, affected nodes, and failure message without hiding previously collected logs

### Requirement: Diagnostic bundle collection SHALL reuse existing diagnostic commands and managed evidence

The system SHALL reuse existing Agent diagnostic commands for log collection, thread dump, and JVM dump, and SHALL also include managed evidence such as recent process events, alert snapshots, error samples, and configuration snapshots in the bundle manifest.

#### Scenario: Collect JVM and thread evidence

- **WHEN** a diagnostic bundle includes JVM diagnostics for a selected node
- **THEN** the system SHALL invoke the existing JVM dump and thread dump collection commands and register the resulting files in the bundle manifest

#### Scenario: Skip optional dump collection by task options

- **WHEN** an operator creates a diagnostic bundle with thread dump or JVM dump disabled
- **THEN** the corresponding collection step SHALL be marked as skipped without failing the whole task

#### Scenario: Skip JVM dump when disk space is insufficient

- **WHEN** JVM dump collection is enabled but the target host does not have enough free disk space according to the task policy
- **THEN** the system SHALL skip JVM dump collection for that node and record the free space, required space, and skip reason in task logs and bundle metadata

#### Scenario: Defer binary HPROF upload in MVP

- **WHEN** JVM dump collection succeeds in the current MVP implementation
- **THEN** the system SHALL record the remote output path, size metadata, and collection status in the bundle manifest
- **AND** the system SHALL NOT require uploading the binary hprof artifact back to the control plane until the later enhancement tracked under 5.5

#### Scenario: Include recent managed evidence

- **WHEN** the diagnostic bundle is assembled
- **THEN** the system SHALL attach structured references to recent error groups, process events, alerts, and inspection context relevant to the source trigger

### Requirement: The system SHALL retain bundle metadata and source traceability

The system SHALL register each completed or failed diagnostic bundle with a manifest that records collected artifact metadata, source references, creation time, operator, and retention status. Users MUST be able to trace a bundle back to the originating error group, inspection finding, alert, or manual request.

#### Scenario: View bundle history

- **WHEN** a user opens the diagnostic task or bundle list
- **THEN** the system SHALL display recent tasks with their source type, status, cluster, and creation time

#### Scenario: Open bundle from related source

- **WHEN** a user views an error group or inspection result that already has generated bundles
- **THEN** the system SHALL show links to the related diagnostic bundle tasks and manifests

### Requirement: Diagnostic bundles SHALL include a human-readable diagnostic report

The system SHALL generate a human-readable diagnostic report together with the machine-readable manifest and JSON evidence so operators can open a completed bundle directly in a browser without relying on the STX UI.

#### Scenario: Open bundle summary offline

- **WHEN** an operator downloads or copies a completed diagnostic bundle out of the control plane
- **THEN** the bundle SHALL include a diagnostic report entry such as `index.html` or `inspection-report.html` that renders the key inspection, error, alert, and artifact metadata in a browser

#### Scenario: Open one diagnostic report to inspect health and every artifact

- **WHEN** an operator opens the generated diagnostic report directly from a completed bundle
- **THEN** the page SHALL present the cluster health overview, inspection findings, error context, alert/process evidence, task execution summary, and the registered artifacts
- **AND** each artifact SHALL expose its metadata and an inline preview or preview note so the operator can review the bundle contents without relying on the STX UI
