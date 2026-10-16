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

Persistence tests use isolated temporary SQLite files with the production
one-connection policy. Run them in the default suite without a database service,
Docker socket, PostgreSQL environment variables or integration tags. The Francis
provider suite also runs by default. Root checks keep Docker unavailable inside
test processes; the separate image-smoke CI job uses Docker only for packaging.

For the 15-minute service-layer acceptance workload:

```sh
CGO_ENABLED=0 GOFLAGS=-p=4 go -C backend test ./internal/schema \
  -run '^$' -bench '^BenchmarkSQLiteConcurrentLoad$' -benchtime=1x -timeout=20m
```

The benchmark reports service-call p95/p99 latency, contention, WAL peaks and exact
committed row/message counts. See the [SQLite contract](docs/design/sqlite-database.md)
and [candidate packaging](deploy/hub/README.md). `pnpm run build` compiles hub,
agent, CLI and web; the Docker build embeds the production web output into the hub.

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
