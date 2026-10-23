//go:build ignore

// Command profile writes the Godot build profile (packaging/engine/freebeams.build)
// that leaves the engine classes FreeBeams does not use out of the custom
// export templates (bin/build-engine).
//
//	go run packaging/engine/profile.go <godot source> packaging/engine/freebeams.build
//
// It follows graphics.gd's own engine/strip_unused_classes (cmd/gd/internal/builder/
// engine_profile.go, MIT, Copyright (C) 2022 Quentin Quaadgras), which follows the
// editor's EditorBuildProfileManager::_detect_from_project: a class is kept when
// the Go code links its graphics.gd/classdb package (graphics.gd looks up the
// method binds of every linked class at startup, so a missing one is an error),
// when its name appears in the project's files or Go sources, when packaging/
// engine/keep.txt names it, or when a kept class needs it (ancestors and
// ADD_CLASS_DEPENDENCY). Only nodes and resources are disabled.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type class struct {
	Inherits string
	Core     bool
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: go run packaging/engine/profile.go <godot source> <out.build>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, "profile:", err)
		os.Exit(1)
	}
}

func run(src, out string) error {
	classes, deps, err := engineClasses(src)
	if err != nil {
		return err
	}
	used, err := classesUsed(classes)
	if err != nil {
		return err
	}
	forced, err := keepList("packaging/engine/keep.txt")
	if err != nil {
		return err
	}
	disabled, dropped := disabledClasses(classes, deps, used, forced)
	data, err := json.MarshalIndent(map[string]any{
		"type":                   "build_profile",
		"disabled_classes":       disabled,
		"disabled_build_options": map[string]any{},
	}, "", "\t")
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("profile: %d of the engine's %d classes left out (%d subtree roots) -> %s\n", dropped, len(classes), len(disabled), out)
	return nil
}

func keepList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var names []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line)
		}
	}
	return names, sc.Err()
}

func engineClasses(src string) (map[string]class, map[string][]string, error) {
	classes, deps := map[string]class{}, map[string][]string{}
	header := regexp.MustCompile(`<class\s+name="([^"]+)"(?:\s+inherits="([^"]+)")?[^>]*?(?:\s+api_type="([^"]+)")?[^>]*>`)
	docs, _ := filepath.Glob(filepath.Join(src, "doc", "classes", "*.xml"))
	modules, _ := filepath.Glob(filepath.Join(src, "modules", "*", "doc_classes", "*.xml"))
	platforms, _ := filepath.Glob(filepath.Join(src, "platform", "*", "doc_classes", "*.xml"))
	for _, path := range slices.Concat(docs, modules, platforms) {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, nil, err
		}
		if m := header.FindSubmatch(data); m != nil {
			classes[string(m[1])] = class{Inherits: string(m[2]), Core: string(m[3]) != "editor"}
		}
	}
	if len(classes) == 0 {
		return nil, nil, fmt.Errorf("no class documentation in %s, is this a Godot source tree?", src)
	}
	binding := regexp.MustCompile(`(?m)^\s*void\s+([A-Za-z0-9_]+)::_bind_methods\(\)|ADD_CLASS_DEPENDENCY\("([^"]+)"\)`)
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == "thirdparty" || n == "bin" || n == ".git" || n == "editor" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".cpp" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), "ADD_CLASS_DEPENDENCY") {
			return err
		}
		var cur string
		for _, m := range binding.FindAllSubmatch(data, -1) {
			if m[1] != nil {
				cur = string(m[1])
			} else if cur != "" {
				deps[cur] = append(deps[cur], string(m[2]))
			}
		}
		return nil
	})
	return classes, deps, err
}

func classesUsed(classes map[string]class) (map[string]bool, error) {
	used := map[string]bool{}
	list, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}} {{.Dir}} {{if .Module}}{{.Module.Main}}{{end}}", "./...").Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	var dirs []string
	for line := range strings.SplitSeq(string(list), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if c, ok := strings.CutPrefix(f[0], "graphics.gd/classdb/"); ok {
			if _, ok := classes[c]; ok {
				used[c] = true
			}
		}
		if len(f) == 3 && f[2] == "true" {
			dirs = append(dirs, f[1])
		}
	}
	outputs := map[string]bool{".so": true, ".dll": true, ".dylib": true, ".exe": true, ".a": true, ".o": true,
		".wasm": true, ".pck": true, ".zip": true, ".icns": true, ".ico": true, ".png": true}
	ident := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	scan := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && path != "graphics" {
				return filepath.SkipDir
			}
			return nil
		}
		if outputs[filepath.Ext(path)] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, n := range ident.FindAll(data, -1) {
			if _, ok := classes[string(n)]; ok {
				used[string(n)] = true
			}
		}
		return nil
	}
	if err := filepath.WalkDir("graphics", scan); err != nil {
		return nil, err
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".go" {
				if err := scan(filepath.Join(dir, e.Name()), e, nil); err != nil {
					return nil, err
				}
			}
		}
	}
	return used, nil
}

func disabledClasses(classes map[string]class, deps map[string][]string, used map[string]bool, forced []string) ([]string, int) {
	// graphics.gd's own list of classes that clash with others when disabled,
	// plus GDExtension: the Go code is loaded through one.
	hardcoded := []string{"Font", "InputEvent", "ShaderInclude", "StyleBox", "Window", "GDExtension"}
	descends := func(c, ancestor string) bool {
		for c != "" {
			if c == ancestor {
				return true
			}
			c = classes[c].Inherits
		}
		return false
	}
	keep := map[string]bool{}
	for c := range used {
		keep[c] = true
	}
	for _, c := range forced {
		keep[c] = true
	}
	for _, c := range hardcoded {
		keep[c] = true
		for other := range classes {
			if descends(other, c) {
				keep[other] = true
			}
		}
	}
	for {
		var more []string
		for c := range keep {
			for _, o := range slices.Concat(deps[c], []string{classes[c].Inherits}) {
				if o != "" && !keep[o] {
					more = append(more, o)
				}
			}
		}
		if len(more) == 0 {
			break
		}
		for _, c := range more {
			keep[c] = true
		}
	}
	var disabled []string
	dropped := 0
	for name, c := range classes {
		if !c.Core || keep[name] || !(descends(name, "Node") || descends(name, "Resource")) {
			continue
		}
		dropped++
		if c.Inherits == "" || keep[c.Inherits] {
			disabled = append(disabled, name)
		}
	}
	slices.Sort(disabled)
	return disabled, dropped
}
