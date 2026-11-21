// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original shared resolved Compose inputs; M34 #548.

package projects

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	compose "github.com/compose-spec/compose-go/v2/types"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
)

const EnvironmentFileLabel = "com.docker.compose.project.environment_file"
const maxRecordedEnvBytes = 1 << 20

type ExternalInput struct{ Path, SHA256 string }
type RecordedFile struct {
	Path  string
	Bytes []byte
}

// ResolvedInputs binds selection, interpolation and dependency identity to the
// same descriptor-read bytes. It is operation-local and never serialized to APIs.
type ResolvedInputs struct {
	State          string
	Directory      string
	Paths          []string
	Files          []RecordedFile
	Environment    EnvMap
	Injection      EnvMap
	External       []ExternalInput
	DependencyHash string
	configFiles    []string
}

type inputKey struct{}
type labelsKey struct{}
type labelSource struct {
	once   sync.Once
	read   func(context.Context) ([]map[string]string, error)
	labels []map[string]string
	err    error
}

// WithComposeContainerLabels shares one fresh, bounded listing across a request.
// Byte resolution is deliberately not cached with that listing.
func WithComposeContainerLabels(ctx context.Context, read func(context.Context) ([]map[string]string, error)) context.Context {
	if ctx.Value(labelsKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, labelsKey{}, &labelSource{read: read})
}
func HasComposeContainerLabels(ctx context.Context) bool { return ctx.Value(labelsKey{}) != nil }
func WithResolvedInputs(ctx context.Context, input *ResolvedInputs) context.Context {
	return context.WithValue(ctx, inputKey{}, input)
}
func (i *ResolvedInputs) Recorded() bool { return i != nil && i.State == "recorded" }
func inputProblem(reason string) error   { return &SafeLoadError{Problem: LoadProblem{Reason: reason}} }

// ResolveComposeInputs is the single entry to conventional or recorded inputs.
// Bootstrap uses directory/config labels only, never the display owner or name.
func ResolveComposeInputs(ctx context.Context, root, dir string, inject bool) (*ResolvedInputs, error) {
	if pinned, ok := ctx.Value(inputKey{}).(*ResolvedInputs); ok && pinned != nil && filepath.Clean(pinned.Directory) == filepath.Clean(dir) {
		return pinned, nil
	}
	input := &ResolvedInputs{State: "none", Directory: dir}
	if source, ok := ctx.Value(labelsKey{}).(*labelSource); ok {
		source.once.Do(func() {
			source.labels, source.err = source.read(ctx)
			if source.err != nil {
				source.labels = nil
				slog.DebugContext(ctx, "recorded Compose input attribution unavailable", "class", recordedListingErrorClass(source.err))
			}
		})
		realDir, err := filepath.EvalSymlinks(dir)
		if err == nil {
			var selected []string
			found := false
			for _, labels := range source.labels {
				if strings.EqualFold(labels["com.docker.compose.oneoff"], "true") {
					continue
				}
				work, e := filepath.EvalSymlinks(labels["com.docker.compose.project.working_dir"])
				if e != nil || work != realDir {
					continue
				}
				configs, valid := canonicalConfigFiles(realDir, labels["com.docker.compose.project.config_files"])
				if !valid {
					continue
				}
				paths := labelEntries(labels[EnvironmentFileLabel])
				if len(paths) == 1 && (paths[0] == filepath.Join(dir, ".env") || paths[0] == filepath.Join(realDir, ".env")) {
					paths = nil
				}
				if found && (!slices.Equal(selected, paths) || !slices.Equal(input.configFiles, configs)) {
					input.State = "conflict"
					return input, inputProblem("compose.env-label-conflict")
				}
				found = true
				selected = paths
				input.configFiles = configs
			}
			if len(selected) > 0 {
				input.State = "recorded"
				input.Paths = selected
				if err := input.readRecorded(root, inject); err != nil {
					input.State = "invalid"
					return input, inputProblem("compose.env-label-invalid")
				}
			}
		}
	}
	if !input.Recorded() {
		env, injected, err := NewEnvLoader(root, dir, inject).loadConventionalEnvironment(ctx)
		if err != nil {
			return input, err
		}
		input.Environment, input.Injection = env, injected
	}
	hashes := make([]ExternalInput, 0, len(input.Files))
	for _, file := range input.Files {
		sum := sha256.Sum256(file.Bytes)
		hashes = append(hashes, ExternalInput{Path: file.Path, SHA256: hex.EncodeToString(sum[:])})
	}
	body, _ := json.Marshal([]any{input.State, input.Paths, hashes})
	sum := sha256.Sum256(body)
	input.DependencyHash = hex.EncodeToString(sum[:])
	return input, nil
}
func labelEntries(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}
func canonicalConfigFiles(dir, raw string) ([]string, bool) {
	entries := labelEntries(raw)
	if len(entries) == 0 {
		return nil, false
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !filepath.IsAbs(entry) {
			return nil, false
		}
		path, err := filepath.EvalSymlinks(entry)
		if err != nil || !pathBelow(dir, path) {
			return nil, false
		}
		paths = append(paths, path)
	}
	return paths, true
}
func pathBelow(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (i *ResolvedInputs) readRecorded(configured string, inject bool) error {
	configured = filepath.Clean(configured)
	rootPath, err := filepath.EvalSymlinks(configured)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	i.Environment = allowedProcessEnvInternal()
	i.Injection = EnvMap{}
	for _, path := range i.Paths {
		content, err := readRecordedFile(root, path, configured, rootPath)
		if err != nil {
			return err
		}
		values, err := ParseProjectEnvContent(string(content), i.Environment)
		if err != nil {
			return err
		}
		maps.Copy(i.Environment, values)
		if inject {
			maps.Copy(i.Injection, values)
		}
		sum := sha256.Sum256(content)
		i.Files = append(i.Files, RecordedFile{Path: path, Bytes: content})
		i.External = append(i.External, ExternalInput{Path: path, SHA256: hex.EncodeToString(sum[:])})
	}
	// Explicit --env-file replaces defaults; nested COMPOSE_ENV_FILES is inert.
	delete(i.Environment, composeEnvFilesKey)
	delete(i.Injection, composeEnvFilesKey)
	return nil
}

// Open relative to the root and validate/read that descriptor. Nonblocking
// open prevents a FIFO from blocking before its file type can be rejected.
// The label may name the configured or the resolved projects directory; the
// os.Root confines the open either way.
func readRecordedFile(root *os.Root, path string, bases ...string) ([]byte, error) {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, os.ErrInvalid
	}
	rel := ""
	for _, base := range bases {
		if pathBelow(base, path) {
			rel, _ = filepath.Rel(base, path)
			break
		}
	}
	if rel == "" {
		return nil, os.ErrInvalid
	}
	file, err := openRecordedFile(root, rel, os.O_RDONLY|syscall.O_NONBLOCK)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrInvalid
	}
	content, err := readRecordedBytes(file)
	if err != nil {
		return nil, err
	}
	if len(content) > maxRecordedEnvBytes {
		return nil, os.ErrInvalid
	}
	return content, nil
}

func (i *ResolvedInputs) confirm(model *compose.Project) error {
	if !i.Recorded() {
		return nil
	}
	dir, err := filepath.EvalSymlinks(model.WorkingDir)
	expected, e := filepath.EvalSymlinks(i.Directory)
	files, valid := canonicalConfigFiles(expected, strings.Join(model.ComposeFiles, ","))
	if err != nil || e != nil || dir != expected || !valid || !slices.Equal(files, i.configFiles) {
		return inputProblem("compose.env-label-conflict")
	}
	return nil
}

// FreshComposeInputs discards an operation pin, retaining its request label
// source. Effect barriers must reread current external bytes.
func FreshComposeInputs(ctx context.Context, root, dir string, inject bool) (*ResolvedInputs, error) {
	return ResolveComposeInputs(context.WithValue(ctx, inputKey{}, (*ResolvedInputs)(nil)), root, dir, inject)
}

// Descriptor seam supports swap and flag regression probes without reopening.
var readRecordedBytes = func(file *os.File) ([]byte, error) { return io.ReadAll(io.LimitReader(file, maxRecordedEnvBytes+1)) }

// RebaseComposeInputs carries an already attributed selection into a disposable
// preview directory. External paths/bytes stay fixed; only definition paths move.
func RebaseComposeInputs(ctx context.Context, input *ResolvedInputs, dir string) context.Context {
	if !input.Recorded() {
		return ctx
	}
	preview := *input
	preview.Directory = dir
	preview.configFiles = nil
	for _, path := range input.configFiles {
		relative, err := filepath.Rel(input.Directory, path)
		if err != nil {
			return ctx
		}
		preview.configFiles = append(preview.configFiles, filepath.Join(dir, relative))
	}
	return WithResolvedInputs(ctx, &preview)
}

var openRecordedFile = func(root *os.Root, relative string, flags int) (*os.File, error) {
	return root.OpenFile(relative, flags, 0)
}

// LoadEnvironment loads and merges environment variables from all sources:
// 1. Allowlisted process environment (TZ)
// 2. Project-specific .env file (from workdir)
// The rest of the Arcane process environment is intentionally excluded so its
// own variables never leak into compose interpolation of managed projects.
func (l *EnvLoader) LoadEnvironment(ctx context.Context) (envMap, injectionVars EnvMap, err error) {
	input, err := ResolveComposeInputs(ctx, l.projectsDirectory, l.workdir, l.autoInjectEnv)
	if err != nil {
		return nil, nil, err
	}
	return maps.Clone(input.Environment), maps.Clone(input.Injection), nil
}

func WithTransientValidationEnvFile(ctx context.Context, projectPath string, effectiveEnvContent *string, run func() error) error {
	input, inputErr := ResolveComposeInputs(ctx, "", projectPath, false)
	if inputErr != nil {
		return inputErr
	}
	if input.Recorded() {
		if run != nil {
			return run()
		}
		return nil
	}
	return withConventionalTransientValidationEnvFile(ctx, projectPath, effectiveEnvContent, run)
}

func CheckProjectEnvAccess(ctx context.Context, projectsDir, dir string) *projecttypes.ConfigurationError {
	input, inputErr := ResolveComposeInputs(ctx, projectsDir, dir, false)
	if input.Recorded() || inputErr != nil && input != nil && input.State != "none" {
		return nil
	}
	return checkConventionalProjectEnvAccess(ctx, projectsDir, dir)
}

func recordedListingErrorClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var timed interface{ Timeout() bool }
	if errors.As(err, &timed) && timed.Timeout() {
		return "timeout"
	}
	return "unavailable"
}
