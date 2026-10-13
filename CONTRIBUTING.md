# Contributing

Use Node 24 and pnpm 11.21.0 (pinned in package.json). M0 has no dependencies;
no install is needed to run its checks. Go requirements will come from go.mod
when the core arrives in M1. The web package follows in M2.

```sh
node scripts/setup-git-hooks.mjs
git add .
pnpm run lint
pnpm run test
pnpm run build
```

The setup sets core.hooksPath to .githooks and fails on a conflicting local
setting. Source archives without Git may skip hook setup, but verification runs
in a checkout. build currently verifies the foundation; it produces no product.
Root commands have module integration points for future Go and web commands.

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
