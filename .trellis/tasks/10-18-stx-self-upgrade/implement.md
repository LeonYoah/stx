# Implement notes (2026-10-18)

## Delivered
- Backend `internal/apps/selfupgrade` with admin-only `/api/v1/self-upgrade/*`
- Failure codes: check_fail, already_latest, download_partial, bad_offline_import, forbidden (middleware 403), step_fail_*, verify_fail, concurrent_upgrade, disk_full, cancel
- Frontend `/system-upgrade` + Profile「检查更新」+ i18n zh/en
- Local mock staging writes stub assets under `./data/self-upgrade/v*/` including `stx-java-proxy-*.jar` glob

## Stubbed
- Real binary replace / process restart (`Execute.Apply` hooks are dry-run; packages are stub files in mock mode)
- Real GitHub asset download bytes (mock materializes placeholders; live mode only resolves latest tag via GitHub API)
- Offline archive unpack (validates extension + non-empty upload, then materializes catalog stubs)
- Audit log write (P1 deferred)

## Smoke
- Unit tests: `go test ./internal/apps/selfupgrade`
- API: check / check_fail / already_latest / download_partial / step_fail_agent / non-admin 403 / happy execute
- UI page compiles at `/system-upgrade`
