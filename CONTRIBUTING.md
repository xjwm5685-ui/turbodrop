# Contributing to TurboDrop

Thanks for your interest in improving TurboDrop.

## Development Setup

```bash
git clone https://github.com/xjwm5685-ui/turbodrop.git
cd turbodrop
go mod download
go build -o turbodrop.exe .
```

Run the Web UI locally:

```bash
./turbodrop.exe
```

Open:

```text
http://localhost:48080/dashboard.html
```

## Recommended Workflow

1. Create a focused feature branch.
2. Keep changes scoped to a clear problem or feature.
3. Add or update tests when the change affects behavior.
4. Run local verification before opening a PR.

## Local Verification

```bash
go test ./...
go build ./...
```

Optional:

```bash
go test -race ./...
go test -coverprofile=coverage.txt ./...
```

## Code Style

- Prefer small, focused functions.
- Keep comments concise and useful.
- Follow existing Go package boundaries.
- Avoid unrelated formatting-only churn.
- Preserve Windows compatibility for scripts and paths where practical.

## Pull Requests

A good pull request should include:

- what changed
- why it changed
- how it was tested
- screenshots or logs for UI/runtime changes when helpful

## Areas That Need Help

- multi-file transfer queue
- settings panel
- release packaging
- broader integration and performance testing
