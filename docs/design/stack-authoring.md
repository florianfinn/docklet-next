# Stack authoring and deployment

## Boundary

This record covers M7: creating a Compose stack from the web editor, saving
`compose.yaml`, `.env` and labels of an existing stack, deploying through the
hub, and converting `docker run` commands on the hub. It reuses the existing
project routes and the [runtime control](runtime-control.md) admission without
adding routes, migrations or wire vocabulary. Multi-file workspace editing
belongs to M8 Storage and file workspace. Templates, Git repositories and GitOps
stay removed (`api/removed-operations.json`). The GitHub/Docker Hub one-time
import remains a demo function and is not offered by the production adapter
until M19 ([core adoption ledger](core-adoption.md#decision-ledger)).

## Operation mapping

| UI operation | Capability ref | Hub requests | Required rights |
| --- | --- | --- | --- |
| `deployStack`, entry point | `hub:0` | none; enables the New stack action | all new-stack rights below together in one environment (global grants count for each) |
| `deployStack`, new stack | `host:<env>` | `create-project` (multipart `project` JSON with name, compose, env, tags and empty `manifest`), then guarded `up` | `projects:create` plus the runtime command rights |
| `deployStack`, existing stack | `stack:<env>~<project>` | `update-project` (compose, env), label diff via `update-project-tag`, then guarded `up` | `projects:update` plus the runtime command rights |
| `saveStackFiles` | `stack:<env>~<project>` | `update-project` | `projects:update` |
| `setStackLabels` | `stack:<env>~<project>` | one `update-project-tag` per added or removed normalized label | `projects:update` |
| `convertDockerRun` | `host:<env>` | `convert-docker-run` | `containers:create` |

The runtime command rights are those of the M6 `runStackCommand` descriptor
(`activities:read`, `projects:deploy`, `projects:down`, `projects:restart`),
because the deploy is admitted through that guarded command after the writes.
Advertising deploy with fewer rights would write files and then refuse `up`.
Narrower deploy-only scopes belong to the later user-policy milestone.

The hub capability descriptor states availability per resource ref from these
rights. Editor controls read the descriptor for the current host or stack and
stay disabled with a localized reason; every route still authorizes itself.
Deploy is `up`, so it inherits the M6 opaque idempotency key, single admission
retry, busy/cancelling conflicts and unknown-admission handling. The browser run
gate treats a production deploy like other durable runtime commands.

## Reading an existing stack

Plain project details (`get-project`) carry the inventory observation and the
hub's retained snapshot, but no file content. The editor reads `compose.yaml`
and `.env` separately through `get-project-compose`, which is not cached.

- Until the files have been read once, the editor tab shows a notice with a
  retry instead of an editor.
- The draft only starts from files that were actually read. A later read
  failure keeps the last loaded files as its base and keeps local edits; the
  editor shows a notice, and Save, keyboard save and Deploy stay disabled until
  the files load again.

## Sequencing and partial state

- Writes precede deploy. A failed save or label change stops before `up`; the
  draft stays in the editor and the stack list is invalidated.
- `create-project` is not idempotent. It is sent once. A lost or unreadable
  response is reported as `stack.create-unknown` without retry, because a
  second create could duplicate the project or collide by name. The operator
  reloads the stack list before trying again.
- When create succeeds but the first `up` is not admitted, the adapter raises
  `StackCreatedError` with the new stack ID. The editor opens that stack instead
  of keeping a new-stack draft, so a retry deploys the existing project. A
  confirmed refusal says the deploy did not start; an unknown admission keeps
  the M6 uncertainty and asks the operator to check Activity before retrying.
- Save also writes labels when they differ, so it needs the label descriptor
  in that case.
- Navigation after a deploy belongs to the editor that started it. When the
  operator has left that editor, a late result only reports through its toast
  and Activity; it never moves away from or discards a later draft. The hub work
  itself is not cancelled.
- Keyboard save follows the same descriptor gate as the Save button.
- The host of an existing stack is immutable; a draft naming another host is
  refused before any request (`stack.host-immutable`).
- Labels are diffed against the current server tags after normalization
  (trimmed, lowercase, de-duplicated). Compose-sourced tags are read-only on the
  hub and surface as a refused change. Browser normalization follows the hub’s
  whitespace trimming and Unicode simple lowercase, including dotted capital I
  and non-contextual Greek sigma.

## docker run conversion

Production converts on the hub for the draft host after an explicit request; it
does not send every keystroke. A conversion lives only while the dialog stays
open on the same host and command: closing, editing the command, changing the
host or replacing the adapter ends it, and only the latest request can settle.
Nothing can be added while a conversion runs. A 400 response means the hub could not parse the command;
other failures are shown as conversion failures. The dialog imports the named
service together with the returned top-level `networks` and `volumes` in one
edit. Structurally equal existing declarations are kept regardless of key
order; a differing one refuses the whole edit. Documents with other top-level sections are refused. Returned environment
lines are appended to the draft `.env`. The hub reports no per-flag notes, so the
production dialog shows none; the demo keeps the local converter and its notes.

## Acceptance focus

Adapter tests cover request shape and order, unknown create without retry,
created-but-not-deployed, host mismatch, 401 and capability refusal. Practical
acceptance creates, deploys, edits, relabels and redeploys a stack, converts a
`docker run` command, and checks invalid Compose, failed deploy and busy-run
outcomes on sandbox hosts.
