package core

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"freebeams/internal/models"
)

// Theme files: a theme shared between computers ("Export Theme…" /
// "Import Theme…"). A .fbtheme file is a zip archive:
//
//	theme.json          {"format": "freebeams-theme", "version": 1, "theme": {…},
//	                     "background": "media/<file>", "poster": "media/<file>",
//	                     "missing": "<original path>"}
//	media/<file>        the image or video background, stored as is
//	media/<file>        a video's poster still, when it has one
//
// The theme's paths point at the archive's own copies. A background that
// could not be read at export is named under "missing" (and the theme keeps
// its original path), so the receiver knows which file to ask for.

// ThemeFileExt is the extension of an exported theme.
const ThemeFileExt = ".fbtheme"

// ThemeFileFormat identifies a theme file's manifest.
const ThemeFileFormat = "freebeams-theme"

type themeManifest struct {
	Format     string       `json:"format"`
	Version    int          `json:"version"`
	Theme      models.Theme `json:"theme"`
	Background string       `json:"background,omitempty"`
	Poster     string       `json:"poster,omitempty"`
	Missing    string       `json:"missing,omitempty"`
}

// ThemeFileReport says what a theme file carried.
type ThemeFileReport struct {
	Background string `json:"background,omitempty"` // the background file's name in the archive / on disk
	Poster     string `json:"poster,omitempty"`
	Missing    string `json:"missing,omitempty"` // a background file that is not in the archive
}

// WithThemeExt adds .fbtheme to name unless it has it.
func WithThemeExt(name string) string {
	if strings.EqualFold(filepath.Ext(name), ThemeFileExt) {
		return name
	}
	return name + ThemeFileExt
}

// themeFileName is a safe file name for a theme ("Clean Dark" -> "Clean Dark").
func themeFileName(name string) string {
	clean := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(name))
	if clean == "" || clean == "." || clean == ".." {
		return "Theme"
	}
	return clean
}

// ThemeFileName is the default file name an exported theme is saved as.
func ThemeFileName(t models.Theme) string { return themeFileName(t.Name) + ThemeFileExt }

// WriteThemeFile writes theme t (with its background files) as a theme file.
func WriteThemeFile(w io.Writer, t models.Theme) (ThemeFileReport, error) {
	var rep ThemeFileReport
	t.ID, t.Custom = 0, false
	m := themeManifest{Format: ThemeFileFormat, Version: 1}
	zw := zip.NewWriter(w)
	bg := &t.Background
	add := func(src string) (string, error) {
		f, err := os.Open(src)
		if err != nil {
			return "", err
		}
		defer f.Close()
		name := "media/" + themeFileName(filepath.Base(src))
		if name == rep.Background { // poster and background with one name
			name = "media/poster-" + themeFileName(filepath.Base(src))
		}
		dst, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(dst, f); err != nil {
			return "", fmt.Errorf("copy %s: %w", src, err)
		}
		return name, nil
	}
	if (bg.Kind == models.BackgroundImage || bg.Kind.Plays()) && bg.Path != "" {
		name, err := add(bg.Path)
		switch {
		case err == nil:
			rep.Background, m.Background = name, name
			bg.Path = name
		case errors.Is(err, os.ErrNotExist):
			rep.Missing, m.Missing = bg.Path, bg.Path
		default:
			return rep, fmt.Errorf("export theme %q: %w", t.Name, err)
		}
	}
	if bg.Kind == models.BackgroundVideo && bg.Poster != "" {
		if name, err := add(bg.Poster); err == nil {
			rep.Poster, m.Poster = name, name
			bg.Poster = name
		} else {
			bg.Poster = "" // a cache file on this computer only
		}
	}
	m.Theme = t
	js, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return rep, fmt.Errorf("export theme %q: %w", t.Name, err)
	}
	mw, err := zw.Create("theme.json")
	if err != nil {
		return rep, fmt.Errorf("export theme %q: %w", t.Name, err)
	}
	if _, err := mw.Write(js); err != nil {
		return rep, fmt.Errorf("export theme %q: %w", t.Name, err)
	}
	if err := zw.Close(); err != nil {
		return rep, fmt.Errorf("export theme %q: %w", t.Name, err)
	}
	return rep, nil
}

// ExportThemeFile writes t to the file at dst (see WriteThemeFile).
func ExportThemeFile(dst string, t models.Theme) (ThemeFileReport, error) {
	f, err := os.Create(dst)
	if err != nil {
		return ThemeFileReport{}, fmt.Errorf("export theme: %w", err)
	}
	rep, err := WriteThemeFile(f, t)
	if cerr := f.Close(); err == nil && cerr != nil {
		err = fmt.Errorf("export theme: %w", cerr)
	}
	if err != nil {
		os.Remove(dst)
	}
	return rep, err
}

// ImportThemeFile reads a theme file. Its background files are extracted
// into mediaDir/<theme name>/ (a fresh folder per import) and the theme's
// paths point there. The theme has ID 0 and the name from the file; the
// caller makes it unique.
func ImportThemeFile(src, mediaDir string) (models.Theme, ThemeFileReport, error) {
	var rep ThemeFileReport
	zr, err := zip.OpenReader(src)
	if err != nil {
		return models.Theme{}, rep, fmt.Errorf("import theme %s: not a theme file: %w", filepath.Base(src), err)
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	mf := files["theme.json"]
	if mf == nil {
		return models.Theme{}, rep, fmt.Errorf("import theme %s: theme.json is missing", filepath.Base(src))
	}
	var m themeManifest
	rc, err := mf.Open()
	if err != nil {
		return models.Theme{}, rep, fmt.Errorf("import theme %s: %w", filepath.Base(src), err)
	}
	err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&m)
	rc.Close()
	if err != nil {
		return models.Theme{}, rep, fmt.Errorf("import theme %s: theme.json: %w", filepath.Base(src), err)
	}
	if m.Format != ThemeFileFormat {
		return models.Theme{}, rep, fmt.Errorf("import theme %s: not a FreeBeams theme (format %q)", filepath.Base(src), m.Format)
	}
	if m.Version > 1 {
		return models.Theme{}, rep, fmt.Errorf("import theme %s: made by a newer FreeBeams (version %d)", filepath.Base(src), m.Version)
	}
	t := m.Theme
	t.ID, t.Custom = 0, false
	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		t.Name = strings.TrimSuffix(filepath.Base(src), filepath.Ext(src))
	}
	rep.Missing = m.Missing
	var dir string
	extract := func(name string) (string, error) {
		f := files[name]
		if f == nil || !strings.HasPrefix(name, "media/") || strings.Contains(name, "..") {
			return "", fmt.Errorf("%s is not in the file", name)
		}
		if dir == "" {
			d, err := uniqueDir(filepath.Join(mediaDir, themeFileName(t.Name)))
			if err != nil {
				return "", err
			}
			dir = d
		}
		dst := filepath.Join(dir, themeFileName(path.Base(name)))
		in, err := f.Open()
		if err != nil {
			return "", err
		}
		defer in.Close()
		out, err := os.Create(dst)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return "", err
		}
		return dst, out.Close()
	}
	bg := &t.Background
	if m.Background != "" {
		p, err := extract(m.Background)
		if err != nil {
			return models.Theme{}, rep, fmt.Errorf("import theme %s: background: %w", filepath.Base(src), err)
		}
		bg.Path, rep.Background = p, p
	}
	if m.Poster != "" {
		if p, err := extract(m.Poster); err == nil {
			bg.Poster, rep.Poster = p, p
		} else {
			bg.Poster = ""
		}
	}
	return t, rep, nil
}

// uniqueDir creates base, or base 2, base 3, … when it exists.
func uniqueDir(base string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		return "", err
	}
	cand := base
	for n := 2; ; n++ {
		err := os.Mkdir(cand, 0o755)
		if err == nil {
			return cand, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
		cand = fmt.Sprintf("%s %d", base, n)
	}
}
