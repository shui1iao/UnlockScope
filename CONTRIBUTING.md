# Contributing

Thanks for helping improve UnlockScope.

## Before opening a change

1. Keep provider logic credential-free and read-only.
2. Put service URLs, group membership, and signal words in `internal/provider/provider.go`; use the probe package for shared network mechanics.
3. Do not copy source code from other region-checking tools. Public endpoint behavior and category ideas are fine; implementation must remain original.
4. Do not commit secrets, cookies, personal IP logs, or full captured provider pages.
5. Explain why a provider URL and heuristic are maintainable, and document ambiguity as `unknown` rather than guessing.

## Local checks

```bash
make fmt
make test
make race
make vet
make lint
make build
bash -n install.sh
```

Provider additions should include registry/region coverage assertions and mock HTTP tests for status, redirect, timeout, and body/JSON signals where applicable. Keep tests deterministic and avoid live provider requests.

## Pull requests

Describe the behavior change, affected platforms, and test commands. Changes to the stable JSON fields or state semantics need an explicit compatibility note in `CHANGELOG.md` and both READMEs. CI must pass before merge.
