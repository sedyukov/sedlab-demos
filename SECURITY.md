# Security & CI hygiene

This repository is **public**. Its CI also tests **private** code from `sedyukov/sedlab-modules`
(checked out with a read-only deploy key). These rules keep private code and secrets out of public logs and storage.

## Rules for workflows

1. **Triggers:** only `push` to `main` and `workflow_dispatch`. Never `pull_request_target`; never run secret-using jobs on PRs.
   GitHub does not expose secrets to workflows from forks — keep it that way.
2. **Access to private code:** only via the `MODULES_DEPLOY_KEY` secret (read-only deploy key scoped to one repo),
   `persist-credentials: false`.
3. **Quiet output:** `go test` without `-v`, `pytest -q --tb=no`, TypeScript via `npm run test:ci` with all output suppressed (vitest/tsc show code frames) — only pass/fail counts are printed; no `set -x`, no `cat`/`tree`/`ls -R` of the private checkout,
   no printing of env.
4. **No persistence of private code:** no `actions/cache`, no `upload-artifact`, `setup-go` with `cache: false`,
   `setup-python` without `cache`.
5. **Images:** images that contain private module code are pushed to GHCR only as **private** packages.
   Deploys are not done from CI — they run from the owner's machine (`infra/deploy.sh`).
6. **Permissions:** `permissions: contents: read` by default; widen per job only when needed.
7. **Secrets** never in the repo. A gitleaks scan runs on every push (`secret-scan.yml`).

## Reporting

Found something sensitive here? Open an issue without details or contact the owner via the site; it will be rotated and removed.
