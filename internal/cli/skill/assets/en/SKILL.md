---
name: stx
description: Use the STX CLI to inspect, diagnose, and manage remote STX and SeaTunnel environments. Apply it to logs, clusters, configuration, installation, upgrades, diagnostic resources, and sync jobs; inspect capabilities and help before running safe commands.
---

# STX CLI

Use `stx` to interact with a remote STX service. Do not bypass STX to modify SeaTunnel directly on a server, and do not guess undocumented APIs or commands.

## Before you start

1. Run `stx --help` to see commands provided by the installed binary.
2. Run `stx namespace list` and `stx whoami` to confirm the target environment and current user.
3. Run `stx capability list` to discover operations supported by the remote server and allowed for the current user.
4. Run `--help` on the specific command and follow its parameters, impact notice, and examples.

Login example:

```bash
stx login --server http://127.0.0.1:17800
```

An interactive terminal can prompt for the username and password. In automation, use the command's non-interactive options and never place tokens in scripts, logs, or responses.

## Handling output

- Prefer `--output json`; read stdout as the single final JSON result.
- stderr carries progress, warnings, and error events. Do not treat stderr as the final business result.
- Use `--pick field1,field2` when only a few fields are needed. If a field is unavailable, use the complete safe result returned by STX.
- When `result_meta.complete=false`, inspect `reason` and `next_command`, then continue only after confirming that the command is safe.
- Download commands write local files. Verify the returned size and checksum when they finish.

## Safety rules

- Query first. Before modifications, installation, upgrades, or diagnostic collection, explain the target and possible impact to the user.
- Pass `--confirm` only when the user has authorized that change. Reuse the same `--idempotency-key` when retrying the same write request.
- Do not expose or attempt deletion commands. If deletion is needed, explain that the CLI does not support it and direct the user to the controlled UI.
- Never reveal passwords, tokens, keys, or original values hidden by server-side masking. Do not attempt to recover secrets from masks.
- Dumps require an administrator and may pause the JVM or increase CPU, disk, and network load. Show the performance notice from command help for other diagnostics as well.
- A cancellation request does not prove that execution stopped. Keep checking until the terminal status is `cancelled` before reporting completion.

## Suggested troubleshooting order

1. Inspect cluster, node, host, and recent job status.
2. Inspect relevant logs, configuration, metrics, submission history, and command audit records.
3. Run `stx diagnostics resource list` to discover diagnostic resources that can be collected separately.
4. Run only the resources needed for the current issue. Thread snapshots are disabled by default, and dumps should run only when necessary and authorized.
5. For asynchronous work, follow the returned `next_command` or use `stx execution get|wait <execution-id>` to track the actual state.

Commands change across STX versions. Treat command help and `capability list` as the source of truth for the current environment instead of maintaining a complete command list in this file.
