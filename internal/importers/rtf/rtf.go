// Package rtf converts RTF documents (as stored by EasyWorship for song lyrics
// and Bible copyright notices) to plain text. It is a small, forgiving
// tokenizer, not a full RTF reader: formatting is dropped, paragraphs and line
// breaks become "\n", and ignorable destinations (font/color tables, pictures,
// document info, {\*\...} groups, ...) are skipped. Malformed input (unbalanced
// braces, truncated escapes) never panics; it yields best-effort text.
package rtf

import (
	"errors"
	"strings"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"
)

// ErrNotRTF is returned when the input does not start with "{\rtf".
var ErrNotRTF = errors.New("not an RTF document")

// IsRTF reports whether s looks like an RTF document.
func IsRTF(s string) bool {
	return strings.HasPrefix(strings.TrimLeft(s, " \t\r\n\ufeff"), `{\rtf`)
}

// ToText converts an RTF document to plain text. Paragraph ends (\par),
// line breaks (\line), sections, pages and table rows become "\n"; tabs and
// table cells become "\t". Trailing spaces on lines are kept; callers that
// need clean lines should trim them.
func ToText(src string) (string, error) {
	if !IsRTF(src) {
		return "", ErrNotRTF
	}
	p := parser{src: src[strings.Index(src, `{\rtf`):], docCP: charmap.Windows1252}
	p.run()
	return p.out.String(), nil
}

// MustText returns ToText(src), or src itself when it is not RTF. Handy for
// fields that may hold either plain text or RTF.
func MustText(src string) string {
	if t, err := ToText(src); err == nil {
		return t
	}
	return src
}

type destination uint8

const (
	destText    destination = iota // normal body text
	destSkip                       // ignored destination
	destFontTbl                    // font table: parsed for \fcharset, text ignored
)

// state is the per-group parser state; '{' pushes a copy, '}' pops it.
type state struct {
	dest   destination
	uc     int              // \ucN: fallback chars to skip after \uN
	cp     *charmap.Charmap // codepage for \'hh and raw 8-bit bytes
	hidden bool             // \v hidden text
}

type parser struct {
	src   string
	i     int
	out   strings.Builder
	st    state
	stack []state

	docCP     *charmap.Charmap // \ansicpgN (or \mac, \pc, \pca)
	deff      int              // \deffN default font
	fontCS    map[int]int      // font number -> \fcharset
	curFont   int              // font being defined in the font table
	skipChars int              // remaining \uc fallback characters to skip
	highSurr  rune             // pending UTF-16 high surrogate from \uN
}

// maxDepth bounds group nesting so hostile input cannot grow the stack forever.
const maxDepth = 512

func (p *parser) run() {
	p.st = state{dest: destText, uc: 1}
	for p.i < len(p.src) {
		c := p.src[p.i]
		switch c {
		case '{':
			p.i++
			p.skipChars = 0
			if len(p.stack) < maxDepth {
				p.stack = append(p.stack, p.st)
			}
		case '}':
			p.i++
			p.skipChars = 0
			if n := len(p.stack); n > 0 {
				p.st = p.stack[n-1]
				p.stack = p.stack[:n-1]
			}
		case '\\':
			p.i++
			p.control()
		case '\r', '\n':
			p.i++
		default:
			p.i++
			p.text(c)
		}
	}
	p.flushSurrogate()
}

// control parses the token after a backslash.
func (p *parser) control() {
	if p.i >= len(p.src) {
		return
	}
	c := p.src[p.i]
	if !isLetter(c) {
		p.i++
		p.symbol(c)
		return
	}
	start := p.i
	for p.i < len(p.src) && isLetter(p.src[p.i]) && p.i-start < 32 {
		p.i++
	}
	word := p.src[start:p.i]
	param, hasParam := 0, false
	neg := false
	if p.i < len(p.src) && p.src[p.i] == '-' && p.i+1 < len(p.src) && isDigit(p.src[p.i+1]) {
		neg = true
		p.i++
	}
	digits := 0
	for p.i < len(p.src) && isDigit(p.src[p.i]) {
		if digits < 10 {
			param = param*10 + int(p.src[p.i]-'0')
		}
		digits++
		hasParam = true
		p.i++
	}
	if neg {
		param = -param
	}
	if p.i < len(p.src) && p.src[p.i] == ' ' {
		p.i++ // the delimiting space belongs to the control word
	}
	p.word(word, param, hasParam)
}

// symbol handles control symbols (backslash + non-letter).
func (p *parser) symbol(c byte) {
	switch c {
	case '\'':
		b, ok := p.hexByte()
		if !ok {
			return
		}
		if p.consumeFallback() {
			return
		}
		p.emitByte(b)
	case '*':
		// {\*\dest ...}: an optional destination we do not understand. Skip it.
		p.st.dest = destSkip
	case '~':
		p.emitPlain(' ')
	case '_':
		p.emitPlain('‑')
	case '-', ':', '|':
		// optional hyphen, index subentry, formula: no text
	case '\\', '{', '}':
		p.emitPlain(rune(c))
	case '\r', '\n':
		p.emitPlain('\n') // "\<newline>" is a paragraph mark
	case '\t':
		p.emitPlain('\t')
	default:
		// unknown control symbol: ignore
	}
}

func (p *parser) hexByte() (byte, bool) {
	if p.i+2 > len(p.src) {
		p.i = len(p.src)
		return 0, false
	}
	hi, ok1 := hexVal(p.src[p.i])
	lo, ok2 := hexVal(p.src[p.i+1])
	if !ok1 || !ok2 {
		// malformed: drop the escape and continue with the next byte
		return 0, false
	}
	p.i += 2
	return hi<<4 | lo, true
}

// destinations whose content is never body text.
var skipDestinations = map[string]bool{
	"colortbl": true, "stylesheet": true, "info": true, "pict": true, "object": true,
	"header": true, "headerl": true, "headerr": true, "headerf": true,
	"footer": true, "footerl": true, "footerr": true, "footerf": true,
	"footnote": true, "annotation": true, "atnid": true, "atnauthor": true,
	"fldinst": true, "xe": true, "tc": true, "txe": true, "rxe": true,
	"themedata": true, "colorschememapping": true, "datastore": true, "latentstyles": true,
	"listtable": true, "listoverridetable": true, "rsidtbl": true, "generator": true,
	"xmlnstbl": true, "mmathPr": true, "pntext": true, "pntxta": true, "pntxtb": true,
	"listtext": true, "bkmkstart": true, "bkmkend": true, "filetbl": true, "revtbl": true,
	"userprops": true, "nonshppict": true, "shpinst": true, "sp": true, "sn": true, "sv": true,
	"author": true, "operator": true, "title": true, "subject": true, "comment": true,
	"keywords": true, "doccomm": true, "company": true, "category": true, "manager": true,
	"hlinkbase": true, "buptim": true, "creatim": true, "printim": true, "revtim": true,
	"ftnsep": true, "ftnsepc": true, "ftncn": true, "aftnsep": true, "aftnsepc": true, "aftncn": true,
	"pgdsctbl": true, "wgrffmtfilter": true, "fontemb": true, "fontfile": true, "falt": true,
	"panose": true, "template": true, "docvar": true, "background": true, "pn": true,
	"objdata": true, "objclass": true, "objname": true,
}

// word handles a control word.
func (p *parser) word(w string, param int, hasParam bool) {
	if w == "bin" {
		// \binN: N bytes of raw binary data follow.
		if hasParam && param > 0 {
			p.i += min(param, len(p.src)-p.i)
		}
		return
	}
	if p.skipChars > 0 && w != "u" {
		// a control word counts as one fallback character
		p.skipChars--
		return
	}
	if w == "fonttbl" {
		p.st.dest = destFontTbl
		return
	}
	if skipDestinations[w] {
		p.st.dest = destSkip
		return
	}
	if p.st.dest == destFontTbl {
		switch w {
		case "f":
			p.curFont = param
		case "fcharset":
			if p.fontCS == nil {
				p.fontCS = map[int]int{}
			}
			p.fontCS[p.curFont] = param
		}
		return
	}
	switch w {
	// document character set
	case "ansi":
		p.docCP = charmap.Windows1252
	case "mac":
		p.docCP = charmap.Macintosh
	case "pc":
		p.docCP = charmap.CodePage437
	case "pca":
		p.docCP = charmap.CodePage850
	case "ansicpg":
		if cm := codepage(param); cm != nil {
			p.docCP = cm
		}
	case "deff":
		p.deff = param
	case "f":
		p.st.cp = p.fontCodepage(param)
	case "plain":
		p.st.hidden = false
		p.st.cp = p.fontCodepage(p.deff)
	case "uc":
		if hasParam && param >= 0 && param < 16 {
			p.st.uc = param
		}
	case "u":
		r := param
		if r < 0 {
			r += 65536
		}
		p.emitRune(rune(r))
		p.skipChars = p.st.uc
	case "v":
		p.st.hidden = !hasParam || param != 0
	case "par", "line", "sect", "page", "row", "softline":
		p.emitPlain('\n')
	case "tab", "cell":
		p.emitPlain('\t')
	case "emdash":
		p.emitPlain('—')
	case "endash":
		p.emitPlain('–')
	case "emspace", "enspace", "qmspace":
		p.emitPlain(' ')
	case "bullet":
		p.emitPlain('•')
	case "lquote":
		p.emitPlain('‘')
	case "rquote":
		p.emitPlain('’')
	case "ldblquote":
		p.emitPlain('“')
	case "rdblquote":
		p.emitPlain('”')
	case "zwj":
		p.emitPlain('‍')
	case "zwnj":
		p.emitPlain('‌')
	}
}

// fontCodepage returns the codepage for font n (nil = document codepage).
func (p *parser) fontCodepage(n int) *charmap.Charmap {
	cs, ok := p.fontCS[n]
	if !ok {
		return nil
	}
	return charsetCodepage(cs)
}

// text handles a raw byte of body text.
func (p *parser) text(c byte) {
	if p.consumeFallback() {
		return
	}
	if c == '\t' {
		p.emitPlain('\t')
		return
	}
	p.emitByte(c)
}

// consumeFallback skips one \uc fallback character; reports whether it did.
func (p *parser) consumeFallback() bool {
	if p.skipChars > 0 {
		p.skipChars--
		return true
	}
	return false
}

func (p *parser) visible() bool {
	return p.st.dest == destText && !p.st.hidden
}

func (p *parser) emitByte(b byte) {
	if !p.visible() {
		return
	}
	p.flushSurrogate()
	if b < 0x80 {
		p.out.WriteByte(b)
		return
	}
	cp := p.st.cp
	if cp == nil {
		cp = p.docCP
	}
	p.out.WriteRune(cp.DecodeByte(b))
}

func (p *parser) emitPlain(r rune) {
	if p.skipChars > 0 {
		p.skipChars--
		return
	}
	if !p.visible() {
		return
	}
	p.flushSurrogate()
	p.out.WriteRune(r)
}

// emitRune writes a \uN character, joining UTF-16 surrogate pairs.
func (p *parser) emitRune(r rune) {
	if !p.visible() {
		return
	}
	switch {
	case utf16.IsSurrogate(r) && r < 0xDC00: // high surrogate
		p.flushSurrogate()
		p.highSurr = r
	case utf16.IsSurrogate(r): // low surrogate
		if p.highSurr != 0 {
			p.out.WriteRune(utf16.DecodeRune(p.highSurr, r))
			p.highSurr = 0
		} else {
			p.out.WriteRune('�')
		}
	default:
		p.flushSurrogate()
		if r != 0 {
			p.out.WriteRune(r)
		}
	}
}

func (p *parser) flushSurrogate() {
	if p.highSurr != 0 {
		p.out.WriteRune('�')
		p.highSurr = 0
	}
}

// charsetCodepage maps an RTF \fcharset to a single-byte codepage, or nil
// for ANSI/default/symbol and unsupported (multi-byte) charsets.
func charsetCodepage(cs int) *charmap.Charmap {
	switch cs {
	case 77:
		return charmap.Macintosh
	case 161:
		return charmap.Windows1253
	case 162:
		return charmap.Windows1254
	case 163:
		return charmap.Windows1258
	case 177:
		return charmap.Windows1255
	case 178:
		return charmap.Windows1256
	case 186:
		return charmap.Windows1257
	case 204:
		return charmap.Windows1251
	case 222:
		return charmap.Windows874
	case 238:
		return charmap.Windows1250
	case 255:
		return charmap.CodePage437
	}
	return nil
}

// codepage maps an \ansicpgN value to a single-byte codepage (nil if unsupported).
func codepage(n int) *charmap.Charmap {
	switch n {
	case 437:
		return charmap.CodePage437
	case 850:
		return charmap.CodePage850
	case 852:
		return charmap.CodePage852
	case 866:
		return charmap.CodePage866
	case 874:
		return charmap.Windows874
	case 1250:
		return charmap.Windows1250
	case 1251:
		return charmap.Windows1251
	case 1252:
		return charmap.Windows1252
	case 1253:
		return charmap.Windows1253
	case 1254:
		return charmap.Windows1254
	case 1255:
		return charmap.Windows1255
	case 1256:
		return charmap.Windows1256
	case 1257:
		return charmap.Windows1257
	case 1258:
		return charmap.Windows1258
	case 10000:
		return charmap.Macintosh
	case 28591:
		return charmap.ISO8859_1
	case 28592:
		return charmap.ISO8859_2
	case 28605:
		return charmap.ISO8859_15
	}
	return nil
}

func isLetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool  { return c >= '0' && c <= '9' }

func hexVal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
