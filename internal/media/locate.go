package media

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// FFmpegDirEnv names a folder holding ffmpeg and ffprobe to use before any
// other (a test of the bundled build, or a build of the user's choice).
const FFmpegDirEnv = "FREEBEAMS_FFMPEG_DIR"

// Locate finds ffmpeg and ffprobe without running them: in $FREEBEAMS_FFMPEG_DIR
// when set, then the copy bundled next to the executable (the Windows zip and
// installer, the Linux tar.gz and the AppImage's usr/bin ship an LGPL build
// there), then on PATH. It only stats files, so it is cheap enough for
// startup; Detect adds the encoder check.
func Locate() Tools {
	var dirs []string
	if d := os.Getenv(FFmpegDirEnv); d != "" {
		dirs = append(dirs, d)
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		if r, err := filepath.EvalSymlinks(exe); err == nil {
			exe = r
		}
		dirs = append(dirs, filepath.Dir(exe))
	}
	return locate(dirs, exec.LookPath)
}

// locate is Locate with the folders to try first and the PATH lookup
// injected (tests). Each tool is looked up on its own.
func locate(dirs []string, lookPath func(string) (string, error)) Tools {
	find := func(name string) string {
		for _, dir := range dirs {
			p := filepath.Join(dir, exeName(name))
			if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
				return p
			}
		}
		if p, err := lookPath(name); err == nil {
			return p
		}
		return ""
	}
	return Tools{FFmpeg: find("ffmpeg"), FFprobe: find("ffprobe")}
}

// exeName adds the platform's executable suffix.
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}
