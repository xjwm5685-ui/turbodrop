# Changelog

All notable changes to this project are documented in this file.

The format loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [v1.1.0] - 2026-06-14

### Added
- LAN bridge mode for phone/computer two-way transfer.
- Phone-to-computer browser uploads saved to the configured computer save folder.
- Computer-to-phone shared download list for files selected on the computer.
- Windows portable starter script: `START_TURBODROP.bat`.
- Portable quick start guide: `QUICKSTART.md`.
- Firewall scripts now cover Web UI TCP 48080, PIN UDP 8899, and QUIC UDP 9001.

### Changed
- Web UI redesigned as a practical dispatch console with clearer RX/TX/LAN/CFG workflows.
- Default Web host now listens on `0.0.0.0` for local-network access.
- Startup logs now print both localhost and the preferred LAN URL.
- LAN visitors see the transfer-focused interface by default.
- Local IP detection now prefers real private LAN adapters over tunnel/test adapters.

### Fixed
- Embedded Web UI is served correctly in standalone portable builds.
- LAN visitors can no longer open the computer settings panel or native folder picker.
- LAN visitors can no longer request sends from arbitrary computer-local file paths.

## [v1.0.0] - 2026-06-08

### Added
- Phase 5 engineering baseline: Go unit tests for discovery, upload path handling, and transfer state persistence.
- Cross-platform build scripts: `build-all.ps1` and `build-all.sh`.
- GitHub Actions workflow for test/build and tagged release artifacts.
- Release-oriented docs: `API.md`, `SECURITY.md`, and `CONTRIBUTING.md`.
- Multi-file queue sending in the Web UI and backend API.
- Persistent transfer history stored in `./data/transfer_history.json`.
- Persistent application config stored in `./data/app_config.json`.
- Settings panel in the Web UI for device name, ports, concurrency, and chunk size.
- PIN two-step challenge-response handshake with per-session salt.
- TLS certificate fingerprint pinning via PIN discovery channel.
- Graceful shutdown with HTTP server, QUIC listener, and background task cleanup.
- JSON body size limits on all API endpoints.
- Background task lifecycle tracking with `sync.WaitGroup`.
- `go install` support with official module path.

### Changed
- `build.bat` and `test.bat` now align with the current Web UI based workflow and automated validation.
- Project status and README will track Phase 5 engineering progress instead of Phase 4 only.
- Dashboard now loads persisted transfer history and shows a pending send queue before transmission starts.
- Transfer runtime now reads configurable chunk size and max concurrent streams.
- WebSocket/CORS restricted to localhost origins instead of wildcard `*`.
- Resume stream count now correctly uses remaining chunks instead of total chunks.

### Fixed
- Path traversal vulnerability in received file names.
- Hub broadcast race condition (mutating map under RLock).
- QUIC sender `file.Seek` return value now checked.
- QUIC receiver `file.Seek` return value now checked.
- Sender now properly closes QUIC uni-stream after writing each chunk.
- Error aggregation in concurrent chunk transfer (all errors returned, not just first).

## [v0.4.0] - 2026-06-07

### Added
- Phase 4 Web UI with radar animation, progress panel, speed chart, and transfer history.
- Browser upload support through the local TurboDrop API.
- End-to-end local transfer validation for Web UI + QUIC flow.
