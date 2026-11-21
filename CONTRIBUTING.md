# Contributing

- Demos (`demos/<name>/`) are thin: UI, glue and mocks only. Reusable logic lives in the private modules repo.
- Demos must run without real keys (mock / sandbox providers).
- Before adding or changing a workflow read [SECURITY.md](SECURITY.md) — CI here runs tests of private code.
- Pull requests from forks run no CI with secrets.
