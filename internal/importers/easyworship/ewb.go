package easyworship

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"freeworship/internal/importers/rtf"
	"freeworship/internal/models"
)

// The .ewb format (EasyWorship 6-8 Bible plugin) is a SQLite database. See
// docs/formats/ewb.md for the full description. In short:
//
//   - header(name, abbrev_name, lang_code, copyright RTF, ...): one row.
//   - books(rowid, name, abbrev_name, alt_name, book_info, verse_info) and
//     streams(rowid, stream): one row each per book, same rowid.
//   - streams.stream is zlib; it inflates to a 2-character prefix followed by
//     the book's verses concatenated as UTF-8 text.
//   - verse_info holds one 8-byte little-endian record per verse:
//     uint32 a = offset<<10 | length   (in characters, after the prefix)
//     uint32 b = bibleID<<25 | book<<18 | chapter<<10 | verse<<2
//   - book_info is chapter count + one byte per chapter (verse counts); it is
//     informational only (wrong in some files), we use verse_info.

const (
	ewbRecordSize   = 8
	ewbStreamPrefix = 2        // characters before the first verse
	maxStreamSize   = 64 << 20 // sanity bound for one inflated book
)

// ewbRecord is one decoded verse_info entry.
type ewbRecord struct {
	Offset, Length       int // in characters, relative to the stream text after the prefix
	Book, Chapter, Verse int
	BibleID              int
}

func decodeVerseInfo(blob []byte) []ewbRecord {
	n := len(blob) / ewbRecordSize
	out := make([]ewbRecord, 0, n)
	for i := 0; i < n; i++ {
		a := binary.LittleEndian.Uint32(blob[i*ewbRecordSize:])
		b := binary.LittleEndian.Uint32(blob[i*ewbRecordSize+4:])
		out = append(out, ewbRecord{
			Offset:  int(a >> 10),
			Length:  int(a & 0x3FF),
			Verse:   int(b>>2) & 0xFF,
			Chapter: int(b>>10) & 0xFF,
			Book:    int(b>>18) & 0x7F,
			BibleID: int(b >> 25),
		})
	}
	return out
}

// inflate decompresses a zlib stream into text runes. Streams are UTF-8; a
// stream that is not valid UTF-8 is decoded as Windows-1252 (older files).
func inflate(stream []byte) ([]rune, error) {
	zr, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		return nil, fmt.Errorf("zlib: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(io.LimitReader(zr, maxStreamSize+1))
	if err != nil {
		return nil, fmt.Errorf("zlib: %w", err)
	}
	if len(raw) > maxStreamSize {
		return nil, errors.New("zlib: stream too large")
	}
	if utf8.Valid(raw) {
		return []rune(string(raw)), nil
	}
	s, err := charmap.Windows1252.NewDecoder().Bytes(raw)
	if err != nil {
		return nil, fmt.Errorf("decode text: %w", err)
	}
	return []rune(string(s)), nil
}

// decodeBook decodes one book. position is the 1-based row position, used as
// the canonical book number when the records do not carry a valid one.
func decodeBook(stream, verseInfo []byte, position int) ([]models.Verse, error) {
	text, err := inflate(stream)
	if err != nil {
		return nil, err
	}
	recs := decodeVerseInfo(verseInfo)
	verses := make([]models.Verse, 0, len(recs))
	prevCh, prevV := 0, 0
	for _, r := range recs {
		book := r.Book
		if book < 1 || book > 66 {
			book = position
		}
		ch, v := r.Chapter, r.Verse
		if ch < 1 || v < 1 {
			continue
		}
		if ch < prevCh || (ch == prevCh && v <= prevV) {
			// Out of order. Seen in HCSB 2 Samuel, where 24:9 is stored as
			// 2:9: when the verse number continues the previous chapter's
			// sequence, keep the previous chapter; otherwise drop it.
			if v != prevV+1 {
				continue
			}
			ch = prevCh
		}
		start := ewbStreamPrefix + r.Offset
		end := start + r.Length
		if r.Length == 0 || start < 0 || end > len(text) {
			continue
		}
		t := cleanVerseText(string(text[start:end]))
		if t == "" {
			continue
		}
		prevCh, prevV = ch, v
		verses = append(verses, models.Verse{Book: book, Chapter: ch, Verse: v, Text: t})
	}
	return verses, nil
}

// cleanVerseText strips the KJV/HCSB italics markers (translator-supplied
// words are wrapped in [brackets]: "darkness [was] upon"), keeping the words,
// and normalises whitespace.
func cleanVerseText(s string) string {
	s = strings.NewReplacer("[", "", "]", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

type ewbHeader struct {
	Name, Abbrev, Lang, Copyright string
}

func readEWBHeader(db *sql.DB) (ewbHeader, error) {
	var name, abbrev, lang, copyright sql.NullString
	err := db.QueryRow(`SELECT name, abbrev_name, lang_code, copyright FROM header LIMIT 1`).
		Scan(&name, &abbrev, &lang, &copyright)
	if err != nil {
		return ewbHeader{}, fmt.Errorf("read header: %w", err)
	}
	h := ewbHeader{
		Name:      strings.TrimSpace(name.String),
		Abbrev:    strings.TrimSpace(abbrev.String),
		Lang:      strings.ToLower(strings.TrimSpace(lang.String)),
		Copyright: cleanNotice(copyright.String),
	}
	if h.Abbrev == "" {
		return ewbHeader{}, errors.New("read header: empty abbreviation")
	}
	return h, nil
}

// cleanNotice converts an RTF (or plain) notice to text with tidy lines.
func cleanNotice(s string) string {
	lines := strings.Split(rtf.MustText(s), "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// readEWB decodes a whole .ewb file.
func readEWB(path string) (models.Bible, []models.Verse, error) {
	db, err := openReadOnly(path)
	if err != nil {
		return models.Bible{}, nil, err
	}
	defer db.Close()
	h, err := readEWBHeader(db)
	if err != nil {
		return models.Bible{}, nil, fmt.Errorf("read bible %s: %w", path, err)
	}
	bible := models.Bible{Abbrev: h.Abbrev, Name: h.Name, Language: h.Lang, Copyright: h.Copyright}
	if isPublicDomain(h.Abbrev, h.Name) {
		// EW's notices are not reliable (the RVA 1909 file carries the
		// Reina-Valera 1960 notice), so state the known status instead.
		bible.Copyright = "Public Domain"
	}

	type bookRow struct {
		rowid     int64
		name      string
		verseInfo []byte
	}
	rows, err := db.Query(`SELECT rowid, name, verse_info FROM books ORDER BY rowid`)
	if err != nil {
		return models.Bible{}, nil, fmt.Errorf("read bible %s: books: %w", path, err)
	}
	var books []bookRow
	for rows.Next() {
		var b bookRow
		var name sql.NullString
		if err := rows.Scan(&b.rowid, &name, &b.verseInfo); err != nil {
			rows.Close()
			return models.Bible{}, nil, fmt.Errorf("read bible %s: books: %w", path, err)
		}
		b.name = name.String
		books = append(books, b)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return models.Bible{}, nil, fmt.Errorf("read bible %s: books: %w", path, err)
	}
	if len(books) == 0 {
		return models.Bible{}, nil, fmt.Errorf("read bible %s: no books", path)
	}

	var verses []models.Verse
	for i, b := range books {
		var stream []byte
		if err := db.QueryRow(`SELECT stream FROM streams WHERE rowid = ?`, b.rowid).Scan(&stream); err != nil {
			return models.Bible{}, nil, fmt.Errorf("read bible %s: book %d (%s) stream: %w", path, b.rowid, b.name, err)
		}
		vs, err := decodeBook(stream, b.verseInfo, i+1)
		if err != nil {
			return models.Bible{}, nil, fmt.Errorf("read bible %s: book %d (%s): %w", path, b.rowid, b.name, err)
		}
		verses = append(verses, vs...)
	}
	return bible, verses, nil
}

// publicDomainAbbrevs are translations known to be in the public domain.
var publicDomainAbbrevs = map[string]bool{
	"KJV": true, "AV": true, "KJVA": true, "AKJV": true,
	"ASV": true, "WEB": true, "WEBBE": true, "WEBUS": true, "YLT": true,
	"DARBY": true, "DBY": true, "BBE": true, "DRA": true, "DRB": true,
	"GNV": true, "GENEVA": true, "WBT": true, "WEBSTER": true,
	"RVA": true, "RV1909": true, "RVA1909": true, "RVR1909": true, "SRV": true,
	"LSG": true, "LUT1912": true, "LUTH1912": true, "ELB1905": true, "SVV": true,
}

// publicDomainNames are lower-case full names of public-domain translations.
var publicDomainNames = map[string]bool{
	"king james version": true, "authorized version": true, "authorised version": true,
	"american standard version": true, "world english bible": true,
	"young's literal translation": true, "darby translation": true, "darby bible": true,
	"bible in basic english": true, "douay-rheims": true, "douay-rheims bible": true,
	"geneva bible": true, "webster bible": true, "webster's bible": true,
	"reina-valera antigua": true, "reina valera antigua": true, "reina-valera 1909": true,
	"reina valera 1909": true, "louis segond 1910": true, "luther 1912": true,
}

// isPublicDomain reports whether a translation is known to be public domain,
// by abbreviation or full name. Unknown translations are treated as
// copyrighted (e.g. HCSB, NIV, NKJV, RVR1960).
func isPublicDomain(abbrev, name string) bool {
	if publicDomainAbbrevs[strings.ToUpper(strings.TrimSpace(abbrev))] {
		return true
	}
	return publicDomainNames[strings.ToLower(strings.Join(strings.Fields(name), " "))]
}
