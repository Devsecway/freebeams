package app

import (
	"os"
	"path/filepath"

	"freebeams/internal/app/core"
	"freebeams/internal/appdirs"
)

// defaultLogoDir materialises the built-in Logo screen (core.WriteDefaultLogo)
// under the cache dir (<cache>/logo, or <FREEBEAMS_CACHE>/logo) and returns
// that directory, which the settings code hands to core.LogoBackground as the
// logo directory. When it cannot be written the error is reported and "" is
// returned, which keeps the gradient as the default logo.
func (a *App) defaultLogoDir() string {
	base := os.Getenv(core.CacheEnv)
	if base == "" {
		var err error
		if base, err = appdirs.Cache(); err != nil {
			a.fail("logo screen", err)
			return ""
		}
	}
	dir, err := core.WriteDefaultLogo(filepath.Join(base, "logo"))
	if err != nil {
		a.fail("logo screen", err)
		return ""
	}
	return dir
}
