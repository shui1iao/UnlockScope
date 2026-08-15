# Security Policy

## Supported versions

Only the latest tagged release is expected to receive security fixes. The project is currently at `v0.1.0`.

## Reporting a vulnerability

Do not open a public issue for a vulnerability. Use a private GitHub security advisory for `shui1iao/UnlockScope` when available, or contact the repository maintainer through a private channel and include:

- the affected version and platform;
- a minimal reproduction or clear steps;
- impact and any required permissions;
- a proposed mitigation, if known.

Please do not include credentials, cookies, private proxy URLs, personal IP addresses, or full provider response bodies. Redact them before sharing logs.

## Security boundaries

UnlockScope is intentionally credential-free. It performs bounded public GET probes, does not execute downloaded release content before checksum verification, and does not write to provider accounts. Providers may still log network metadata. Run probes only against endpoints and proxies you are authorized to use, and review terms of service and local law.
