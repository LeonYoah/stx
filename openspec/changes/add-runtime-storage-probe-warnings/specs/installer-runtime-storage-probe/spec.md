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

## ADDED Requirements

### Requirement: The installer SHALL attempt real runtime storage probes for remote checkpoint and IMAP backends

The system SHALL attempt a real SeaTunnel runtime storage probe for remote checkpoint and IMAP storage backends during installation after the SeaTunnel package has been extracted and before the related configuration step completes. The probe MUST use the `stx-java-proxy` one-shot CLI and the extracted installation directory as `SEATUNNEL_HOME`.

#### Scenario: Probe remote checkpoint storage during installation

- **WHEN** a host is installing SeaTunnel with checkpoint storage type `HDFS`, `S3`, or `OSS`
- **THEN** the Agent SHALL attempt a one-shot checkpoint runtime probe during the checkpoint configuration step

#### Scenario: Probe remote IMAP storage during installation

- **WHEN** a host is installing SeaTunnel with IMAP storage type `HDFS`, `S3`, or `OSS`
- **THEN** the Agent SHALL attempt a one-shot IMAP runtime probe during the IMAP configuration step

#### Scenario: Skip local-only storage runtime probe

- **WHEN** checkpoint or IMAP storage is configured as `LOCAL_FILE` or IMAP is `DISABLED`
- **THEN** the installer SHALL skip the runtime probe for that storage target

### Requirement: Runtime storage probe failures SHALL not block installation

If the runtime storage probe fails, times out, or cannot be started, the installer SHALL record a warning and continue installation. The related configuration step MUST still be allowed to finish successfully unless the configuration write itself fails.

#### Scenario: Continue installation when checkpoint probe fails

- **WHEN** the checkpoint runtime probe returns an error such as invalid credentials or bucket access denied
- **THEN** the installer SHALL keep the checkpoint configuration step non-fatal
- **AND** the installation SHALL continue with a warning that includes the probe failure summary

#### Scenario: Continue installation when IMAP probe fails

- **WHEN** the IMAP runtime probe returns an initialization or read/write failure
- **THEN** the installer SHALL keep the IMAP configuration step non-fatal
- **AND** the installation SHALL continue with a warning that includes the probe failure summary

#### Scenario: Continue installation when proxy assets are unavailable

- **WHEN** the Agent cannot locate the proxy script or the ordinary stx-java-proxy jar needed for one-shot execution
- **THEN** the installer SHALL skip the runtime probe
- **AND** the installation SHALL continue with a warning indicating that the real runtime probe was not executed

### Requirement: Installation warnings SHALL be visible to operators

The installer SHALL preserve warning messages emitted during runtime storage probing and expose them in installation status so the frontend can display them separately from fatal errors.

#### Scenario: Surface runtime probe warnings in installation status

- **WHEN** one or more runtime storage probes emit warning messages during installation
- **THEN** the installation status SHALL include a deduplicated warnings collection
- **AND** the frontend SHALL render those warnings in the installation progress view

### Requirement: Capability proxy assets SHALL be distributed with installable artifacts

The system SHALL package the stx-java-proxy thin jar and launcher script with installable STX artifacts and SHALL install them onto target hosts as part of Agent distribution so runtime storage probes do not depend on a local source checkout. The Agent SHALL prefer a jar whose file name matches the SeaTunnel cluster version and SHALL fall back to the packaged `2.3.13` jar when an exact versioned jar is unavailable.

#### Scenario: Bundle stx-java-proxy assets in the control-plane release package

- **WHEN** the control-plane release package is built
- **THEN** it SHALL include `lib/stx-java-proxy-2.3.13.jar`
- **AND** it SHALL include `scripts/stx-java-proxy.sh`

#### Scenario: Install stx-java-proxy assets with the Agent

- **WHEN** an operator runs the generated Agent install script
- **THEN** the script SHALL download the stx-java-proxy jar and script from the control plane
- **AND** it SHALL install them into a fixed local support directory
- **AND** the Agent process SHALL receive environment variables pointing at the installed support home and launcher script path

#### Scenario: Pick a versioned stx-java-proxy jar with fallback

- **WHEN** the Agent runs a runtime storage probe for SeaTunnel version `X`
- **THEN** it SHALL first look for `stx-java-proxy-X.jar`
- **AND** if that jar is unavailable it SHALL fall back to `stx-java-proxy-2.3.13.jar`
