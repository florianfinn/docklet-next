// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package bindpath

const FileWriteScript = `set -e
live=$1
staged=$2
size=$3
backup=/tmp/arcane-workspace/live-backup
rm -f -- /tmp/arcane-workspace/recovery-failed
test "$(stat -c %s "$staged")" = "$size"
old=$(stat -c %s "$live")
head -c "$old" "$live" > "$backup"
test "$(stat -c %s "$backup")" = "$old"
if head -c "$size" "$staged" > "$live" && test "$(stat -c %s "$live")" = "$size"; then exit 0; fi
if head -c "$old" "$backup" > "$live" && test "$(stat -c %s "$live")" = "$old"; then exit 1; fi
printf failed > /tmp/arcane-workspace/recovery-failed
exit 2`

const RestoreFileScript = `set -e
size=$(stat -c %s "$1")
head -c "$size" "$1" > "$2"
test "$(stat -c %s "$2")" = "$size"`
