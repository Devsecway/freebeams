package easyworship

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"freeworship/internal/models"
	"freeworship/internal/store"
)

// EasyWorship's media library (Media.db, next to Songs.db) lists the stock
// images, motion backgrounds and sounds under Resources/{Images,Videos,Audio}.
// Rows have a media_type (1 video, 2 image, 3 audio; others: feeds, DVDs), a
// title, a `path` placeholder ("<images>", "<videos>", "<audio>"), an
// optional sub `folder`, the file name, the pixel size and, for videos and
// audio, the duration in 100 ns ticks.
const mediaDB = "Media.db"

// EasyWorship media_type values.
const (
	ewMediaVideo = 1
	ewMediaImage = 2
	ewMediaAudio = 3
)

// ewTick is the unit of EasyWorship's duration column (100 ns, as in Windows FILETIME).
const ewTick = 100 * time.Nanosecond

// DefaultResourcesDir returns the platform's EasyWorship Resources directory
// (macOS: /Users/Shared/Softouch/Easyworship/Default/Resources; Windows: the
// same layout under C:\Users\Public\Documents\Softouch). Other platforms return "".
func DefaultResourcesDir() string {
	switch runtime.GOOS {
	case "darwin":
		return "/Users/Shared/Softouch/Easyworship/Default/Resources"
	case "windows":
		return filepath.Join(publicDocuments(), "Softouch", "Easyworship", "Default", "Resources")
	}
	return ""
}

// ReadMedia reads the images and videos registered in Media.db (in dataDir,
// the Databases/Data dir; "" = DefaultSongsDir()) whose files exist under
// resourcesDir ("" = DefaultResourcesDir()), as unsaved library items ordered
// by EasyWorship's row order. Rows of other media types, and rows whose file
// is missing, are left out; the latter are reported as warnings. A missing
// Media.db is not an error: it yields no items.
func ReadMedia(dataDir, resourcesDir string) (items []models.Media, warnings []string, err error) {
	if dataDir == "" {
		dataDir = DefaultSongsDir()
	}
	if resourcesDir == "" {
		resourcesDir = DefaultResourcesDir()
	}
	if dataDir == "" || resourcesDir == "" {
		return nil, nil, nil
	}
	dbPath := filepath.Join(dataDir, mediaDB)
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	db, err := openReadOnly(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("read EasyWorship media: %w", err)
	}
	defer db.Close()
	// No ORDER BY / WHERE on text columns (UTF8_U_CI collation).
	rows, err := db.Query(`SELECT rowid, media_type, title, folder, path, filename, width, height, duration FROM media`)
	if err != nil {
		return nil, nil, fmt.Errorf("read EasyWorship media: %w", err)
	}
	defer rows.Close()
	type row struct {
		id   int64
		item models.Media
	}
	var list []row
	for rows.Next() {
		var (
			id                            int64
			mediaType                     sql.NullInt64
			title, folder, path, filename sql.NullString
			width, height, duration       sql.NullInt64
		)
		if err := rows.Scan(&id, &mediaType, &title, &folder, &path, &filename, &width, &height, &duration); err != nil {
			return nil, nil, fmt.Errorf("read EasyWorship media: %w", err)
		}
		var kind models.MediaKind
		switch mediaType.Int64 {
		case ewMediaVideo:
			kind = models.MediaVideo
		case ewMediaImage:
			kind = models.MediaImage
		case ewMediaAudio:
			kind = models.MediaAudio
		default:
			continue // feeds, DVDs
		}
		file, ok := mediaFile(resourcesDir, path.String, folder.String, filename.String)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("media %q: unknown location %q", title.String, path.String))
			continue
		}
		if fi, err := os.Stat(file); err != nil || !fi.Mode().IsRegular() {
			warnings = append(warnings, fmt.Sprintf("media %q: file not found: %s", title.String, file))
			continue
		}
		if fileKind, ok := models.MediaKindOf(file); !ok || fileKind != kind {
			warnings = append(warnings, fmt.Sprintf("media %q: %s is not a %s file", title.String, filepath.Base(file), kind))
			continue
		}
		m := models.Media{
			Path:   file,
			Kind:   kind,
			Title:  clean(title.String),
			Width:  int(width.Int64),
			Height: int(height.Int64),
		}
		if m.Title == "" {
			m.Title = models.MediaTitle(file)
		}
		if kind.Plays() && duration.Int64 > 0 {
			m.Duration = time.Duration(duration.Int64) * ewTick
		}
		list = append(list, row{id: id, item: m})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("read EasyWorship media: %w", err)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].id < list[j].id })
	items = make([]models.Media, 0, len(list))
	for _, r := range list {
		items = append(items, r.item)
	}
	return items, warnings, nil
}

// mediaFile resolves a Media.db row to a file under resourcesDir. The path
// column is a placeholder for a Resources subfolder; an absolute path is used
// as is (user-added media).
func mediaFile(resourcesDir, path, folder, filename string) (string, bool) {
	if strings.TrimSpace(filename) == "" {
		return "", false
	}
	var base string
	switch strings.ToLower(strings.TrimSpace(path)) {
	case "<images>":
		base = filepath.Join(resourcesDir, "Images")
	case "<videos>":
		base = filepath.Join(resourcesDir, "Videos")
	case "<audio>":
		base = filepath.Join(resourcesDir, "Audio")
	case "":
		return "", false
	default:
		if !filepath.IsAbs(path) {
			return "", false
		}
		base = path
	}
	if f := strings.Trim(strings.TrimSpace(folder), `/\`); f != "" {
		base = filepath.Join(base, filepath.FromSlash(strings.ReplaceAll(f, `\`, "/")))
	}
	return filepath.Join(base, filename), true
}

// ImportMedia registers EasyWorship's images and videos in st. It is
// idempotent: files already in the library (by path) are skipped. It does
// not make thumbnails or transcodes; the app derives those in the background.
// Problems reading EasyWorship data become Warnings; only store failures
// (and cancellation) return an error.
func ImportMedia(ctx context.Context, st *store.Store, opt Options) (Report, error) {
	var rep Report
	if st == nil {
		return rep, errors.New("import EasyWorship media: nil store")
	}
	return rep, importMedia(ctx, st, opt, &rep)
}

func importMedia(ctx context.Context, st *store.Store, opt Options, rep *Report) error {
	items, warnings, err := ReadMedia(opt.SongsDir, opt.ResourcesDir)
	rep.Warnings = append(rep.Warnings, warnings...)
	if err != nil {
		rep.Warnings = append(rep.Warnings, err.Error())
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	existing, err := st.SearchMedia(ctx, "", "", 0)
	if err != nil {
		return fmt.Errorf("import EasyWorship media: %w", err)
	}
	have := make(map[string]bool, len(existing))
	for _, m := range existing {
		have[m.Path] = true
	}
	var batch []*models.Media
	for i := range items {
		m := &items[i]
		if have[m.Path] {
			rep.MediaSkipped++
			continue
		}
		have[m.Path] = true
		batch = append(batch, m)
	}
	if len(batch) == 0 {
		return nil
	}
	if err := st.SaveMediaItems(ctx, batch); err != nil {
		return fmt.Errorf("import EasyWorship media: %w", err)
	}
	rep.MediaImported = len(batch)
	return nil
}
