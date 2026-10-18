package rtf

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.txt golden files")

// TestGolden converts real EasyWorship lyric blobs (public-domain hymns copied
// from SongWords.db) and compares them with the reviewed .txt goldens.
func TestGolden(t *testing.T) {
	files, err := filepath.Glob("testdata/*.rtf")
	if err != nil || len(files) == 0 {
		t.Fatalf("no testdata: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ToText(string(src))
			if err != nil {
				t.Fatalf("ToText: %v", err)
			}
			golden := strings.TrimSuffix(f, ".rtf") + ".txt"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update): %v", err)
			}
			if got != string(want) {
				t.Errorf("ToText(%s) mismatch\n--- got ---\n%s\n--- want ---\n%s", f, got, want)
			}
			for _, bad := range []string{`\`, "{", "}", "Arial", "Verdana", "pnseclvl", "fntnamaut"} {
				if strings.Contains(got, bad) {
					t.Errorf("output contains %q", bad)
				}
			}
		})
	}
}

func TestToText(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", `{\rtf1 Hello world}`, "Hello world"},
		{"par and line", `{\rtf1 a\par b\line c\par}`, "a\nb\nc\n"},
		{"control word space delimiter", `{\rtf1\b bold\b0  text}`, "bold text"},
		{"raw newlines ignored", "{\\rtf1 a\r\nb\nc}", "abc"},
		{"backslash newline is par", "{\\rtf1 a\\\nb}", "a\nb"},
		{"tab", `{\rtf1 a\tab b}`, "a\tb"},
		{"escaped specials", `{\rtf1 \\ \{ \}}`, `\ { }`},
		{"nbsp, optional and nonbreaking hyphen", `{\rtf1 a\~b c\-d e\_f}`, "a\u00A0b cd e\u2011f"},
		{"hex cp1252 default", `{\rtf1\ansi caf\'e9 \'93q\'94 \'92 \'a9}`, "café \u201Cq\u201D \u2019 ©"},
		{"hex uppercase", `{\rtf1 caf\'E9}`, "café"},
		{"ansicpg1251", `{\rtf1\ansi\ansicpg1251 \'cf\'f0\'e8\'e2\'e5\'f2}`, "Привет"},
		{"ansicpg1250", `{\rtf1\ansi\ansicpg1250 \'9a\'9e}`, "šž"},
		{"mac charset", `{\rtf1\mac \'8e}`, "é"},
		{"font charset overrides", `{\rtf1\ansi\deff0{\fonttbl{\f0\fnil\fcharset0 Arial;}{\f1\fnil\fcharset204 Arial Cyr;}}\f0 \'e9 \f1 \'e9\plain  \'e9}`, "é й é"},
		{"unicode with fallback", `{\rtf1 \u233?t\u233?}`, "été"},
		{"unicode negative", `{\rtf1 \u-3913?}`, "\uF0B7"},
		{"uc0", `{\rtf1\uc0 \u233 t}`, "ét"},
		{"uc2 hex fallback", `{\rtf1\uc2 \u8364\'80\'80 x}`, "€ x"},
		{"uc scoped to group", `{\rtf1 {\uc2 \u233 ab}\u233 cd}`, "ééd"},
		{"surrogate pair", `{\rtf1 \u-10179?\u-8704?}`, "😀"},
		{"lone low surrogate", `{\rtf1 \u-8704?x}`, "\uFFFDx"},
		{"lone high surrogate", `{\rtf1 \u-10179?x}`, "\uFFFDx"},
		{"ignorable destination", `{\rtf1 a{\*\generator Riched20;}b}`, "ab"},
		{"unknown ignorable with groups", `{\rtf1 a{\*\foo {x}{\bar y}}b}`, "ab"},
		{"fonttbl colortbl stylesheet", `{\rtf1{\fonttbl{\f0 Times;}}{\colortbl;\red0\green0\blue0;}{\stylesheet{\s0 Normal;}}x}`, "x"},
		{"info", `{\rtf1{\info{\title T}{\author A}}body}`, "body"},
		{"pict with binary", "{\\rtf1 a{\\pict\\bin3 }}}}b}", "ab"},
		{"pict hex", `{\rtf1 a{\pict\pngblip 89504e47}b}`, "ab"},
		{"field result kept", `{\rtf1 {\field{\*\fldinst HYPERLINK "x"}{\fldrslt link}}}`, "link"},
		{"hidden text", `{\rtf1 a{\v hidden}b\v c\v0 d}`, "abd"},
		{"quotes and dashes", `{\rtf1 \lquote a\rquote  \ldblquote b\rdblquote  \emdash \endash \bullet}`, "\u2018a\u2019 \u201Cb\u201D \u2014\u2013\u2022"},
		{"leading whitespace and BOM", "\ufeff  {\\rtf1 x}", "x"},
		{"numeric param negative", `{\rtf1\fi-360 x}`, "x"},
		{"pntext skipped", `{\rtf1{\pntext 1.\tab}item}`, "item"},
		{"listtext skipped", `{\rtf1{\listtext\'95\tab}item}`, "item"},
		{"EW bible notice (Riched20)", "{\\rtf1\\ansi\\ansicpg1252\\deff0\\nouicompat{\\fonttbl{\\f0\\fnil\\fcharset0 Courier New;}}\r\n{\\colortbl ;\\red0\\green0\\blue255;}\r\n{\\*\\generator Riched20 6.3.9600}\\viewkind4\\uc1 \r\n\\pard\\nowidctlpar\\b\\f0\\fs20\\lang1033 RVA BIBLE TEXT\\cf1\\par\r\n\\cf0\\b0 Reina-Valera Antigua\\par\r\n\\par\r\nLa versi\\'f3n \\'a9 B\\'edblicas en Am\\'e9rica.\\lang9\\par\r\n}\r\n", "RVA BIBLE TEXT\nReina-Valera Antigua\n\nLa versión © Bíblicas en América.\n"},
		{"EW style", "{\\rtf1\\ansi\\deff0{\\fonttbl{\\f0\\fnil\\fcharset0 Arial;}}{\\*\\pnseclvl1\\pnucrm\\pnstart1\\pnhang\\pnindent720{\\pntxtb}{\\pntxta{.}}}\n{\\pard\\li0\\fi0\\ri0\\sb0\\sl\\sa0 \\plain\\f1\\fntnamaut Verse 1\\par\n\\li0\\sa0 \\plain\\f1\\fntnamaut Line\\par\n\\li0\\sa0 \\par\n\\li0 \\plain\\f1\\fntnamaut Chorus 1\\par\n}\n}", "Verse 1\nLine\n\nChorus 1\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToText(tc.in)
			if err != nil {
				t.Fatalf("ToText: %v", err)
			}
			if got != tc.want {
				t.Fatalf("ToText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNotRTF(t *testing.T) {
	for _, in := range []string{"", "plain text", "{rtf1 x}", `\rtf1`} {
		if _, err := ToText(in); !errors.Is(err, ErrNotRTF) {
			t.Errorf("ToText(%q) err = %v, want ErrNotRTF", in, err)
		}
	}
	if got := MustText("plain"); got != "plain" {
		t.Errorf("MustText(plain) = %q", got)
	}
	if got := MustText(`{\rtf1 x\par}`); got != "x\n" {
		t.Errorf("MustText(rtf) = %q", got)
	}
}

// TestMalformed: corrupt input must not panic and should keep readable text.
func TestMalformed(t *testing.T) {
	tests := []struct{ in, want string }{
		{`{\rtf1 unclosed`, "unclosed"},
		{`{\rtf1 x}}}} y`, "x y"},
		{`{\rtf1 trailing\`, "trailing"},
		{`{\rtf1 bad hex \'zz ok}`, "bad hex zz ok"},
		{`{\rtf1 short hex \'4`, "short hex "},
		{`{\rtf1 \u`, ""},
		{`{\rtf1 \u99999999999999999999 x}`, "\uFFFD"},
		{`{\rtf1 \bin999999 x}`, ""},
		{`{\rtf1 \bin-5 x}`, "x"},
		{`{\rtf1 \verylongcontrolwordthatkeepsgoingonandonandonforever x}`, "x"},
		{"{\\rtf1 " + strings.Repeat("{", 5000) + "deep" + strings.Repeat("}", 5000) + "}", "deep"},
		{"{\\rtf1 \x00\xff\x80}", "\x00ÿ€"},
	}
	for _, tc := range tests {
		got, err := ToText(tc.in)
		if err != nil {
			t.Errorf("ToText(%q): %v", tc.in, err)
			continue
		}
		if tc.want != "" && !strings.Contains(got, tc.want) {
			t.Errorf("ToText(%q) = %q, want it to contain %q", tc.in, got, tc.want)
		}
	}
}

func FuzzToText(f *testing.F) {
	f.Add(`{\rtf1\ansi{\fonttbl{\f0 Arial;}}\f0 Hello\par world}`)
	f.Add(`{\rtf1 \u-10179?\u-8704? \'e9 {\*\x y} \bin2 ab}`)
	f.Add(`{\rtf1 {{{{`)
	f.Add(`{\rtf1 \uc5\u233 \'`)
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = ToText(s)
	})
}
