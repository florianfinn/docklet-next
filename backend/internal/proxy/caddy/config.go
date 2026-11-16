// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Caddy command, global-option and import graph inventory.

package caddy

import (
	"context"
	"errors"
	"path"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
)

type directive struct {
	words                   []string
	depth, line, start, end int
}

// scan retains byte spans for import replacement. Braces within placeholders and
// quoted tokens are ordinary token data, not block delimiters.
func scan(text string) ([]directive, error) {
	out := []directive{}
	depth := 0
	offset := 0
	for n, line := range strings.SplitAfter(text, "\n") {
		words := []string{}
		var b strings.Builder
		quote := rune(0)
		escaped := false
		flush := func() {
			if b.Len() > 0 {
				words = append(words, b.String())
				b.Reset()
			}
		}
		for _, c := range strings.TrimSuffix(line, "\n") {
			if escaped {
				b.WriteRune(c)
				escaped = false
				continue
			}
			if quote != 0 {
				if c == quote {
					quote = 0
				} else if c == '\\' && quote == '"' {
					escaped = true
				} else {
					b.WriteRune(c)
				}
				continue
			}
			if c == '#' {
				break
			}
			if c == '"' || c == '`' {
				quote = c
				continue
			}
			if c == ' ' || c == '\t' || c == '\r' {
				flush()
			} else {
				b.WriteRune(c)
			}
		}
		if quote != 0 || escaped {
			return nil, problem(dto.InvalidGraph, "", n+1, "unterminated quoted token")
		}
		flush()
		if len(words) > 0 {
			out = append(out, directive{words: words, depth: depth, line: n + 1, start: offset, end: offset + len(line)})
			for _, w := range words {
				if w == "{" {
					depth++
				}
				if w == "}" {
					depth--
					if depth < 0 {
						return nil, problem(dto.InvalidGraph, "", n+1, "unbalanced block")
					}
				}
			}
		}
		offset += len(line)
	}
	if depth != 0 {
		return nil, problem(dto.InvalidGraph, "", 0, "unbalanced block")
	}
	return out, nil
}
func problem(code dto.Reason, p string, line int, detail string) *dto.Problem {
	return &dto.Problem{Status: 422, Code: code, Path: p, Line: line, Detail: detail}
}
func fileKey(i dto.Instance, abs string) string {
	prefix := strings.TrimSuffix(i.Source.MountTarget, "/") + "/"
	if strings.HasPrefix(abs, prefix) {
		return strings.TrimPrefix(abs, prefix)
	}
	return abs
}
func absolute(i dto.Instance, p string) string {
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Join(i.Source.MountTarget, p)
}
func refused(code dto.Reason, p string) dto.Eligibility {
	return dto.Eligibility{Reason: code, Path: p}
}
func command(c container.InspectResponse) (string, bool, error) {
	args := []string{}
	if c.Config != nil {
		args = append(slices.Clone(c.Config.Entrypoint), c.Config.Cmd...)
	}
	if len(args) == 0 {
		return "", false, problem(dto.IncompleteInventory, "", 0, "missing container command")
	}
	// Caddy without --config reads ./Caddyfile from the working directory or runs empty,
	// and without --adapter guesses from the file name; both are refused, never assumed.
	root := ""
	adapter := false
	resume := false
	run := false
	for n := 0; n < len(args); n++ {
		arg := args[n]
		if arg == "run" {
			run = true
			continue
		}
		if !run {
			continue
		}
		key, value, eq := strings.Cut(arg, "=")
		switch key {
		case "--config", "--adapter":
			if !eq {
				n++
				var ok bool
				value, ok = argument(args, n)
				if !ok {
					return "", false, problem(dto.IncompleteInventory, "", 0, "missing command value")
				}
			}
			if key == "--config" {
				root = value
			} else if value != "caddyfile" {
				return "", false, problem(dto.UnsupportedFormat, "", 0, "Caddyfile adapter required")
			} else {
				adapter = true
			}
		case "--resume":
			if eq && value != "true" && value != "false" {
				return "", false, problem(dto.IncompleteInventory, "", 0, "invalid resume flag")
			}
			resume = !eq || value == "true"
		case "--watch", "--environ":
		default:
			return "", false, problem(dto.IncompleteInventory, "", 0, "unsupported Caddy run option")
		}
	}
	if !run || !path.IsAbs(root) || path.Clean(root) != root || strings.ContainsAny(root, "\x00\r\n") {
		return "", false, problem(dto.IncompleteInventory, "", 0, "absolute Caddyfile path required")
	}
	// Like Caddy's own inference, a .json extension means JSON even after a Caddyfile prefix.
	if !adapter && (!strings.HasPrefix(path.Base(root), "Caddyfile") || strings.EqualFold(path.Ext(root), ".json")) {
		return "", false, problem(dto.UnsupportedFormat, root, 0, "Caddyfile adapter required")
	}
	return root, resume, nil
}

type graph struct {
	inventory dto.Inventory
	files     map[string]dto.File
	imports   map[string][]directive
	opaque    []dto.OpaqueFile
	root      string
	routes    string
}

func (e *engine) load(ctx context.Context, i dto.Instance) (graph, error) {
	g := graph{files: map[string]dto.File{}, imports: map[string][]directive{}, opaque: []dto.OpaqueFile{}}
	inv := dto.Inventory{Generation: i.Generation, Identity: i.Identity, Eligibility: i.Eligibility, EntryPoints: []dto.EntryPoint{}, Resolvers: []string{}, Providers: []string{}, Plugins: []string{}, Imports: []string{}, PersistConfig: true, AdminAddress: "localhost:2019"}
	g.inventory = inv
	c, err := e.runtime.InspectContainer(ctx, i)
	if err != nil {
		return g, err
	}
	root, resume, err := command(c)
	if err != nil {
		return g, err
	}
	g.root = root
	g.routes = absolute(i, i.RouteDirectory)
	if i.RouteDirectory == "" || path.Clean(i.RouteDirectory) != i.RouteDirectory || strings.ContainsAny(i.RouteDirectory, "\x00\r\n") {
		return g, problem(dto.InvalidInput, i.RouteDirectory, 0, "invalid route directory")
	}
	inv.RootFile = root
	inv.Resume = resume
	inv.ImportLine = "import " + importToken(path.Join(g.routes, "*.caddy"))
	inv.Eligibility = dto.Eligibility{Writable: true}
	if i.Source.ReadOnly || i.Source.Kind == dto.SingleFileSource || i.Source.Kind == dto.UnknownSourceKind {
		inv.Eligibility = refused(dto.SourceReadOnly, i.Source.MountTarget)
	}
	if i.RootFile != "" && absolute(i, i.RootFile) != root {
		inv.Eligibility = refused(dto.IncompleteInventory, root)
	}
	// A route directory must resolve through a directory mount. Source() performs
	// the actual stat, bind policy and read-only checks at the Docker seam.
	if g.routes != i.Source.MountTarget && !hostio.Relative(fileKey(i, g.routes)) {
		inv.Eligibility = refused(dto.SourceReadOnly, g.routes)
	}
	mounted := false
	for _, m := range c.Mounts {
		if g.routes == m.Destination || strings.HasPrefix(g.routes, strings.TrimSuffix(m.Destination, "/")+"/") {
			s, er := e.runtime.Source(ctx, c, m)
			if er != nil {
				return g, er
			}
			if s.Kind != dto.SingleFileSource && s.Kind != dto.UnknownSourceKind {
				mounted = true
			}
			if strings.HasPrefix(m.Destination, strings.TrimSuffix(i.Source.MountTarget, "/")+"/") && m.Source != path.Join(i.Source.Canonical, fileKey(i, m.Destination)) {
				inv.Eligibility = refused(dto.SourceReadOnly, g.routes)
			}
			if s.ReadOnly {
				inv.Eligibility = refused(s.Reason, g.routes)
			}
		}
	}
	if !mounted {
		inv.Eligibility = refused(dto.ImportMissing, g.routes)
	}
	g.inventory = inv
	total := 0
	visiting := map[string]bool{}
	var visit func(string, int) error
	visit = func(p string, depth int) error {
		if depth > hostio.MaxDepth {
			return problem(dto.InputLimit, p, 0, "import depth")
		}
		if visiting[p] {
			return problem(dto.InvalidGraph, p, 0, "cyclic import")
		}
		if _, ok := g.files[p]; ok {
			return nil
		}
		if hostio.Secret(p) {
			g.opaque = append(g.opaque, dto.OpaqueFile{Path: fileKey(i, p)})
			g.inventory.Eligibility = refused(dto.OpaqueInput, p)
			return nil
		}
		fs, opaque, er := e.runtime.Copy(ctx, i, p, true)
		if er != nil {
			return er
		}
		if len(opaque) > 0 {
			g.opaque = append(g.opaque, opaque...)
			g.inventory.Eligibility = refused(dto.OpaqueInput, p)
			return nil
		}
		if len(fs) != 1 {
			return problem(dto.IncompleteInventory, p, 0, "missing imported file")
		}
		f := fs[0]
		f.Path = fileKey(i, p)
		total += len(f.Content)
		if len(g.files) >= hostio.MaxFiles || total > hostio.MaxTotalBytes {
			return problem(dto.InputLimit, p, 0, "graph size")
		}
		g.files[p] = f
		visiting[p] = true
		defer delete(visiting, p)
		directives, er := scan(f.Content)
		if er != nil {
			var pr *dto.Problem
			if errors.As(er, &pr) {
				pr.Path = p
			}
			return er
		}
		for _, d := range directives {
			if d.words[0] != "import" {
				continue
			}
			if len(d.words) != 2 {
				g.inventory.Eligibility = refused(dto.UnsupportedFormat, p)
				continue
			}
			value := d.words[1]
			resolved := value
			if !path.IsAbs(value) {
				resolved = path.Join(path.Dir(p), value)
				if resolved != path.Join(g.routes, "*.caddy") {
					g.inventory.Eligibility = refused(dto.RelativeImport, p)
					continue
				}
			}
			if strings.ContainsAny(value, "{}\x00\r\n") {
				g.inventory.Eligibility = refused(dto.IncompleteInventory, p)
				continue
			}
			if p == root {
				g.inventory.Imports = append(g.inventory.Imports, value)
				if d.depth == 0 && resolved == path.Join(g.routes, "*.caddy") {
					g.inventory.ImportPresent = true
				}
			}
			g.imports[p] = append(g.imports[p], d)
			matches, opaque, er := e.expand(ctx, i, resolved)
			if er != nil {
				if resolved == path.Join(g.routes, "*.caddy") && errdefs.IsNotFound(er) {
					g.inventory.Eligibility = refused(dto.ImportMissing, g.routes)
					continue
				}
				return er
			}
			if len(opaque) > 0 {
				g.opaque = append(g.opaque, opaque...)
				g.inventory.Eligibility = refused(dto.OpaqueInput, p)
			}
			for _, match := range matches {
				if er := visit(match, depth+1); er != nil {
					return er
				}
			}
		}
		return nil
	}
	if err = visit(root, 0); err != nil {
		return g, err
	}
	// Read orphaned owned files as well: setup must offer insertion again after an
	// administrator removes the import, rather than hiding those routes.
	routeFiles, opaque, err := e.expand(ctx, i, path.Join(g.routes, "*.caddy"))
	if err != nil {
		if errdefs.IsNotFound(err) {
			g.inventory.Eligibility = refused(dto.ImportMissing, g.routes)
		} else {
			return g, err
		}
	}
	if len(opaque) > 0 {
		g.opaque = append(g.opaque, opaque...)
		g.inventory.Eligibility = refused(dto.OpaqueInput, g.routes)
	}
	for _, p := range routeFiles {
		if err := visit(p, 1); err != nil {
			return g, err
		}
	}
	optionsText, err := g.optionsText()
	if err != nil {
		return g, err
	}
	directives, err := scan(optionsText)
	if err != nil {
		return g, err
	}
	global := false
	for _, d := range directives {
		if d.depth == 0 {
			global = len(d.words) == 1 && d.words[0] == "{"
			continue
		}
		if !global || d.depth != 1 {
			continue
		}
		if (d.words[0] == "admin" || d.words[0] == "persist_config") && strings.ContainsAny(strings.Join(d.words[1:], " "), "{}") {
			g.inventory.Eligibility = refused(dto.IncompleteInventory, root)
			continue
		}
		if len(d.words) == 2 && d.words[0] == "admin" {
			if d.words[1] == "off" {
				g.inventory.AdminOff = true
				g.inventory.AdminAddress = ""
			} else {
				g.inventory.AdminAddress = d.words[1]
			}
		}
		if len(d.words) == 2 && d.words[0] == "persist_config" && d.words[1] == "off" {
			g.inventory.PersistConfig = false
		}
	}
	if resume && !g.inventory.PersistConfig {
		g.inventory.Eligibility = refused(dto.PersistenceOff, root)
	}
	// --resume needs the running configuration to compare with the Caddyfile; with
	// admin off it cannot be read, so the instance is read-only (AUTO-3).
	if resume && g.inventory.AdminOff && g.inventory.Eligibility.Writable {
		g.inventory.Eligibility = refused(dto.AdminUnavailable, root)
	}
	if !g.inventory.ImportPresent && g.inventory.Eligibility.Writable {
		g.inventory.Eligibility = refused(dto.ImportMissing, root)
		// The framework admits the root operation through the same source lock.
		// Prepare still refuses every route operation until the import is present.
		g.inventory.Eligibility.Writable = e.rootWritable(ctx, i, c, root)
	}
	seenOpaque := map[string]bool{}
	opaqueFiles := []dto.OpaqueFile{}
	for _, f := range g.opaque {
		f.Path = fileKey(i, f.Path)
		if !seenOpaque[f.Path] {
			seenOpaque[f.Path] = true
			opaqueFiles = append(opaqueFiles, f)
		}
	}
	sort.Slice(opaqueFiles, func(a, b int) bool { return opaqueFiles[a].Path < opaqueFiles[b].Path })
	g.opaque = opaqueFiles
	return g, nil
}
func (e *engine) expand(ctx context.Context, i dto.Instance, pattern string) ([]string, []dto.OpaqueFile, error) {
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, nil, problem(dto.InvalidGraph, pattern, 0, err.Error())
	}
	if !strings.ContainsAny(pattern, "*?[") {
		return []string{pattern}, nil, nil
	}
	dir := path.Dir(pattern)
	if strings.ContainsAny(dir, "*?[") {
		return nil, nil, problem(dto.UnsupportedFormat, pattern, 0, "glob directory is unsupported")
	}
	fs, opaque, err := e.runtime.Copy(ctx, i, dir, false)
	if err != nil {
		return nil, nil, err
	}
	out := []string{}
	secrets := []dto.OpaqueFile{}
	for _, f := range fs {
		p := path.Join(dir, f.Path)
		matches, er := path.Match(pattern, p)
		if er != nil {
			return nil, nil, er
		}
		if matches {
			out = append(out, p)
		}
	}
	for _, f := range opaque {
		p := path.Join(dir, f.Path)
		matches, er := path.Match(pattern, p)
		if er != nil {
			return nil, nil, er
		}
		if matches {
			secrets = append(secrets, dto.OpaqueFile{Path: fileKey(i, p)})
		}
	}
	sort.Strings(out)
	return out, secrets, nil
}

func (e *engine) rootWritable(ctx context.Context, i dto.Instance, c container.InspectResponse, root string) bool {
	if !hostio.Relative(fileKey(i, root)) {
		return false
	}
	for _, m := range c.Mounts {
		if m.Destination == root {
			return false
		}
		if strings.HasPrefix(root, strings.TrimSuffix(m.Destination, "/")+"/") && strings.HasPrefix(m.Destination, strings.TrimSuffix(i.Source.MountTarget, "/")+"/") {
			if !m.RW || m.Source != path.Join(i.Source.Canonical, fileKey(i, m.Destination)) {
				return false
			}
		}
	}
	return !i.Source.ReadOnly && i.Source.Kind != dto.SingleFileSource && i.Source.Kind != dto.UnknownSourceKind
}

func argument(args []string, index int) (string, bool) {
	for n, arg := range args {
		if n == index {
			return arg, true
		}
	}
	return "", false
}
func importToken(value string) string {
	if token.MatchString(value) {
		return value
	}
	return strconv.Quote(value)
}

// optionsText expands only already-consumed readable imports, retaining their
// lexical scope. Global options may live in an absolute import before the sites.
func (g graph) optionsText() (string, error) {
	paths := []string{}
	for p := range g.files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	total := 0
	var expand func(string, int) (string, error)
	expand = func(p string, depth int) (string, error) {
		text := g.files[p].Content
		total += len(text)
		if depth > hostio.MaxDepth || total > hostio.MaxTotalBytes {
			return "", problem(dto.InputLimit, p, 0, "expanded import graph")
		}
		ds := g.imports[p]
		for n := len(ds) - 1; n >= 0; n-- {
			d := ds[n]
			pattern := d.words[1]
			if !path.IsAbs(pattern) {
				pattern = path.Join(path.Dir(p), pattern)
			}
			var replacement strings.Builder
			for _, other := range paths {
				matches, err := path.Match(pattern, other)
				if err != nil {
					return "", err
				}
				if !matches {
					continue
				}
				part, err := expand(other, depth+1)
				if err != nil {
					return "", err
				}
				replacement.WriteString(part)
				if !strings.HasSuffix(part, "\n") {
					replacement.WriteByte('\n')
				}
			}
			text = text[:d.start] + replacement.String() + text[d.end:]
		}
		return text, nil
	}
	return expand(g.root, 0)
}
