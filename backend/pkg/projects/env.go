// Source: getarcaneapp/arcane/backend/pkg/projects/env.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories.

package projects

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/compose-spec/compose-go/v2/consts"
	"github.com/compose-spec/compose-go/v2/dotenv"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"go.getarcane.app/acfs"
	kit "go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/common"
)

const (
	EffectiveEnvFileName                = ".env"
	ProjectEnvModeDirect ProjectEnvMode = "direct"

	// Docker Compose pre-defined variable names that compose-go's consts package
	// does not export. See https://docs.docker.com/compose/how-tos/environment-variables/envvars.
	composeEnvFilesKey      = "COMPOSE_ENV_FILES"
	composeRemoveOrphansKey = "COMPOSE_REMOVE_ORPHANS"
	composeIgnoreOrphansKey = "COMPOSE_IGNORE_ORPHANS"
	composeParallelLimitKey = "COMPOSE_PARALLEL_LIMIT"

	defaultComposePathSeparator = ":"
)

type EnvMap = map[string]string

type ProjectEnvMode string

type ProjectEnvState struct {
	Mode             ProjectEnvMode
	EditableFileName string
	EditableContent  string
	EffectiveContent string
	DirectContent    string
	HasEffective     bool
	// The *Unreadable fields report a file that exists on disk but could not be
	// read because of a permission error (e.g. a chmod 000 or foreign-owned
	// file reachable through a bind mount). Such a file is treated as absent
	// for merge purposes, and callers persisting env state must not attempt to
	// write or remove it — its contents are unknown, so writing could either
	// fail (bricking the caller) or silently clobber operator intent.
	EffectiveUnreadable bool
}

type EnvLoader struct {
	workdir       string
	autoInjectEnv bool
}

func NewEnvLoader(_ string, workdir string, autoInjectEnv bool) *EnvLoader {
	return &EnvLoader{workdir: workdir, autoInjectEnv: autoInjectEnv}
}

// processEnvAllowlist is the only part of Arcane's own process environment
// that flows into compose interpolation of managed projects: timezone and
// locale, whose container values are safe to share. Everything else is
// excluded so Arcane's variables never leak into ${VAR} references or
// pass-through environment entries — its PORT collides with project port
// mappings, secrets would be readable from any compose file, and vars like
// HOME or PUID carry container-internal values that are wrong for projects.
var processEnvAllowlist = []string{"TZ", "LANG", "LANGUAGE", "LC_ALL"}

func allowedProcessEnvInternal() EnvMap {
	envMap := make(EnvMap)
	for _, key := range processEnvAllowlist {
		if val, ok := os.LookupEnv(key); ok {
			envMap[key] = val
		}
	}
	return envMap
}

// LoadEnvironment loads and merges environment variables from all sources:
// 1. Allowlisted process environment (TZ)
// 2. Project-specific .env file (from workdir)
// The rest of the Arcane process environment is intentionally excluded so its
// own variables never leak into compose interpolation of managed projects.
func (l *EnvLoader) LoadEnvironment(ctx context.Context) (envMap, injectionVars EnvMap, err error) {
	envMap = allowedProcessEnvInternal()
	injectionVars = make(EnvMap)

	// COMPOSE_DISABLE_ENV_FILE skips the project .env when supplied by an
	// allowed process source.
	if parseComposeBoolInternal(envMap, consts.ComposeDisableDefaultEnvFile) {
		slog.DebugContext(ctx, "COMPOSE_DISABLE_ENV_FILE set; skipping project .env", "workdir", l.workdir)
	} else {
		projectEnvPath := filepath.Join(l.workdir, EffectiveEnvFileName)
		if loadAndMergeProjectEnvErr := l.loadAndMergeProjectEnv(ctx, projectEnvPath, envMap, injectionVars); loadAndMergeProjectEnvErr != nil {
			switch {
			case errors.Is(loadAndMergeProjectEnvErr, os.ErrNotExist):
				slog.DebugContext(ctx, "Project .env file does not exist", "path", projectEnvPath)
			case errors.Is(loadAndMergeProjectEnvErr, syscall.EISDIR):
				// A non-file .env retains the existing absent/leave-untouched policy.
				slog.WarnContext(ctx, "Project .env path is a directory; skipping", "path", projectEnvPath)
			case errors.Is(loadAndMergeProjectEnvErr, os.ErrPermission):
				return envMap,
					injectionVars,
					common.Classify(common.ErrProjectEnvUnreadable,
						fmt.Errorf("%s is not readable by the runtime user (uid %d, gid %d); fix its ownership/read permission or set PUID/PGID to a user that can read it: %w",
							projectEnvPath,
							os.Geteuid(),
							os.Getegid(),
							loadAndMergeProjectEnvErr))
			default:
				return envMap, injectionVars, loadAndMergeProjectEnvErr
			}
		}
	}

	// COMPOSE_ENV_FILES, when declared, layers additional env files on top of
	// the project .env (later entries win), reading current bytes each time.
	l.mergeComposeEnvFilesInternal(ctx, envMap, injectionVars)

	return envMap, injectionVars, nil
}

func (l *EnvLoader) mergeComposeEnvFilesInternal(ctx context.Context, envMap, injectionVars EnvMap) {
	onMerged := func(values EnvMap) {
		if l.autoInjectEnv {
			maps.Copy(injectionVars, values)
		}
	}
	mergeComposeEnvFilesInternal(ctx, l.workdir, envMap, parseProjectEnvFileExistingInternal, onMerged)
}

func (l *EnvLoader) loadAndMergeProjectEnv(ctx context.Context, path string, envMap, injectionVars EnvMap) error {
	values, err := parseProjectEnvFileExistingInternal(path, envMap)
	if err != nil {
		return err
	}

	for k, v := range values {
		envMap[k] = v
		if l.autoInjectEnv {
			injectionVars[k] = v
		}
	}

	slog.DebugContext(ctx, "Merged project .env into environment map", "totalEnvCount", len(envMap))
	return nil
}

// Read and parse the same bytes on every load, including supported external
// symlink targets. Modification times are not authoritative environment data.
func parseProjectEnvFileExistingInternal(path string, contextEnv EnvMap) (EnvMap, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	return ParseProjectEnvContent(string(content), contextEnv)
}

// ParseProjectEnvFile parses a project .env file with variable expansion using the provided
// context map (e.g. process env). Returns nil without error when the file does not exist.
// Only the specified file is read — global env files are intentionally not loaded here.
//
// Stays on os.*: env files may be symlinks resolving outside any confinement
// root (a supported setup), which acfs cannot follow.
func ParseProjectEnvFile(path string, contextEnv EnvMap) (EnvMap, error) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil, nil //nolint:nilerr // missing .env is not an error
	}
	return parseProjectEnvFileExistingInternal(path, contextEnv)
}

// ParseProjectEnvContent parses project .env content from a string using
// compose-go's dotenv parser with variable expansion. Lookups resolve from
// contextEnv (previously loaded vars) only; the Arcane process environment is
// intentionally never consulted so its variables don't leak into project env.
func ParseProjectEnvContent(content string, contextEnv EnvMap) (EnvMap, error) {
	lookupFn := func(key string) (string, bool) {
		val, ok := contextEnv[key]
		return val, ok
	}

	envMap, err := dotenv.ParseWithLookup(strings.NewReader(content), lookupFn)
	if err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	return envMap, nil
}

// WithTransientValidationEnvFile temporarily writes a project .env file while
// running compose validation, then restores the original file state.
func WithTransientValidationEnvFile(ctx context.Context, projectPath string, effectiveEnvContent *string, run func() error) (err error) {
	// Read through os rather than the root-confined API: a project .env is
	// allowed to be a symlink whose target lives outside the project directory,
	// and that write-through is deliberately preserved (#3556).
	originalContent, readErr := os.ReadFile(filepath.Join(projectPath, ".env"))
	originalExists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		if !errors.Is(readErr, os.ErrPermission) && !errors.Is(readErr, syscall.EISDIR) {
			return fmt.Errorf("prepare env file for compose validation: %w", readErr)
		}
		// The path exists but is permission-locked (e.g. chmod 000, foreign-owned) or a directory.
		// Its contents can't be verified or safely overwritten, so leave it
		// untouched and validate against whatever's already on disk instead of
		// aborting the whole update.
		slog.WarnContext(ctx, "skipping unreadable .env during compose validation; leaving it untouched", "projectPath", projectPath, "error", readErr)
		if run == nil {
			return nil
		}
		return run()
	}

	contentMatches := effectiveEnvContent != nil && originalExists && string(originalContent) == *effectiveEnvContent
	shouldWrite := !contentMatches && (effectiveEnvContent != nil || !originalExists)
	if shouldWrite {
		content := ""
		if effectiveEnvContent != nil {
			content = *effectiveEnvContent
		}
		if writeErr := WriteProjectFile(ctx, projectPath, projectPath, ".env", content); writeErr != nil {
			return fmt.Errorf("prepare env file for compose validation: %w", writeErr)
		}

		defer func() {
			var restoreErr error
			switch {
			case originalExists:
				restoreErr = WriteProjectFile(ctx, projectPath, projectPath, ".env", string(originalContent))
			default:
				restoreErr = acfs.Remove(ctx, projectPath, "/.env")
			}

			if restoreErr != nil && !os.IsNotExist(restoreErr) {
				if err == nil {
					err = fmt.Errorf("restore env file after compose validation: %w", restoreErr)
				}
			}
		}()
	}

	if run == nil {
		return nil
	}

	return run()
}

// CheckProjectEnvAccess reports a configuration error when the runtime user
// lacks permission to open dir's .env or traverse its path.
func CheckProjectEnvAccess(ctx context.Context, projectsDir, dir string) *projecttypes.ConfigurationError {
	envPath := filepath.Join(dir, EffectiveEnvFileName)
	file, err := os.Open(envPath)
	if err == nil {
		if closeErr := file.Close(); closeErr != nil {
			slog.WarnContext(ctx, "failed to close project env access probe", "path", envPath, "error", closeErr)
		}
		return nil
	}
	if !errors.Is(err, os.ErrPermission) {
		return nil
	}

	return &projecttypes.ConfigurationError{
		Code:             common.ConfigurationErrorCodeEnvFileUnreadable,
		Path:             envPath,
		UID:              os.Geteuid(),
		GID:              os.Getegid(),
		BlocksOperations: true,
	}
}

func ReadProjectEnvState(projectPath string) (ProjectEnvState, error) {
	effectiveContent, hasEffective, effectiveUnreadable, err := readOptionalProjectFileInternal(projectPath, EffectiveEnvFileName)
	if err != nil {
		return ProjectEnvState{}, err
	}

	state := ProjectEnvState{
		Mode:                ProjectEnvModeDirect,
		EditableFileName:    EffectiveEnvFileName,
		EditableContent:     effectiveContent,
		DirectContent:       effectiveContent,
		EffectiveContent:    effectiveContent,
		HasEffective:        hasEffective,
		EffectiveUnreadable: effectiveUnreadable,
	}
	return state, nil
}

// WriteManagedEnvFile writes the project's effective .env file.
func WriteManagedEnvFile(ctx context.Context, projectsDirectory, projectPath, fileName string, unreadable bool, content string) error {
	if fileName != EffectiveEnvFileName {
		return fmt.Errorf("write managed env file: unsupported file name %q", fileName)
	}
	if unreadable {
		slog.WarnContext(ctx, "skipping unreadable project env file; leaving it untouched", "projectPath", projectPath, "file", fileName)
		return nil
	}
	return WriteProjectFile(ctx, projectsDirectory, projectPath, EffectiveEnvFileName, content)
}

// readOptionalProjectFileInternal reads fileName from projectPath. A missing
// file is reported via exists=false with no error. A permission error or a
// directory at the path is reported via unreadable=true with no error: its
// contents cannot be verified, so callers must treat it as absent for merge
// purposes and must not attempt to overwrite or remove it. Any other I/O
// error is still returned as a hard failure.
// A project env file may itself be a symlink whose target lives outside the
// project directory, so the read goes through os rather than the root-confined
// API — the same deliberate exception the .env write path makes (#3556).
func readOptionalProjectFileInternal(projectPath, fileName string) (content string, exists, unreadable bool, err error) {
	raw, readErr := os.ReadFile(filepath.Join(projectPath, fileName))
	if readErr == nil {
		return string(raw), true, false, nil
	}
	if errors.Is(readErr, os.ErrNotExist) {
		return "", false, false, nil
	}
	if errors.Is(readErr, os.ErrPermission) || errors.Is(readErr, syscall.EISDIR) {
		return "", false, true, nil
	}
	return "", false, false, fmt.Errorf("read %s: %w", fileName, readErr)
}

// ComposeEnvOptions holds the deployment-relevant Docker Compose pre-defined
// environment variables parsed from a project's merged environment.
type ComposeEnvOptions struct {
	// ConfigFiles is the resolved, absolute COMPOSE_FILE selection in order (the
	// first entry is the base file). Nil when COMPOSE_FILE is unset.
	ConfigFiles []string
	// Profiles is the COMPOSE_PROFILES selection.
	Profiles []string
	// ProjectName is COMPOSE_PROJECT_NAME (not normalized).
	ProjectName string
	// EnvFiles is the resolved, absolute COMPOSE_ENV_FILES selection in order.
	EnvFiles []string
	// RemoveOrphans is COMPOSE_REMOVE_ORPHANS.
	RemoveOrphans bool
	// IgnoreOrphans is COMPOSE_IGNORE_ORPHANS.
	IgnoreOrphans bool
	// ParallelLimit is COMPOSE_PARALLEL_LIMIT; 0 means unset.
	ParallelLimit int
}

// ParseComposeEnvOptions extracts the deployment-relevant COMPOSE_* variables
// from an already-merged environment map. Invalid auxiliary paths are skipped,
// matching environment loading. Malformed COMPOSE_FILE selections return a
// classified common.ErrComposeFileEnvInvalid; malformed scalars are ignored
// with a debug log, matching `docker compose`'s tolerance.
func ParseComposeEnvOptions(workdir string, env EnvMap) (ComposeEnvOptions, error) {
	opts := ComposeEnvOptions{
		ProjectName: strings.TrimSpace(env[consts.ComposeProjectName]),
	}

	files, err := resolveComposeFileSelectionInternal(workdir, env)
	if err != nil {
		return ComposeEnvOptions{}, err
	}
	opts.ConfigFiles = files

	if raw := strings.TrimSpace(env[consts.ComposeProfiles]); raw != "" {
		opts.Profiles = kit.TrimNonEmpty(strings.Split(raw, ","))
	}

	for _, entry := range ComposeEnvFileEntriesFromEnv(env) {
		resolved, resErr := ResolvePathWithinDir(workdir, entry)
		if resErr != nil {
			// The environment loader already warns and skips excluded auxiliary paths.
			continue
		}
		opts.EnvFiles = append(opts.EnvFiles, resolved)
	}

	opts.RemoveOrphans = parseComposeBoolInternal(env, composeRemoveOrphansKey)
	opts.IgnoreOrphans = parseComposeBoolInternal(env, composeIgnoreOrphansKey)

	if raw := strings.TrimSpace(env[composeParallelLimitKey]); raw != "" {
		if n, convErr := strconv.Atoi(raw); convErr == nil && n > 0 {
			opts.ParallelLimit = n
		} else {
			slog.DebugContext(context.Background(), "ignoring invalid COMPOSE_PARALLEL_LIMIT", "value", raw) //nolint:forbidigo // Compose environment parsing is a pure helper without a request context.
		}
	}

	return opts, nil
}

// ComposeFileEnvSelection resolves the COMPOSE_FILE selection for dir with the
// project .env and COMPOSE_ENV_FILES entries. Returns nil when the merged
// environment does not set COMPOSE_FILE.
func ComposeFileEnvSelection(ctx context.Context, projectsDir, dir string) ([]string, error) {
	envMap, _, err := NewEnvLoader(projectsDir, dir, false).LoadEnvironment(ctx)
	if err != nil {
		return nil, err
	}
	return resolveComposeFileSelectionInternal(dir, envMap)
}

// ComposeFileEntriesFromEnv returns the raw COMPOSE_FILE entries declared in env
// (split on COMPOSE_PATH_SEPARATOR, ":" by default), or nil when unset. Entries
// are not resolved, validated, or stat'd.
func ComposeFileEntriesFromEnv(env EnvMap) []string {
	raw, ok := env[consts.ComposeFilePath]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil
	}

	separator := defaultComposePathSeparator
	if custom := strings.TrimSpace(env[consts.ComposePathSeparator]); custom != "" {
		separator = custom
	}

	return kit.TrimNonEmpty(strings.Split(raw, separator))
}

// ComposeEnvFileEntriesFromEnv returns the raw COMPOSE_ENV_FILES entries
// declared in env (comma-separated), or nil when unset. Entries are not
// resolved, validated, or stat'd.
func ComposeEnvFileEntriesFromEnv(env EnvMap) []string {
	raw := strings.TrimSpace(env[composeEnvFilesKey])
	if raw == "" {
		return nil
	}
	return kit.TrimNonEmpty(strings.Split(raw, ","))
}

// resolveComposeFileSelectionInternal parses COMPOSE_FILE from env into an
// ordered list of absolute compose-file paths, or nil when COMPOSE_FILE is
// unset. Every entry must be a relative path to an existing file within
// workdir; the first entry (the base file) must sit directly in workdir.
func resolveComposeFileSelectionInternal(workdir string, env EnvMap) ([]string, error) {
	entries := ComposeFileEntriesFromEnv(env)
	if len(entries) == 0 {
		return nil, nil
	}

	absWorkdir, err := filepath.Abs(filepath.Clean(workdir))
	if err != nil {
		return nil, common.Classify(common.ErrComposeFileEnvInvalid, fmt.Errorf("resolve project directory: %w", err))
	}

	files := make([]string, 0, len(entries))
	for i, entry := range entries {
		if filepath.IsAbs(entry) {
			return nil, common.Classify(common.ErrComposeFileEnvInvalid, fmt.Errorf("COMPOSE_FILE entry %q must be relative to the project directory", entry))
		}

		resolved, resErr := ResolvePathWithinDir(absWorkdir, entry)
		if resErr != nil {
			return nil, common.Classify(common.ErrComposeFileEnvInvalid, fmt.Errorf("COMPOSE_FILE entry %q: %w", entry, resErr))
		}

		if i == 0 && filepath.Dir(resolved) != absWorkdir {
			return nil, common.Classify(common.ErrComposeFileEnvInvalid, fmt.Errorf("the first COMPOSE_FILE entry %q must be in the project directory", entry))
		}

		// os.Stat rather than acfs: compose files may be symlinks resolving
		// outside any confinement root, and workdir can be an imported project
		// living outside the projects directory.
		info, statErr := os.Stat(resolved)
		if statErr != nil {
			return nil, common.Classify(common.ErrComposeFileEnvInvalid, fmt.Errorf("COMPOSE_FILE entry %q: %w", entry, statErr))
		}
		if info.IsDir() {
			return nil, common.Classify(common.ErrComposeFileEnvInvalid, fmt.Errorf("COMPOSE_FILE entry %q is a directory", entry))
		}

		files = append(files, resolved)
	}

	return files, nil
}

// mergeComposeEnvFilesInternal merges each COMPOSE_ENV_FILES entry (in order,
// later wins) on top of envMap; bad entries are warned and skipped. Unlike
// `docker compose`, the entries are additive on top of the project .env rather
// than a replacement: .env is Arcane's managed effective env file and the place
// COMPOSE_ENV_FILES itself is declared, so replacing it would be self-defeating.
func mergeComposeEnvFilesInternal(ctx context.Context, workdir string, envMap EnvMap, parse func(path string, contextEnv EnvMap) (EnvMap, error), onMerged func(values EnvMap)) {
	for _, entry := range ComposeEnvFileEntriesFromEnv(envMap) {
		resolved, err := ResolvePathWithinDir(workdir, entry)
		if err != nil {
			slog.WarnContext(ctx, "skipping COMPOSE_ENV_FILES entry outside project directory", "entry", entry, "error", err)
			continue
		}
		// os.Stat rather than acfs: env files may be symlinks resolving outside
		// any confinement root (a supported setup).
		if _, statErr := os.Stat(resolved); statErr != nil {
			slog.WarnContext(ctx, "COMPOSE_ENV_FILES entry not found", "path", resolved, "error", statErr)
			continue
		}
		values, err := parse(resolved, envMap)
		if err != nil {
			slog.WarnContext(ctx, "failed to load COMPOSE_ENV_FILES entry", "path", resolved, "error", err)
			continue
		}
		if len(values) == 0 {
			continue
		}
		maps.Copy(envMap, values)
		if onMerged != nil {
			onMerged(values)
		}
	}
}

func parseComposeBoolInternal(env EnvMap, key string) bool {
	raw := strings.TrimSpace(env[key])
	if raw == "" {
		return false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		slog.DebugContext(context.Background(), "ignoring invalid compose boolean env var", //nolint:forbidigo // Compose environment parsing is a pure helper without a request context.
			"key", key, "value", raw)
		return false
	}
	return v
}
