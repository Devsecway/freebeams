package uicore

import (
	"bytes"
	"io"
	"os"
	"strings"
)

// LoadableFontFile reports whether path starts like a font file Godot's
// FreeType can open (TrueType, OpenType, a TrueType collection, WOFF). The
// OS lists families whose file is missing, empty or not yet downloaded;
// drawing a name in such a face logs a FreeType error per draw. macOS's
// reserved system UI fonts (PingFang under FontServices' Reserved folder)
// resolve to a collection FreeType cannot open from there either.
func LoadableFontFile(path string) bool {
	if path == "" || strings.Contains(path, "/PrivateFrameworks/") || strings.Contains(path, "/Reserved/") {
		return false
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	if _, err := io.ReadFull(f, head); err != nil {
		return false
	}
	for _, tag := range [][]byte{{0, 1, 0, 0}, []byte("OTTO"), []byte("ttcf"), []byte("true"), []byte("wOFF"), []byte("wOF2")} {
		if bytes.Equal(head, tag) {
			return true
		}
	}
	return false
}
