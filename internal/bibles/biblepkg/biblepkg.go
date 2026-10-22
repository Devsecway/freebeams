// Package biblepkg reads and writes FreeBeams Bible packages: one
// translation in one file (Ext, ".fbbible"). It is pure Go.
//
// A package is the gzip of a single-file Bible JSON (the layout
// internal/importers/bibletext decodes) whose first line is a header object
// with the package's metadata:
//
//	{"format":"freebeams-bible","version":1,"abbrev":"KJV","name":"King James Version","language":"en","copyright":"Public Domain","verses":31102,
//	"books":{"Genesis":{"1":{"1":"In the beginning God created the heaven and the earth.",…}},
//	…}}
//
// Because the header is the first line, ReadHeader lists a package after
// inflating a few hundred bytes. The bundled translations
// (internal/bibles/bundled) are packages too, so built-ins, the Bibles folder
// and "Save as Package" share this one reader and writer. The verse count in
// the header is checked against the books on Read, and gzip's CRC catches a
// truncated or damaged file.
package biblepkg

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"freebeams/internal/importers/bibletext"
	"freebeams/internal/models"
)

// Ext is the package file extension.
const Ext = ".fbbible"

// Format is the header's "format" value.
const Format = "freebeams-bible"

// Version is the package format version this code writes and the newest it
// reads.
const Version = 1

// maxSize bounds the inflated package (a whole Bible is about 4.5 MB of JSON).
const maxSize = 64 << 20

// maxHeader bounds the header line.
const maxHeader = 64 << 10

// ErrNotPackage is returned for a file that is not a Bible package (not
// gzip, or no FreeBeams header).
var ErrNotPackage = errors.New("not a FreeBeams Bible package")

// Header is a package's metadata (its first line).
type Header struct {
	Format    string `json:"format"`
	Version   int    `json:"version"`
	Abbrev    string `json:"abbrev"`
	Name      string `json:"name,omitempty"`
	Language  string `json:"language,omitempty"` // ISO 639-1
	Copyright string `json:"copyright,omitempty"`
	Verses    int    `json:"verses"`
}

// AsBible is the header as a models.Bible (no ID).
func (h Header) AsBible() models.Bible {
	return models.Bible{Abbrev: h.Abbrev, Name: h.Name, Language: h.Language, Copyright: h.Copyright}
}

// Package is a decoded package.
type Package struct {
	Header
	Bible    models.Bible
	Verses   []models.Verse // canonical order
	Warnings []string       // entries the decoder skipped (none in a package FreeBeams wrote)
}

// Write writes b and verses as a package (gzip, best compression, no
// timestamp: the same input gives the same bytes).
func Write(w io.Writer, b models.Bible, verses []models.Verse) error {
	b.Abbrev = strings.TrimSpace(b.Abbrev)
	if b.Abbrev == "" {
		return errors.New("write bible package: abbrev is required")
	}
	if len(verses) == 0 {
		return fmt.Errorf("write bible package %s: no verses", b.Abbrev)
	}
	hdr, err := json.Marshal(Header{Format: Format, Version: Version, Abbrev: b.Abbrev, Name: b.Name,
		Language: b.Language, Copyright: b.Copyright, Verses: len(verses)})
	if err != nil {
		return fmt.Errorf("write bible package %s: %w", b.Abbrev, err)
	}
	zw, err := gzip.NewWriterLevel(w, gzip.BestCompression)
	if err != nil {
		return fmt.Errorf("write bible package %s: %w", b.Abbrev, err)
	}
	if err := bibletext.EncodeWithHeader(zw, hdr, b, verses); err != nil {
		zw.Close()
		return fmt.Errorf("write bible package: %w", err)
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("write bible package %s: %w", b.Abbrev, err)
	}
	return nil
}

// ReadHeader reads only a package's header line.
func ReadHeader(r io.Reader) (Header, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return Header{}, fmt.Errorf("read bible package: %w", ErrNotPackage)
	}
	defer zr.Close()
	line, err := bufio.NewReaderSize(io.LimitReader(zr, maxHeader), 4096).ReadBytes('\n')
	if errors.Is(err, io.EOF) {
		return Header{}, noHeaderLine(line)
	}
	if err != nil {
		return Header{}, fmt.Errorf("read bible package: header: %w", err)
	}
	return parseHeader(line)
}

// noHeaderLine is the error for data that ends before the header line does:
// a truncated package, or something else.
func noHeaderLine(data []byte) error {
	if bytes.Contains(data, []byte(`"`+Format+`"`)) {
		return errors.New("read bible package: header: truncated")
	}
	return fmt.Errorf("read bible package: %w", ErrNotPackage)
}

// parseHeader decodes the first line: the header object, reopened for
// "books" (`{…,"verses":3,`).
func parseHeader(line []byte) (Header, error) {
	line = bytes.TrimSpace(line)
	if !bytes.HasPrefix(line, []byte("{")) || !bytes.HasSuffix(line, []byte(",")) {
		return Header{}, fmt.Errorf("read bible package: %w", ErrNotPackage)
	}
	obj := append(append([]byte(nil), line[:len(line)-1]...), '}')
	var h Header
	if err := json.Unmarshal(obj, &h); err != nil {
		return Header{}, fmt.Errorf("read bible package: header: %w", ErrNotPackage)
	}
	switch {
	case h.Format != Format:
		return Header{}, fmt.Errorf("read bible package: %w", ErrNotPackage)
	case h.Version < 1:
		return Header{}, fmt.Errorf("read bible package: bad format version %d", h.Version)
	case h.Version > Version:
		return Header{}, fmt.Errorf("read bible package %s: format version %d is newer than this FreeBeams reads (%d)", h.Abbrev, h.Version, Version)
	case strings.TrimSpace(h.Abbrev) == "":
		return Header{}, errors.New(`read bible package: the header has no "abbrev"`)
	case h.Verses < 1:
		return Header{}, fmt.Errorf("read bible package %s: the header counts %d verses", h.Abbrev, h.Verses)
	}
	h.Abbrev = strings.TrimSpace(h.Abbrev)
	return h, nil
}

// Read decodes a whole package and checks it against its header.
func Read(r io.Reader) (Package, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return Package{}, fmt.Errorf("read bible package: %w", ErrNotPackage)
	}
	defer zr.Close()
	data, err := io.ReadAll(io.LimitReader(zr, maxSize+1))
	if err != nil {
		return Package{}, fmt.Errorf("read bible package: damaged or truncated: %w", err)
	}
	if len(data) > maxSize {
		return Package{}, errors.New("read bible package: too large")
	}
	nl := bytes.IndexByte(data, '\n')
	if nl < 0 {
		return Package{}, noHeaderLine(data)
	}
	h, err := parseHeader(data[:nl])
	if err != nil {
		return Package{}, err
	}
	res, err := bibletext.Decode(bytes.NewReader(data))
	if err != nil {
		return Package{}, fmt.Errorf("read bible package %s: %w", h.Abbrev, err)
	}
	if !strings.EqualFold(res.Bible.Abbrev, h.Abbrev) {
		return Package{}, fmt.Errorf("read bible package %s: the books are %s", h.Abbrev, res.Bible.Abbrev)
	}
	if len(res.Verses) != h.Verses {
		return Package{}, fmt.Errorf("read bible package %s: damaged: %d verses, the header says %d", h.Abbrev, len(res.Verses), h.Verses)
	}
	b := h.AsBible()
	return Package{Header: h, Bible: b, Verses: res.Verses, Warnings: res.Warnings}, nil
}

// ReadFile reads the package at path.
func ReadFile(path string) (Package, error) {
	f, err := os.Open(path)
	if err != nil {
		return Package{}, fmt.Errorf("read bible package: %w", err)
	}
	defer f.Close()
	p, err := Read(f)
	if err != nil {
		return Package{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return p, nil
}

// ReadHeaderFile reads the header of the package at path.
func ReadHeaderFile(path string) (Header, error) {
	f, err := os.Open(path)
	if err != nil {
		return Header{}, fmt.Errorf("read bible package: %w", err)
	}
	defer f.Close()
	h, err := ReadHeader(f)
	if err != nil {
		return Header{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return h, nil
}

// WriteFile writes a package to path atomically (a temporary file in the
// same folder, renamed into place), so a reader never sees half a package.
func WriteFile(path string, b models.Bible, verses []models.Verse) error {
	var buf bytes.Buffer
	if err := Write(&buf, b, verses); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("write bible package: %w", err)
	}
	_, err = tmp.Write(buf.Bytes())
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
		return fmt.Errorf("write bible package %s: %w", filepath.Base(path), err)
	}
	return nil
}

// FileName is the package file name for a translation ("KJV.fbbible"):
// the abbreviation with path separators and other unsafe characters
// replaced.
func FileName(abbrev string) string {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r < 32, strings.ContainsRune(`/\:*?"<>|`, r):
			return '_'
		}
		return r
	}, strings.TrimSpace(abbrev))
	clean = strings.Trim(clean, ". ")
	if clean == "" {
		clean = "bible"
	}
	return clean + Ext
}
