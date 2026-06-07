# Changelog

All notable changes to this project are documented in this file.

The format loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added
- Phase 5 engineering baseline: Go unit tests for discovery, upload path handling, and transfer state persistence.
- Cross-platform build scripts: `build-all.ps1` and `build-all.sh`.
- GitHub Actions workflow for test/build and tagged release artifacts.
- Release-oriented docs: `API.md`, `SECURITY.md`, and `CONTRIBUTING.md`.
- Multi-file queue sending in the Web UI and backend API.
- Persistent transfer history stored in `./data/transfer_history.json`.
- Persistent application config stored in `./data/app_config.json`.
- Settings panel in the Web UI for device name, ports, concurrency, and chunk size.

### Changed
- `build.bat` and `test.bat` now align with the current Web UI based workflow and automated validation.
- Project status and README will track Phase 5 engineering progress instead of Phase 4 only.
- Dashboard now loads persisted transfer history and shows a pending send queue before transmission starts.
- Transfer runtime now reads configurable chunk size and max concurrent streams.

## [v0.4.0] - 2026-06-07

### Added
- Phase 4 Web UI with radar animation, progress panel, speed chart, and transfer history.
- Browser upload support through the local TurboDrop API.
- End-to-end local transfer validation for Web UI + QUIC flow.
