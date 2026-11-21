// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Recorded input attribution, confinement and replacement regressions; M34 #548.

package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/compose-spec/compose-go/v2/loader"
	composetypes "github.com/compose-spec/compose-go/v2/types"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func recordedFixture(t *testing.T) (string, string, string, map[string]string) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "stack")
	require.NoError(t, os.Mkdir(dir, 0700))
	compose := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(compose, []byte("services:\n  app:\n    image: alpine:3.22\n    environment:\n      VALUE: ${VALUE:?required}\n    command: ['echo', '${VALUE}']\n    labels:\n      example.value: ${VALUE}\n"), 0600))
	env := filepath.Join(root, "runtime.env")
	require.NoError(t, os.WriteFile(env, []byte("VALUE=recorded-sentinel\n"), 0600))
	labels := map[string]string{"com.docker.compose.project": "same-name", "com.docker.compose.project.working_dir": dir, "com.docker.compose.project.config_files": compose, EnvironmentFileLabel: env}
	return root, dir, env, labels
}
func recordedContext(t *testing.T, labels ...map[string]string) context.Context {
	t.Helper()
	return WithComposeContainerLabels(t.Context(), func(context.Context) ([]map[string]string, error) { return labels, nil })
}
func TestRecordedInputSafeRead(t *testing.T) {
	for _, kind := range []string{"relative", "unclean", "prefix", "escape", "directory", "fifo", "size", "missing", "loop", "permission"} {
		t.Run(kind, func(t *testing.T) {
			root, dir, env, label := recordedFixture(t)
			switch kind {
			case "relative":
				label[EnvironmentFileLabel] = "runtime.env"
			case "unclean":
				label[EnvironmentFileLabel] = root + "/stack/../runtime.env"
			case "prefix":
				label[EnvironmentFileLabel] = root + "-other/runtime.env"
			case "escape":
				outside := filepath.Join(t.TempDir(), "outside.env")
				require.NoError(t, os.WriteFile(outside, []byte("VALUE=outside-sentinel\n"), 0600))
				require.NoError(t, os.Remove(env))
				require.NoError(t, os.Symlink(outside, env))
			case "directory":
				label[EnvironmentFileLabel] = dir
			case "fifo":
				require.NoError(t, os.Remove(env))
				require.NoError(t, syscall.Mkfifo(env, 0600))
			case "size":
				require.NoError(t, os.WriteFile(env, []byte("VALUE="+strings.Repeat("a", maxRecordedEnvBytes)), 0600))
			case "missing":
				require.NoError(t, os.Remove(env))
			case "loop":
				require.NoError(t, os.Remove(env))
				require.NoError(t, os.Symlink(env, env))
			case "permission":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses discretionary read permissions")
				}
				require.NoError(t, os.Chmod(env, 0))
			}
			input, err := ResolveComposeInputs(recordedContext(t, label), root, dir, false)
			require.Error(t, err)
			require.Equal(t, "invalid", input.State)
			require.Equal(t, "compose.env-label-invalid", ClassifyLoadError(err).Reason)
			require.NotContains(t, err.Error(), "outside-sentinel")
		})
	}
}
func TestRecordedInputSymlinkedProjectsRoot(t *testing.T) {
	root, _, _, _ := recordedFixture(t)
	link := filepath.Join(t.TempDir(), "projects")
	require.NoError(t, os.Symlink(root, link))
	for name, base := range map[string]string{"configured": link, "resolved": root} {
		t.Run(name, func(t *testing.T) {
			label := map[string]string{"com.docker.compose.project.working_dir": filepath.Join(base, "stack"), "com.docker.compose.project.config_files": filepath.Join(root, "stack", "compose.yaml"), EnvironmentFileLabel: filepath.Join(base, "runtime.env")}
			input, err := ResolveComposeInputs(recordedContext(t, label), link, filepath.Join(link, "stack"), false)
			require.NoError(t, err)
			require.Equal(t, "recorded", input.State)
			require.Equal(t, "recorded-sentinel", input.Environment["VALUE"])
		})
	}
}
func TestRecordedInputDescriptorBytesAndFlags(t *testing.T) {
	root, dir, env, label := recordedFixture(t)
	inside := filepath.Join(root, "inside")
	outside := t.TempDir()
	require.NoError(t, os.Mkdir(inside, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(inside, "vars.env"), []byte("VALUE=inside\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "vars.env"), []byte("VALUE=outside\n"), 0600))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink("inside", link))
	env = filepath.Join(link, "vars.env")
	label[EnvironmentFileLabel] = env
	originalOpen := openRecordedFile
	t.Cleanup(func() { openRecordedFile = originalOpen })
	reads := 0
	openRecordedFile = func(root *os.Root, relative string, flags int) (*os.File, error) {
		file, err := originalOpen(root, relative, flags)
		require.NoError(t, err)
		reads++
		actualFlags, err := unix.FcntlInt(file.Fd(), unix.F_GETFL, 0)
		require.NoError(t, err)
		require.NotZero(t, actualFlags&syscall.O_NONBLOCK, "opened descriptor must be nonblocking, even for regular files")
		require.NoError(t, os.Remove(link))
		require.NoError(t, os.Symlink(outside, link))
		return file, nil
	}
	input, err := ResolveComposeInputs(recordedContext(t, label), root, dir, true)
	require.NoError(t, err)
	require.Equal(t, 1, reads)
	require.Equal(t, "inside", input.Environment["VALUE"])
	require.Equal(t, "VALUE=inside\n", string(input.Files[0].Bytes))
	require.Equal(t, input.Environment["VALUE"], input.Injection["VALUE"])
}
func TestRecordedAttributionAndReplacement(t *testing.T) {
	root, dir, env, label := recordedFixture(t)
	custom := filepath.Join(dir, "custom.yaml")
	require.NoError(t, os.Rename(filepath.Join(dir, "compose.yaml"), custom))
	override := filepath.Join(dir, "second.yaml")
	require.NoError(t, os.WriteFile(override, []byte("services:\n  app:\n    labels:\n      example.order: second\n"), 0600))
	label["com.docker.compose.project.config_files"] = custom + "," + override
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("MALFORMED='\n"), 0600))
	require.NoError(t, os.WriteFile(env, []byte("VALUE=first\nCOMPOSE_FILE=custom.yaml:second.yaml\nCOMPOSE_PROFILES=debug\nCOMPOSE_ENV_FILES=missing.env\nCOMPOSE_PROJECT_NAME=from-recorded\n"), 0600))
	last := filepath.Join(root, "last.env")
	require.NoError(t, os.WriteFile(last, []byte("VALUE=last\nINJECT_ONLY=ordered-injection\n"), 0600))
	label[EnvironmentFileLabel] = env + "," + last
	other := map[string]string{"com.docker.compose.project": "same-name", "com.docker.compose.project.working_dir": root, "com.docker.compose.project.config_files": custom, EnvironmentFileLabel: "/srv/example/unattributed.env"}
	oneoff := map[string]string{}
	for k, v := range label {
		oneoff[k] = v
	}
	oneoff["com.docker.compose.oneoff"] = "True"
	oneoff[EnvironmentFileLabel] = "/srv/example/oneoff.env"
	lists := 0
	ctx := WithComposeContainerLabels(t.Context(), func(context.Context) ([]map[string]string, error) {
		lists++
		return []map[string]string{other, oneoff, label}, nil
	})
	for _, inject := range []bool{false, true} {
		input, err := ResolveComposeInputs(ctx, root, dir, inject)
		require.NoError(t, err)
		pinned := WithResolvedInputs(ctx, input)
		selected, err := DetectComposeFile(pinned, root, dir)
		require.NoError(t, err)
		require.Equal(t, custom, selected)
		model, err := LoadComposeProject(pinned, selected, "", root, inject, nil, nil, nil, false, nil, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "from-recorded", model.Name)
		require.Equal(t, []string{custom, override}, model.ComposeFiles)
		require.Equal(t, "last", *model.Services["app"].Environment["VALUE"])
		if inject {
			require.Equal(t, "ordered-injection", *model.Services["app"].Environment["INJECT_ONLY"])
		} else {
			require.NotContains(t, model.Services["app"].Environment, "INJECT_ONLY")
		}
		require.Equal(t, "second", model.Services["app"].Labels["example.order"])
		for _, service := range model.Services {
			require.Equal(t, label[EnvironmentFileLabel], service.CustomLabels[EnvironmentFileLabel])
		}
		require.NotContains(t, model.Environment, "COMPOSE_ENV_FILES")
		metadata, err := ParseArcaneComposeMetadata(pinned, selected, root, inject)
		require.NoError(t, err)
		require.Contains(t, metadata.ComposeFiles, custom)
		_, _, err = LoadComposeIdentity(pinned, selected, "", root, inject)
		require.NoError(t, err)
		_, err = BuildValidationEnvironment(pinned, root, dir, nil)
		require.NoError(t, err)
		projection, err := ProjectServiceProjection(pinned, model, root, nil)
		require.NoError(t, err)
		body, err := json.Marshal(projection.Services)
		require.NoError(t, err)
		require.NotContains(t, string(body), "last")
		require.Contains(t, string(body), "${VALUE}")
	}
	input, err := ResolveComposeInputs(ctx, root, dir, false)
	require.NoError(t, err)
	content, err := os.ReadFile(custom)
	require.NoError(t, err)
	require.NoError(t, ValidateComposeContentForUpdate(WithResolvedInputs(ctx, input), root, dir, "stack", string(content), nil, nil, "", false))
	preview := filepath.Join(root, "preview")
	require.NoError(t, os.Mkdir(preview, 0700))
	for _, file := range []string{custom, override} {
		body, e := os.ReadFile(file)
		require.NoError(t, e)
		require.NoError(t, os.WriteFile(filepath.Join(preview, filepath.Base(file)), body, 0600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(preview, ".env"), []byte("BROKEN='\n"), 0600))
	previewModel, _, err := LoadComposeProjectFromDir(RebaseComposeInputs(ctx, input, preview), preview, "stack", root, false, nil)
	require.NoError(t, err)
	require.Equal(t, "last", *previewModel.Services["app"].Environment["VALUE"])
	independent := filepath.Join(root, "independent")
	require.NoError(t, os.Mkdir(independent, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(independent, "compose.yaml"), []byte("services:\n  app:\n    image: alpine:3.22\n"), 0600))
	_, _, err = LoadComposeProjectFromDir(ctx, independent, "independent", root, false, nil)
	require.NoError(t, err, "unattributable labels never fail another target")
	require.Equal(t, 1, lists)
}
func TestRecordedAttributionConflictAndFallback(t *testing.T) {
	for _, selection := range []string{"missing", "empty", "own", "own-canonical-directory", "conflict", "mismatch", "order"} {
		t.Run(selection, func(t *testing.T) {
			root, dir, _, label := recordedFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("VALUE=conventional\n"), 0600))
			labels := []map[string]string{label}
			switch selection {
			case "missing":
				delete(label, EnvironmentFileLabel)
			case "empty":
				label[EnvironmentFileLabel] = ""
			case "own":
				label[EnvironmentFileLabel] = filepath.Join(dir, ".env")
			case "own-canonical-directory":
				label[EnvironmentFileLabel] = filepath.Join(dir, ".env")
				alias := filepath.Join(root, "alias")
				require.NoError(t, os.Symlink("stack", alias))
				dir = alias
			case "conflict":
				second := map[string]string{}
				for k, v := range label {
					second[k] = v
				}
				second[EnvironmentFileLabel] = ""
				labels = append(labels, second)
			case "mismatch":
				extra := filepath.Join(dir, "different.yaml")
				require.NoError(t, os.WriteFile(extra, []byte("services: {}\n"), 0600))
				label["com.docker.compose.project.config_files"] = extra
			case "order":
				label["com.docker.compose.project.config_files"] += "," + label["com.docker.compose.project.config_files"]
			}
			ctx := recordedContext(t, labels...)
			model, _, err := LoadComposeProjectFromDir(ctx, dir, "stack", root, false, nil)
			if selection == "conflict" || selection == "mismatch" || selection == "order" {
				require.Error(t, err)
				require.Equal(t, "compose.env-label-conflict", ClassifyLoadError(err).Reason)
			} else {
				require.NoError(t, err)
				require.Equal(t, "conventional", *model.Services["app"].Environment["VALUE"])
			}
		})
	}
}
func TestRecordedCacheResolvesLabelsAndBytes(t *testing.T) {
	root, dir, env, label := recordedFixture(t)
	cache := NewParsedComposeCache()
	load := func() string {
		ctx := recordedContext(t, label)
		model, err := LoadCachedComposeProject(ctx, cache, "stack", dir, filepath.Join(dir, "compose.yaml"), "stack", root, false, nil)
		require.NoError(t, err)
		return *model.Services["app"].Environment["VALUE"]
	}
	require.Equal(t, "recorded-sentinel", load())
	conflicting := map[string]string{}
	for key, value := range label {
		conflicting[key] = value
	}
	conflicting[EnvironmentFileLabel] = ""
	_, conflictErr := LoadCachedComposeProject(recordedContext(t, label, conflicting), cache, "stack", dir, filepath.Join(dir, "compose.yaml"), "stack", root, false, nil)
	require.Error(t, conflictErr)
	require.Equal(t, "compose.env-label-conflict", ClassifyLoadError(conflictErr).Reason)

	info, err := os.Stat(env)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(env, []byte("VALUE=changed\n"), 0600))
	require.NoError(t, os.Chtimes(env, info.ModTime(), info.ModTime()))
	require.Equal(t, "changed", load())
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("VALUE=default\n"), 0600))
	label[EnvironmentFileLabel] = ""
	require.Equal(t, "default", load())
	label[EnvironmentFileLabel] = env
	require.Equal(t, "changed", load())
	target := filepath.Join(root, "target.env")
	require.NoError(t, os.WriteFile(target, []byte("VALUE=retargeted\n"), 0600))
	require.NoError(t, os.Remove(env))
	require.NoError(t, os.Symlink("target.env", env))
	require.Equal(t, "retargeted", load())
	require.NoError(t, os.Remove(target))
	_, err = LoadCachedComposeProject(recordedContext(t, label), cache, "stack", dir, filepath.Join(dir, "compose.yaml"), "stack", root, false, nil)
	require.Error(t, err)
	require.Equal(t, "compose.env-label-invalid", ClassifyLoadError(err).Reason)
	require.NoError(t, os.WriteFile(target, []byte("VALUE=repaired\n"), 0600))
	require.Equal(t, "repaired", load())
}

func TestRecordedSafeLoadSinkClasses(t *testing.T) {
	root, dir, env, label := recordedFixture(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	require.NoError(t, os.WriteFile(env, []byte("VALUE=recorded-sentinel\nCOMPOSE_PARALLEL_LIMIT=recorded-sentinel\nCOMPOSE_REMOVE_ORPHANS=recorded-sentinel\nCOMPOSE_IGNORE_ORPHANS=recorded-sentinel\n"), 0600))
	ctx := recordedContext(t, label)
	_, _, err := LoadComposeProjectFromDir(ctx, dir, "stack", root, false, nil)
	require.NoError(t, err)
	// Raw required-variable messages and panic values are adversarial operator
	// text; only a typed problem may reach callers and log handlers.
	composePath := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(composePath, []byte("services:\n  app:\n    image: alpine:${MISSING:?recorded-sentinel}\n"), 0600))
	_, err = LoadComposeProject(ctx, composePath, "stack", root, false, nil, nil, nil, false, nil, nil, nil)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "recorded-sentinel")
	err = ValidateComposeContentForUpdate(ctx, root, dir, "stack", "services:\n  app:\n    image: alpine:${MISSING:?recorded-sentinel}\n", nil, nil, "", false)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "recorded-sentinel")
	_, err = ParseIncludesFromContent(composePath, []byte("include: ['${MISSING:?recorded-sentinel}']\n"), nil, false)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "recorded-sentinel")
	// The optional volume-source pass logs its own failure before the outer
	// loader can wrap it, so that sink must classify the error independently.
	raw := harvestRawSourcesInternal(ctx, composetypes.ConfigDetails{
		WorkingDir:  dir,
		ConfigFiles: []composetypes.ConfigFile{{Filename: composePath, Content: []byte("services:\n  app:\n    image: alpine:${MISSING:?recorded-sentinel}\n")}},
	}, func(o *loader.Options) { o.SetProjectName("example", true) })
	require.Nil(t, raw)
	require.Contains(t, logs.String(), "compose.variable-missing")
	var model *composetypes.Project
	func() {
		defer recoverComposeLoadPanicInternal(ctx, composePath, &model, &err)
		panic("recorded-sentinel")
	}()
	require.Error(t, err)
	require.NotContains(t, err.Error(), "recorded-sentinel")
	require.NotContains(t, logs.String(), "recorded-sentinel")
}

func TestRecordedInputGrowthIsBoundedAfterStat(t *testing.T) {
	root, dir, path, label := recordedFixture(t)
	original := readRecordedBytes
	t.Cleanup(func() { readRecordedBytes = original })
	readRecordedBytes = func(file *os.File) ([]byte, error) {
		writer, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		require.NoError(t, err)
		_, err = writer.WriteString("GROWTH=" + strings.Repeat("a", maxRecordedEnvBytes*2) + "\n")
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		body, err := original(file)
		require.LessOrEqual(t, len(body), maxRecordedEnvBytes+1, "descriptor reads stop at the cap plus one overflow byte, including growth after fstat")
		return body, err
	}
	_, err := ResolveComposeInputs(recordedContext(t, label), root, dir, false)
	require.Error(t, err)
	require.Equal(t, "compose.env-label-invalid", ClassifyLoadError(err).Reason)
}

func TestRecordedListingFailureUsesConventionalInputs(t *testing.T) {
	for _, failure := range []string{"not-found", "daemon", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			root, dir, _, label := recordedFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("VALUE=conventional\n"), 0600))
			var logs bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			t.Cleanup(func() { slog.SetDefault(previous) })
			calls := 0
			failureErr := error(fmt.Errorf("synthetic %s recorded-sentinel", failure))
			if failure == "timeout" {
				failureErr = context.DeadlineExceeded
			}
			ctx := WithComposeContainerLabels(t.Context(), func(context.Context) ([]map[string]string, error) {
				calls++
				return []map[string]string{label}, failureErr
			})
			for repeat := 0; repeat < 2; repeat++ {
				model, _, err := LoadComposeProjectFromDir(ctx, dir, "stack", root, false, nil)
				require.NoError(t, err)
				require.Equal(t, "conventional", *model.Services["app"].Environment["VALUE"])
				require.Empty(t, model.Services["app"].CustomLabels[EnvironmentFileLabel])
			}
			require.Equal(t, 1, calls)
			require.Equal(t, 1, strings.Count(logs.String(), "recorded Compose input attribution unavailable"))
			require.NotContains(t, logs.String(), "recorded-sentinel")
		})
	}
	// A successful response is authoritative input evidence, even when it is
	// unusable: ambiguity remains a target-local named problem, never fallback.
	root, dir, _, label := recordedFixture(t)
	other := map[string]string{}
	for key, value := range label {
		other[key] = value
	}
	other[EnvironmentFileLabel] = ""
	_, _, err := LoadComposeProjectFromDir(recordedContext(t, label, other), dir, "stack", root, false, nil)
	require.Error(t, err)
	require.Equal(t, "compose.env-label-conflict", ClassifyLoadError(err).Reason)
}
