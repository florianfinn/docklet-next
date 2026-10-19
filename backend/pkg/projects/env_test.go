// Source: getarcaneapp/arcane/backend/pkg/projects/env_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories.

package projects

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
)

func TestLoadEnvironmentUsesProjectEnvOnly(t *testing.T) {
	projectsDir := t.TempDir()
	workdir := filepath.Join(projectsDir, "myproject")
	require.NoError(t, os.MkdirAll(workdir, utils.DirPerm))
	require.NoError(t, os.WriteFile(filepath.Join(projectsDir, ".env.global"), []byte("SHARED_VAR=global\n"), utils.FilePerm))
	require.NoError(t, os.WriteFile(filepath.Join(workdir, ".env"), []byte("PROJECT_VAR=project\n"), utils.FilePerm))

	for _, autoInject := range []bool{false, true} {
		envMap, injectionVars, err := NewEnvLoader(projectsDir, workdir, autoInject).LoadEnvironment(t.Context())
		require.NoError(t, err)
		assert.Equal(t, "project", envMap["PROJECT_VAR"])
		assert.NotContains(t, envMap, "SHARED_VAR")
		assert.Equal(t, autoInject, injectionVars["PROJECT_VAR"] == "project")
	}
}

func TestWithTransientValidationEnvFile_PreservesExternalSymlink(t *testing.T) {
	projectDir := t.TempDir()
	targetPath := filepath.Join(t.TempDir(), "project.env")
	originalContent := "VALUE=original\n"
	targetPerm := os.FileMode(0o640)
	require.NoError(t, os.WriteFile(targetPath, []byte(originalContent), targetPerm))
	require.NoError(t, os.Chmod(targetPath, targetPerm))

	envPath := filepath.Join(projectDir, EffectiveEnvFileName)
	if err := os.Symlink(targetPath, envPath); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}
	originalLinkTarget, err := os.Readlink(envPath)
	require.NoError(t, err)

	validationErr := errors.New("validation failed")
	updatedContent := "VALUE=validation\n"
	err = WithTransientValidationEnvFile(t.Context(), projectDir, &updatedContent, func() error {
		content, readErr := os.ReadFile(targetPath)
		require.NoError(t, readErr)
		assert.Equal(t, updatedContent, string(content))

		linkInfo, statErr := os.Lstat(envPath)
		require.NoError(t, statErr)
		require.NotZero(t, linkInfo.Mode()&os.ModeSymlink)
		return validationErr
	})
	require.ErrorIs(t, err, validationErr)

	restoredContent, err := os.ReadFile(targetPath)
	require.NoError(t, err)
	assert.Equal(t, originalContent, string(restoredContent))
	currentLinkTarget, err := os.Readlink(envPath)
	require.NoError(t, err)
	assert.Equal(t, originalLinkTarget, currentLinkTarget)
	if runtime.GOOS != "windows" {
		targetInfo, statErr := os.Stat(targetPath)
		require.NoError(t, statErr)
		assert.Equal(t, targetPerm, targetInfo.Mode().Perm())
	}
}

func TestParseComposeEnvOptions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.yml"), []byte("services: {}\n"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "extra.yml"), []byte("services: {}\n"), 0o600))

	t.Run("scalars, profiles split, and custom separator", func(t *testing.T) {
		t.Parallel()
		env := EnvMap{
			"COMPOSE_FILE":           "base.yml;sub/extra.yml",
			"COMPOSE_PATH_SEPARATOR": ";",
			"COMPOSE_PROFILES":       "frontend, debug",
			"COMPOSE_PROJECT_NAME":   "myproj",
			"COMPOSE_REMOVE_ORPHANS": "true",
			"COMPOSE_IGNORE_ORPHANS": "1",
			"COMPOSE_PARALLEL_LIMIT": "4",
		}

		opts, err := ParseComposeEnvOptions(dir, env)
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(dir, "base.yml"), filepath.Join(dir, "sub", "extra.yml")}, opts.ConfigFiles)
		assert.Equal(t, []string{"frontend", "debug"}, opts.Profiles)
		assert.Equal(t, "myproj", opts.ProjectName)
		assert.True(t, opts.RemoveOrphans)
		assert.True(t, opts.IgnoreOrphans)
		assert.Equal(t, 4, opts.ParallelLimit)
	})

	t.Run("invalid scalars are ignored", func(t *testing.T) {
		t.Parallel()
		opts, err := ParseComposeEnvOptions(dir, EnvMap{
			"COMPOSE_REMOVE_ORPHANS": "nope",
			"COMPOSE_PARALLEL_LIMIT": "-3",
		})
		require.NoError(t, err)
		assert.False(t, opts.RemoveOrphans)
		assert.Zero(t, opts.ParallelLimit)
	})

	t.Run("COMPOSE_ENV_FILES resolved in order, traversal skipped", func(t *testing.T) {
		t.Parallel()
		opts, err := ParseComposeEnvOptions(dir, EnvMap{"COMPOSE_ENV_FILES": "sub/extra.env, .env.local"})
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(dir, "sub", "extra.env"), filepath.Join(dir, ".env.local")}, opts.EnvFiles)

		opts, err = ParseComposeEnvOptions(dir, EnvMap{"COMPOSE_ENV_FILES": "../outside.env"})
		require.NoError(t, err)
		assert.Empty(t, opts.EnvFiles)
	})

	t.Run("absolute entry rejected", func(t *testing.T) {
		t.Parallel()
		_, err := ParseComposeEnvOptions(dir, EnvMap{"COMPOSE_FILE": filepath.Join(dir, "base.yml")})
		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrComposeFileEnvInvalid)
	})

	t.Run("escaping entry rejected", func(t *testing.T) {
		t.Parallel()
		_, err := ParseComposeEnvOptions(dir, EnvMap{"COMPOSE_FILE": "../base.yml"})
		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrComposeFileEnvInvalid)
	})

	t.Run("non-root first entry rejected", func(t *testing.T) {
		t.Parallel()
		_, err := ParseComposeEnvOptions(dir, EnvMap{"COMPOSE_FILE": "sub/extra.yml:base.yml"})
		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrComposeFileEnvInvalid)
	})

	t.Run("missing entry rejected", func(t *testing.T) {
		t.Parallel()
		_, err := ParseComposeEnvOptions(dir, EnvMap{"COMPOSE_FILE": "base.yml:sub/missing.yml"})
		require.Error(t, err)
		assert.ErrorIs(t, err, common.ErrComposeFileEnvInvalid)
	})
}

func TestComposeFileEnvSelection(t *testing.T) {
	t.Parallel()

	t.Run("nil when no .env", func(t *testing.T) {
		t.Parallel()
		files, err := ComposeFileEnvSelection(t.Context(), "", t.TempDir())
		require.NoError(t, err)
		assert.Nil(t, files)
	})

	t.Run("resolves COMPOSE_FILE from .env", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "base.yml"), []byte("services: {}\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "extra.yml"), []byte("services: {}\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_FILE=base.yml:extra.yml\n"), 0o600))

		files, err := ComposeFileEnvSelection(t.Context(), "", dir)
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(dir, "base.yml"), filepath.Join(dir, "extra.yml")}, files)
	})

	t.Run("resolves COMPOSE_FILE from COMPOSE_ENV_FILES entry", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "base.yml"), []byte("services: {}\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "aux.env"), []byte("COMPOSE_FILE=base.yml\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_ENV_FILES=aux.env\n"), 0o600))

		files, err := ComposeFileEnvSelection(t.Context(), "", dir)
		require.NoError(t, err)
		assert.Equal(t, []string{filepath.Join(dir, "base.yml")}, files)
	})

}

func rewriteEnvSameMtime(t *testing.T, path, content string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
}

func TestEnvironmentFreshBytesAndContext(t *testing.T) {
	for _, source := range []string{"primary", "auxiliary", "primary-symlink", "auxiliary-symlink"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".env")
			auxiliary := source == "auxiliary" || source == "auxiliary-symlink"
			if auxiliary {
				require.NoError(t, os.WriteFile(path, []byte("COMPOSE_ENV_FILES=aux.env\nBASE=project\n"), 0600))
				path = filepath.Join(dir, "aux.env")
			}
			target := path
			if source == "primary-symlink" || source == "auxiliary-symlink" {
				target = filepath.Join(t.TempDir(), "external.env")
				require.NoError(t, os.Symlink(target, path))
			}
			t.Setenv("LANG", "example_locale")
			t.Setenv("DOCKLET_PRIVATE_TEST_VALUE", "must-not-leak")
			require.NoError(t, os.WriteFile(target, []byte("VALUE=first\nEXPANDED=${LANG}\nPRIVATE=${DOCKLET_PRIVATE_TEST_VALUE:-absent}\n"), 0600))
			loader := NewEnvLoader(dir, dir, true)
			for _, value := range []string{"first", "second"} {
				if value == "second" {
					rewriteEnvSameMtime(t, target, "VALUE=second\nEXPANDED=${LANG}\nPRIVATE=${DOCKLET_PRIVATE_TEST_VALUE:-absent}\n")
				}
				env, injected, err := loader.LoadEnvironment(t.Context())
				require.NoError(t, err)
				require.Equal(t, value, env["VALUE"])
				require.Equal(t, value, injected["VALUE"])
				require.Equal(t, "example_locale", env["EXPANDED"])
				require.Equal(t, "absent", env["PRIVATE"])
				require.NotContains(t, env, "DOCKLET_PRIVATE_TEST_VALUE")
			}
		})
	}
}

func TestEnvironmentPrimaryFailureSharedByDirectLoadAndSelection(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "warm-same-mtime"}[warm], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".env")
			compose := filepath.Join(dir, "compose.yaml")
			require.NoError(t, os.WriteFile(compose, []byte("services:\n  app:\n    image: registry.example.com/app:1\n"), 0600))
			if warm {
				require.NoError(t, os.WriteFile(path, []byte("VALUE=valid\n"), 0600))
				_, _, err := NewEnvLoader(dir, dir, false).LoadEnvironment(t.Context())
				require.NoError(t, err)
				_, err = ComposeFileEnvSelection(t.Context(), dir, dir)
				require.NoError(t, err)
				rewriteEnvSameMtime(t, path, "VALUE=\"unterminated\n")
			} else {
				require.NoError(t, os.WriteFile(path, []byte("VALUE=\"unterminated\n"), 0600))
			}
			_, _, err := NewEnvLoader(dir, dir, false).LoadEnvironment(t.Context())
			require.ErrorContains(t, err, "parse env")
			_, err = ComposeFileEnvSelection(t.Context(), dir, dir)
			require.ErrorContains(t, err, "parse env")
			_, err = LoadComposeProject(t.Context(), compose, "stored", dir, false, nil, nil, nil, false, nil, nil, nil)
			require.ErrorContains(t, err, "parse env")
			require.NoError(t, os.Remove(path))
			model, err := LoadComposeProject(t.Context(), compose, "stored", dir, false, nil, nil, nil, false, nil, nil, nil)
			require.NoError(t, err)
			require.Equal(t, "stored", model.Name)
		})
	}
}

func TestEnvironmentAuxiliarySkipSelectionAndRepair(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"first.yml", "second.yml"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("services:\n  app:\n    image: registry.example.com/app:1\n"), 0600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_ENV_FILES=valid.env,missing.env,bad.env,../outside.env,directory\nBASE=project\n"), 0600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "valid.env"), []byte("COMPOSE_FILE=first.yml\nVALUE=${BASE}-first\n"), 0600))
	bad := filepath.Join(dir, "bad.env")
	require.NoError(t, os.WriteFile(bad, []byte("VALUE=\"unterminated\n"), 0600))
	for _, value := range []string{"project-first", "repaired"} {
		expectedConfigFiles := []string{filepath.Join(dir, "first.yml")}
		if value == "repaired" {
			rewriteEnvSameMtime(t, bad, "COMPOSE_FILE=second.yml\nVALUE=repaired\n")
			expectedConfigFiles = []string{filepath.Join(dir, "second.yml")}
		}
		env, injected, err := NewEnvLoader(dir, dir, true).LoadEnvironment(t.Context())
		require.NoError(t, err)
		require.Equal(t, value, env["VALUE"])
		require.Equal(t, value, injected["VALUE"])
		opts, err := ParseComposeEnvOptions(dir, env)
		require.NoError(t, err)
		require.Equal(t, expectedConfigFiles, opts.ConfigFiles)
		require.NotContains(t, opts.EnvFiles, filepath.Join(filepath.Dir(dir), "outside.env"))
		selected, err := ComposeFileEnvSelection(t.Context(), dir, dir)
		require.NoError(t, err)
		require.Equal(t, expectedConfigFiles, selected)
	}
	require.NoError(t, os.WriteFile(bad, []byte("COMPOSE_FILE=first.yml:../outside.yml\n"), 0600))
	_, err := ComposeFileEnvSelection(t.Context(), dir, dir)
	require.ErrorIs(t, err, common.ErrComposeFileEnvInvalid)
}

func TestEnvironmentSelectionUsesAllowlistedContext(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("LANG", "base")
	t.Setenv("COMPOSE_FILE", "ambient.yml")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.yml"), []byte("services: {}\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_FILE=${LANG}.yml\n"), 0600))
	selected, err := ComposeFileEnvSelection(t.Context(), dir, dir)
	require.NoError(t, err)
	require.Equal(t, []string{filepath.Join(dir, "base.yml")}, selected)
}

func TestEnvironmentDirectoryRemainsOptionalAndUntouched(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	require.NoError(t, os.Mkdir(env, 0700))
	marker := filepath.Join(env, "keep")
	require.NoError(t, os.WriteFile(marker, []byte("preserved"), 0600))
	_, _, err := NewEnvLoader(dir, dir, false).LoadEnvironment(t.Context())
	require.NoError(t, err)
	selected, err := ComposeFileEnvSelection(t.Context(), dir, dir)
	require.NoError(t, err)
	require.Empty(t, selected)
	content, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Equal(t, "preserved", string(content))
}

func TestEnvironmentExcludedAuxiliaryCannotSupplyValues(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "project")
	require.NoError(t, os.Mkdir(dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "outside.env"), []byte("VALUE=outside\nCOMPOSE_FILE=outside.yml\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_ENV_FILES=../outside.env\nVALUE=inside\n"), 0600))
	env, _, err := NewEnvLoader(root, dir, false).LoadEnvironment(t.Context())
	require.NoError(t, err)
	require.Equal(t, "inside", env["VALUE"])
	require.NotContains(t, env, "COMPOSE_FILE")
	selected, err := ComposeFileEnvSelection(t.Context(), root, dir)
	require.NoError(t, err)
	require.Empty(t, selected)
}
