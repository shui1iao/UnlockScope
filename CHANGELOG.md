# Changelog

## [v0.1.2] - 2026-08-21

- Add a runtime `country` field sourced from the checking node's egress lookup.
- Keep region-group selection separate from ISO country codes.
- Stop treating arbitrary provider-page locale metadata as a detected region.
- Preserve unknown when the runtime country cannot be determined.

## [v0.1.1] - 2026-08-19

- Grouped terminal results under English category headings while preserving first-seen category order.
- Rendered human-readable states in Chinese with uppercase region codes inside full-width parentheses; unavailable results no longer show a region.
- Kept duration and probe notes in the grouped terminal output and added regression coverage for the new format.

## [v0.1.0] - 2026-08-16

- Initial public release of the standalone `unlockscope` Go CLI.
- Added 161 declarative providers across streaming, AI, social/knowledge, sports, games, and HK/TW/JP/KR/NA/EU/SA/AF/Oceania regional groups.
- Added bounded concurrent probing, per-provider and global timeouts, IPv4/IPv6 and source interface/address selection, HTTP/SOCKS5 proxies, automatic egress-region selection, JSON/plain output, and provider listing.
- Added release installer with SHA-256 verification, multi-platform build/release workflows, tests, and development documentation.

Platform interfaces and public endpoints can change at any time; treat this release as a heuristic detector rather than an availability guarantee.
