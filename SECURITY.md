# Security Policy

## Supported Versions

TurboDrop is currently maintained on the latest development line in this repository.

| Version | Supported |
| ------- | --------- |
| main / master | Yes |
| older snapshots | No |

## Reporting a Vulnerability

If you discover a security issue, please avoid opening a public issue with exploit details.

Recommended process:

1. Prepare a minimal reproduction or proof of concept.
2. Describe impact, affected endpoints, and expected risk.
3. Send the report privately to the project maintainer or through a private security channel if one is published later.

Until a dedicated security contact is configured, include the following in your report:

- affected commit or version
- environment and operating system
- steps to reproduce
- observed behavior
- expected safe behavior
- any temporary mitigation

## Current Security Considerations

- TurboDrop is designed for local network use.
- PIN-based discovery reduces accidental peer selection but is not a substitute for full identity verification.
- QUIC transport uses TLS.
- Uploaded browser files are stored locally in `./uploads` as temporary working files.
- Path sanitization is applied to uploaded file names before temporary storage.

## Out of Scope

The following are not considered security issues by themselves:

- performance bottlenecks without confidentiality/integrity impact
- missing production hardening for internet-facing deployment
- risks caused by running TurboDrop on an untrusted public network against documented guidance
