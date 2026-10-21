# Stack authoring and deployment

## Boundary

This record covers M7: creating a Compose stack from the web editor, saving
`compose.yaml`, `.env` and labels of an existing stack, deploying through the
hub, and converting `docker run` commands on the hub. It reuses the existing
project routes and the [runtime control](runtime-control.md) admission without
adding routes, migrations or wire vocabulary. Multi-file workspace editing
belongs to M8 Storage and file workspace. Templates, Git repositories and GitOps
stay removed (`api/removed-operations.json`). The GitHub/Docker Hub one-time
import remains a demo function and is not offered by the production adapter.

## Operation mapping

| UI operation | Capability ref | Hub requests | Required rights |
| --- | --- | --- | --- |
| `deployStack`, new stack | `host:<env>` | `create-project` (multipart `project` JSON with name, compose, env, tags and empty `manifest`), then guarded `up` | `projects:create`, `projects:deploy`, `activities:read` |
| `deployStack`, existing stack | `stack:<env>~<project>` | `update-project` (compose, env), label diff via `update-project-tag`, then guarded `up` | `projects:update`, `projects:deploy`, `activities:read` |
| `saveStackFiles` | `stack:<env>~<project>` | `update-project` | `projects:update` |
| `setStackLabels` | `stack:<env>~<project>` | one `update-project-tag` per added or removed normalized label | `projects:update` |
| `convertDockerRun` | `host:<env>` | `convert-docker-run` | `containers:create` |

The hub capability descriptor states availability per resource ref from these
rights; the browser check is advisory and every route still authorizes itself.
Deploy is `up`, so it inherits the M6 opaque idempotency key, single admission
retry, busy/cancelling conflicts and unknown-admission handling. The browser run
gate treats a production deploy like other durable runtime commands.

## Sequencing and partial state

- Writes precede deploy. A failed save or label change stops before `up`; the
  draft stays in the editor and the stack list is invalidated.
- `create-project` is not idempotent. It is sent once. A lost or unreadable
  response is reported as `stack.create-unknown` without retry, because a
  second create could duplicate the project or collide by name. The operator
  reloads the stack list before trying again.
- When create succeeds but the first `up` is not admitted, the adapter raises
  `StackCreatedError` with the new stack ID. The editor opens that stack instead
  of keeping a new-stack draft, so a retry deploys the existing project.
- The host of an existing stack is immutable; a draft naming another host is
  refused before any request (`stack.host-immutable`).
- Labels are diffed against the current server tags after normalization
  (trimmed, lowercase, de-duplicated). Compose-sourced tags are read-only on the
  hub and surface as a refused change.

## docker run conversion

Production converts on the hub for the draft host after an explicit request; it
does not send every keystroke. A 400 response means the hub could not parse the
command; other failures are shown as conversion failures. The dialog takes the
named service from the returned Compose document and appends returned
environment lines to the draft `.env`. The hub reports no per-flag notes, so the
production dialog shows none; the demo keeps the local converter and its notes.

## Acceptance focus

Adapter tests cover request shape and order, unknown create without retry,
created-but-not-deployed, host mismatch, 401 and capability refusal. Practical
acceptance creates, deploys, edits, relabels and redeploys a stack, converts a
`docker run` command, and checks invalid Compose, failed deploy and busy-run
outcomes on sandbox hosts.
