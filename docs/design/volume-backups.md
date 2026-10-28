<!--
Source: docklet-next/issues/283
Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
Retrieved: 2026-10-28
License: Apache-2.0
Deviations: Original lifecycle, target and portable recovery rationale.
-->

# Volume backups and recovery

Named Docker volumes use encrypted Rustic snapshots. Bind mounts are outside this
backup contract. Each host runs its own schedules in its configured time zone,
including while the hub is disconnected. A volume has at most one policy; six-field
cron is retained for custom schedules. Retention counts scheduled snapshots per
volume across policy and target changes. Zero keeps all scheduled snapshots;
manual snapshots remain until deleted. Safety snapshots retain the newest two,
plus any snapshot pinned by an outstanding failed restore.

## Repository identity and keys

The built-in `local` destination preserves the existing `/backups` mount or
`ARCANE_BACKUP_VOLUME_NAME` fallback. Its repository password is a stored recovery
key when present, otherwise the existing derivation from the host encryption key.
A derived password changes with that encryption key; a stored password stays the
same, but decrypting its database record requires the original encryption key.
Attaching an exported local key verifies the repository before storing the key;
it does not change the repository password.

Stored targets support host-local directories, existing host mounts, SMB and NFS.
Local and mount targets belong to one host; SMB and NFS targets are shared target
choices. A target ID permanently identifies one location. Moving a repository
requires a new target and a policy change. Name, credentials and mount options may
change without redirecting old snapshots. Historical consumers resolve the
backup row's destination, including retired targets, rather than the current plan.
Safety snapshots always use the owning host's built-in local destination.

Stored target repositories use `docklet-backups/<hub host ID>/volumes` below the
target root. Each has its own generated recovery key, using eight groups of six
base32 characters. An existing key can attach an existing repository. An initialized
repository that refuses a key is never initialized again or automatically re-keyed.
Export recovery keys separately from the databases and repository copies.

Credentials and stored keys are encrypted independently in each host database.
Ordinary DTOs, activities and events exclude them. Key view/export routes require
restore permission, disable caching and record a key-free audit event. Docker
operators can inspect SMB credentials in local-driver volume options; this is an
accepted operational boundary. Docklet hides target mount volumes from its generic
volume routes and redacts their `o` option as an additional serialization guard.
SMB username, password and domain values containing comma or equals are refused;
mount options use per-kind allowlists. NFS defaults to `hard` mounts.

## Distribution and admission

The hub owns a target authority UUID and monotonically increasing set version.
Host-filtered sync envelopes include the recipient's hub host ID and a hash of the
canonical non-secret content, including configuration versions. Agents apply the
set transactionally, re-encrypt secrets and retire absent definitions while
retaining their keys. Same-authority lower versions and equal versions with a
different hash are refused. A new authority or recipient host ID establishes a new
set, while immutable-location checks continue to protect previously held IDs.
The pairing credential is checked again inside the commit transaction.

Target retirement holds the hub's per-target write fence through authoritative
reference checks, commit and delivery. Policy changes and explicit manual target
admission hold the matching read fence through their response. All relevant hosts
must answer online with no policy, backup or running references before retirement.
A hub database restore regenerates only the authority before activation and keeps
the restored set counter, so target and check configuration versions remain ordered.
Newer agent
copies absent from the restored hub set become retired, with keys retained.

Volume operations acquire durable engine admission before the shared workspace
mutex, using non-waiting acquisition. Safety capture and retention inherit the
caller's ownership. Busy responses identify the running activity when that
information is available. Restore-as-new also owns its destination name before
persisting the creation intent; creation, rename and prune share that fence.

## Restore responsibility

Restore admission returns an activity and restore ID. Its durable phases are
`preparing`, `writing` and `done`. A replace restore records its own consumer IDs
before stopping them, takes a safety snapshot, writes the selected data, and
restarts the still-matching IDs. Every outstanding pending restore of the volume
contributes inherited restart responsibility. A failed preparing recovery restarts
only consumers it stopped itself; earlier failed-write responsibilities stay in
their original rows until a later restore succeeds.

A failure after writing starts leaves consumers stopped and retains the safety pin.
Restart reconciles interrupted work without replaying a data write. Unconfirmed
old engine or helper removal fences the entire volume, including settlement and
consumer restarts, until a retry confirms the writer is gone. Restored hub database
rows describe historical Docker state: they never authorize container restarts or
new-volume cleanup, and all restored pending responsibilities are cleared.

Restore-as-new persists its intent before Docker creation and verifies the unique
restore label afterward. Cleanup removes only a volume still carrying that run's
label and only after its writer is confirmed gone. It never deletes a foreign
same-name volume. Failed cleanup remains a durable retry obligation.

## Portable uploads and runtime tools

Compressed sessions are limited to 4 GiB. Admission transfers the consumed file to
an operation-owned input directory, outside upload-session expiry. A complete Go
validation pass checks gzip/tar integrity, relative paths, duplicate normalized
paths, allowed member types, link relationships and at most 200,000 entries.
Symlinks remain data; later entries below a symlink are refused. Hardlinks may
refer only to earlier regular members. The sum of regular-file sizes is capped at
16 GiB (`expanded-too-large`); the serving process does not infer daemon-host free
space. Scratch ENOSPC is `insufficient-space` during preparation, before target
writes. Consumers remain untouched or are restarted after a preparing abort.

A second pass verifies the input hash while re-encoding only validated headers to
an uncompressed tar stream sent through Docker's archive API into a separately
labelled scratch volume. Container-local input paths are never daemon bind sources.
The existing pinned helper has no `cp`, `df` or `find -exec`: copying uses checked
staging through a temporary tar, and clearing uses shell globs for ordinary and
hidden names. Capacity fields are optional and omitted when unavailable.

The Rustic image is a scratch image without a shell or `stat`. For mount targets,
a tools helper runs `stat -c %d /target /parent` immediately before each Rustic
engine container, using the same private host-bind plan. Equal devices refuse the
operation before repository effects. Same-filesystem bind mounts are unsupported.
There is an accepted window between the helper and engine creates: the host mount
can change in that interval, because Docker resolves each bind at its own create.
A historical passing target test does not replace this per-operation witness.

## Transport and recovery boundaries

Read operations have a 60-second deadline and an 8 MiB JSON/output limit. Browse
is non-recursive and refuses more than 5,000 entries. Edge retains its buffered
five-minute transport: downloads require a known snapshot size no greater than
512 MiB, and the packaged archive is checked again against that cap. Preparation
has a three-minute Edge budget; cancellation retains cleanup ownership. Hub-local
and Direct downloads stream after scratch preparation without the Edge byte cap.

Hub-local workers register with the maintenance drain and settle their durable
state before database replacement can proceed. Hub database archives are
unencrypted. Encrypted target credentials and keys inside them require the
original encryption key; exported recovery keys do not encrypt the archive or
replace that encryption key. Snapshot data without metadata rows is not listed,
and restored metadata for externally forgotten snapshots is unavailable.

The runtime engine is the upstream Rustic v0.11.2 OCI index pinned by digest;
`DOCKLET_BACKUP_ENGINE_IMAGE` may override the complete reference. The index digest
was resolved on 2026-10-28 using the registry manifest response and independently
checked against the SHA-256 of the response bytes. Rustic is
[Apache-2.0 OR MIT](https://github.com/rustic-rs/rustic/blob/v0.11.2/Cargo.toml).
The upstream image is executed as supplied and is not republished by Docklet.

The engine reference is overridden only through `DOCKLET_BACKUP_ENGINE_IMAGE`
in this host process's configuration. It is reported read-only by repository
metadata. Writable settings do not select an engine image; adding that path
would require a separate product decision. This keeps engine selection under
the operator's deployment configuration and can be reversed independently of
repository data or the agent protocol.

The external-image licence and digest resolution record is
[`docs/provenance/runtime-images.json`](../provenance/runtime-images.json).
