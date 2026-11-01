//go:build toolsintegration

// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"bytes"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/stretchr/testify/require"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"uuid"
)

// toolsintegration is opt-in: the acceptance runner owns the sandbox daemon.
func TestPinnedToolsBindCommands(t *testing.T) {
	image := volumehelper.DefaultToolsImage
	name := "docklet-bind-tools-" + uuid.New().String()
	run := func(stdin []byte, args ...string) []byte {
		t.Helper()
		command := exec.CommandContext(t.Context(), "docker", args...)
		command.Stdin = bytes.NewReader(stdin)
		out, err := command.CombinedOutput()
		require.NoError(t, err, string(out))
		return out
	}
	run(nil, "volume", "create", "--label", "t3.project=docklet-next-m16-tools", name)
	t.Cleanup(func() { _ = exec.Command("docker", "volume", "rm", name).Run() })
	base := []string{"run", "--rm", "--network", "none", "--label", "t3.project=docklet-next-m16-tools", "--entrypoint", "sh"}
	create := append(append([]string{}, base...), "--mount", "type=volume,src="+name+",dst=/fixture", image, "-c", `set -e; printf original > /fixture/app.conf; mkdir /fixture/tree; printf abc > /fixture/tree/one; printf abcd > /fixture/tree/two`)
	run(nil, create...)
	source := strings.TrimSpace(string(run(nil, "volume", "inspect", "--format", "{{.Mountpoint}}", name)))
	for _, content := range [][]byte{[]byte("replacement"), {}, {0, 255, 1, 0}} {
		script := `set -e
mkdir -p /tmp/arcane-workspace
head -c "$3" > "$2"
before=$(stat -c '%i %u %g %a' "$1")
(` + bindpath.FileWriteScript + `)
after=$(stat -c '%i %u %g %a' "$1")
test "$before" = "$after"
head -c "$3" "$1"`
		args := append(append([]string{}, base...), "-i", "--mount", "type=bind,src="+source+"/app.conf,dst=/volume/app.conf", image, "-c", script, "sh", "/volume/app.conf", "/tmp/arcane-workspace/change-0", strconv.Itoa(len(content)))
		require.Equal(t, content, run(content, args...))
	}
	restoreScript := `set -e
mkdir /scratch
printf restored > /scratch/app.conf
before=$(stat -c '%i %u %g %a' "$2")
(` + bindpath.RestoreFileScript + `)
after=$(stat -c '%i %u %g %a' "$2")
test "$before" = "$after"
head -c 8 "$2"`
	restoreArgs := append(append([]string{}, base...), "--mount", "type=bind,src="+source+"/app.conf,dst=/volume/app.conf", image, "-c", restoreScript, "sh", "/scratch/app.conf", "/volume/app.conf")
	require.Equal(t, []byte("restored"), run(nil, restoreArgs...))
	args := append(append([]string{}, base...), "--mount", "type=bind,src="+source+"/tree,dst=/source,readonly", image, "-c", `set -e; find /source; stat -c '%d %i %h %s %F' /source /source/one /source/two`)
	output := string(run(nil, args...))
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.GreaterOrEqual(t, len(lines), 6)
	fields := strings.Fields(lines[len(lines)-3])
	sum := bindSizeSum{device: fields[0], seen: map[string]bool{}}
	for _, line := range lines[len(lines)-3:] {
		sum.add(line)
	}
	require.EqualValues(t, 7, sum.size)
	require.False(t, sum.partial)
}
