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

# Managed STX Java Proxy Service

## Why

`probe-once` works for install-time validation, but it is a poor fit for repeated storage diagnostics:

- every operation starts a new JVM
- file browsing and repeated probes become expensive
- later storage-tab interactions need a stable API surface

We therefore move toward a **managed stx-java-proxy service**.

## Target direction

The stx-java-proxy becomes a platform-managed component that:

- starts on the master node after install
- serves HTTP APIs for checkpoint / IMAP diagnostics
- is reused by both install-time probes and post-install storage inspection
- is later surfaced in the platform as a managed auxiliary service with status / restart / logs

## First-pass implementation in this change

This change implements the install-time foundation:

1. Runtime storage probes now **try a managed HTTP proxy service first**.
2. If the service is not available, installer code **falls back to existing `probe-once` CLI behavior**.
3. When SeaTunnel runtime has already been extracted, installer code can **lazy-start** a local proxy service by running:
   - `scripts/stx-java-proxy.sh`
   - with `-Dstx.java.proxy.port=<port>`
4. Service state is persisted under:
   - `<SEATUNNEL_HOME>/.stx/stx-java-proxy/`
   - including `service.port`, `service.pid`, and `service.log`

## Behavior

### Install-time probe path

For checkpoint / IMAP runtime probe:

1. use `STX_JAVA_PROXY_ENDPOINT` if explicitly configured
2. else reuse an already healthy local managed proxy service if found
3. else start a local managed proxy service lazily
4. if any of the above fails, fall back to `probe-once`

This keeps current installs backward compatible while enabling service-based probing.

## New environment knobs

- `STX_JAVA_PROXY_ENDPOINT`: force installer probes to use an existing proxy service
- `STX_JAVA_PROXY_PORT`: preferred port for the lazily started managed service

## What is not yet included

This first pass does **not** yet provide:

- control-plane start / stop / restart APIs
- health/state persistence in platform metadata
- UI exposure for proxy lifecycle management
- storage-tab list / stat / inspect APIs wired into the control plane

## Recommended next phases

### Phase 2: platform-managed lifecycle

- record proxy deployment/status per cluster
- add control-plane APIs for status / restart / logs
- auto-heal or auto-restart unhealthy proxy service

### Phase 3: storage inspector

- checkpoint / IMAP probe
- file listing
- size/stat operations
- later checkpoint deserialize / deep diagnostics
