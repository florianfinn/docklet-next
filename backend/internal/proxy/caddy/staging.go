// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original staged Caddy graph validation through the shared writer.

package caddy

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

func commandError(res hostio.ExecResult, err error, p string) error {
	// ExecRunning must survive: the journal retains the source lock and must not
	// restore while an exec might still apply configuration.
	if err != nil && reason(err, "") != dto.ExecFailed {
		return err
	}
	if res.ExitCode != 0 || err != nil {
		return problem(dto.InvalidGraph, p, 0, strings.TrimSpace(string(res.Stderr)+"\n"+string(res.Stdout)))
	}
	return nil
}
func reason(err error, fallback dto.Reason) dto.Reason {
	var p *dto.Problem
	if errors.As(err, &p) {
		return p.Code
	}
	return fallback
}
func (e *engine) Validate(ctx context.Context, i proxy.Intent) error {
	g, err := e.load(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	if len(g.opaque) > 0 {
		return problem(dto.OpaqueInput, g.root, 0, "secret-named configuration cannot be staged")
	}
	if !g.inventory.Eligibility.Writable {
		return problem(g.inventory.Eligibility.Reason, g.inventory.Eligibility.Path, 0, "graph became read-only")
	}
	target := absolute(i.Request.Instance, i.Request.Target)
	if i.New.Present {
		g.files[target] = dto.File{Content: string(i.New.Content), Mode: i.Mode}
	} else {
		delete(g.files, target)
	}
	if !hostio.Relative(i.StagingPath) || !strings.HasPrefix(path.Base(i.StagingPath), ".docklet-staging-") {
		return problem(dto.InvalidInput, i.StagingPath, 0, "invalid staging path")
	}
	w, err := e.runtime.Writer(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	if err = w.Mkdir(ctx, i.StagingPath); err != nil {
		return err
	}
	paths := []string{}
	for p := range g.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	names := map[string]string{}
	for n, p := range paths {
		names[p] = path.Join(i.StagingPath, fmt.Sprintf("file-%03d.caddy", n))
	}
	for _, p := range paths {
		text := g.files[p].Content
		ds, err := scan(text)
		if err != nil {
			return err
		}
		// Replace backwards so byte spans always refer to the original bytes. Every
		// matched import expands to staged files; no live candidate is validated.
		for n := len(ds) - 1; n >= 0; n-- {
			d := ds[n]
			if d.words[0] != "import" {
				continue
			}
			if len(d.words) != 2 {
				return problem(dto.UnsupportedFormat, p, d.line, "import arguments cannot be staged")
			}
			pattern := d.words[1]
			if !path.IsAbs(pattern) {
				pattern = path.Join(path.Dir(p), pattern)
			}
			replacements := []string{}
			for _, other := range paths {
				match, err := path.Match(pattern, other)
				if err != nil {
					return err
				}
				if match {
					replacements = append(replacements, "import "+importToken(absolute(i.Request.Instance, names[other]))+"\n")
				}
			}
			replacement := "# empty staged import\n"
			if len(replacements) > 0 {
				replacement = strings.Join(replacements, "")
			}
			text = text[:d.start] + replacement + text[d.end:]
		}
		if err = w.Write(ctx, names[p], []byte(text), 0644); err != nil {
			return err
		}
	}
	root := absolute(i.Request.Instance, names[g.root])
	res, err := e.runtime.Exec(ctx, i.Request.Instance, "validate", root, "", "")
	return commandError(res, err, g.root)
}
