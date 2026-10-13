# Contributing

Use Node 24, pnpm 11.21.0 (pinned in package.json), and the Go version in
backend/go.mod. The default test suite needs no database, Docker socket or
running agent.

```sh
node scripts/setup-git-hooks.mjs
git add .
pnpm run lint
pnpm run test
pnpm run build
```

The setup sets core.hooksPath to .githooks and fails on a conflicting local
setting. Source archives without Git may skip hook setup, but verification runs
in a checkout. The build compiles Go commands and the web bundle. The root
commands run Go and web checks.

PostgreSQL integration tests need a dedicated PostgreSQL 18 test service. Use
its test-only connection URL as `DATABASE_URL`, `DOCKLET_TEST_POSTGRES_DSN`
and `ARCANE_TEST_POSTGRES_DSN`; the tests create and drop
temporary databases and may modify the configured test database. If Go runs
inside a toolchain container, forward `DATABASE_URL` into that container.
Use a fresh configured test database for each full tagged run because durable
actor tests persist state there.
Run the tagged suite serially relative to other test suites and cap Go package
parallelism on constrained machines:

```sh
cd backend
GOFLAGS=-p=2 \
  DATABASE_URL='postgres://docklet:test-only@127.0.0.1:5432/docklet?sslmode=disable' \
  DOCKLET_TEST_POSTGRES_DSN='postgres://docklet:test-only@127.0.0.1:5432/docklet?sslmode=disable' \
  ARCANE_TEST_POSTGRES_DSN='postgres://docklet:test-only@127.0.0.1:5432/docklet?sslmode=disable' \
  go test -tags=postgres_integration ./...
```

CI runs this suite in a separate required gate against a pinned PostgreSQL 18
service. The [baseline design](docs/design/database-baseline.md) describes the
fresh database and model comparison tests.

Read [AGENTS.md](AGENTS.md) and [review-workflow.md](docs/design/review-workflow.md)
before work. Follow module ownership and the ready gate. Keep changes local until
push/PR instructions are explicit. Use English Conventional Commits and public
human noreply identities; include the issue number when one exists. Review is
independent and comes from a different provider at milestone completion or a risk gate.

Before adopting code, preserve its notices and record source path, full revision
or content hash, retrieval date, licence and deviations. Register adopted paths,
exact oversized-file baselines, dependency inventory and binary asset hashes.
See [provenance](docs/design/provenance.md) and [licensing](docs/design/licensing.md).
Use only free shadcn/ui source. Check publication suitability of prose and images.

The optional .publication-denylist.local.json is an untracked JSON array of
operator-specific strings; case-insensitive literal matches fail publication.
Do not publish its contents or add it to Git. See the
[publication policy](docs/design/publication-policy.md) for limitations.

Report security issues privately under [SECURITY.md](SECURITY.md), not in public
issues. Feature and decision requests belong in the owning milestone; small
maintenance changes need only a PR.
