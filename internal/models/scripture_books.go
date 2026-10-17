package models

import (
	"fmt"
	"sort"
	"strings"
)

// Books is the 66-book Protestant canon in order (Books[0] is Genesis).
//
// Aliases are written in natural form ("1 sam", "song of songs"); matching
// ignores case, spaces and periods, and ordinal prefixes ("I", "II", "III",
// "First", "Second", "Third", "1st", "2nd", "3rd") are read as 1/2/3, so the
// aliases only need the digit form.
var Books = []Book{
	{1, "Genesis", "Gen", []string{"gen", "gn", "ge"}},
	{2, "Exodus", "Exod", []string{"exod", "ex", "exo", "exd"}},
	{3, "Leviticus", "Lev", []string{"lev", "lv", "le"}},
	{4, "Numbers", "Num", []string{"num", "nm", "nu", "nb", "numb"}},
	{5, "Deuteronomy", "Deut", []string{"deut", "dt", "de", "deu"}},
	{6, "Joshua", "Josh", []string{"josh", "jos", "jsh"}},
	{7, "Judges", "Judg", []string{"judg", "jdg", "jg", "jdgs"}},
	{8, "Ruth", "Ruth", []string{"ruth", "ru", "rth"}},
	{9, "1 Samuel", "1 Sam", []string{"1 sam", "1 sa", "1 sm", "1 s"}},
	{10, "2 Samuel", "2 Sam", []string{"2 sam", "2 sa", "2 sm", "2 s"}},
	{11, "1 Kings", "1 Kgs", []string{"1 kgs", "1 ki", "1 kin", "1 kg", "1 k"}},
	{12, "2 Kings", "2 Kgs", []string{"2 kgs", "2 ki", "2 kin", "2 kg", "2 k"}},
	{13, "1 Chronicles", "1 Chr", []string{"1 chr", "1 ch", "1 chron", "1 chro"}},
	{14, "2 Chronicles", "2 Chr", []string{"2 chr", "2 ch", "2 chron", "2 chro"}},
	{15, "Ezra", "Ezra", []string{"ezra", "ezr"}},
	{16, "Nehemiah", "Neh", []string{"neh", "ne"}},
	{17, "Esther", "Esth", []string{"esth", "est", "es"}},
	{18, "Job", "Job", []string{"job", "jb"}},
	{19, "Psalms", "Ps", []string{"ps", "psa", "psm", "pss", "psalm", "pslm"}},
	{20, "Proverbs", "Prov", []string{"prov", "pr", "prv", "pro"}},
	{21, "Ecclesiastes", "Eccl", []string{"eccl", "ecc", "ec", "eccles", "qoh", "qoheleth"}},
	{22, "Song of Solomon", "Song", []string{"song", "sos", "ss", "sng", "song of songs", "song of sol", "canticles", "canticle of canticles", "cant"}},
	{23, "Isaiah", "Isa", []string{"isa", "is"}},
	{24, "Jeremiah", "Jer", []string{"jer", "je", "jr"}},
	{25, "Lamentations", "Lam", []string{"lam", "la"}},
	{26, "Ezekiel", "Ezek", []string{"ezek", "eze", "ezk"}},
	{27, "Daniel", "Dan", []string{"dan", "da", "dn"}},
	{28, "Hosea", "Hos", []string{"hos", "ho"}},
	{29, "Joel", "Joel", []string{"joel", "jl", "joe"}},
	{30, "Amos", "Amos", []string{"amos", "am"}},
	{31, "Obadiah", "Obad", []string{"obad", "ob", "oba"}},
	{32, "Jonah", "Jonah", []string{"jonah", "jon", "jnh"}},
	{33, "Micah", "Mic", []string{"mic", "mc"}},
	{34, "Nahum", "Nah", []string{"nah", "na"}},
	{35, "Habakkuk", "Hab", []string{"hab", "hb"}},
	{36, "Zephaniah", "Zeph", []string{"zeph", "zep", "zp"}},
	{37, "Haggai", "Hag", []string{"hag", "hg"}},
	{38, "Zechariah", "Zech", []string{"zech", "zec", "zc"}},
	{39, "Malachi", "Mal", []string{"mal", "ml"}},
	{40, "Matthew", "Matt", []string{"matt", "mt", "mat"}},
	{41, "Mark", "Mark", []string{"mark", "mk", "mr", "mrk"}},
	{42, "Luke", "Luke", []string{"luke", "lk", "luk"}},
	{43, "John", "John", []string{"john", "jn", "jhn", "joh"}},
	{44, "Acts", "Acts", []string{"acts", "ac", "act"}},
	{45, "Romans", "Rom", []string{"rom", "ro", "rm"}},
	{46, "1 Corinthians", "1 Cor", []string{"1 cor", "1 co"}},
	{47, "2 Corinthians", "2 Cor", []string{"2 cor", "2 co"}},
	{48, "Galatians", "Gal", []string{"gal", "ga"}},
	{49, "Ephesians", "Eph", []string{"eph", "ephes"}},
	{50, "Philippians", "Phil", []string{"phil", "php", "pp"}},
	{51, "Colossians", "Col", []string{"col"}},
	{52, "1 Thessalonians", "1 Thess", []string{"1 thess", "1 thes", "1 th"}},
	{53, "2 Thessalonians", "2 Thess", []string{"2 thess", "2 thes", "2 th"}},
	{54, "1 Timothy", "1 Tim", []string{"1 tim", "1 ti", "1 tm"}},
	{55, "2 Timothy", "2 Tim", []string{"2 tim", "2 ti", "2 tm"}},
	{56, "Titus", "Titus", []string{"titus", "tit"}},
	{57, "Philemon", "Phlm", []string{"phlm", "philem", "phm"}},
	{58, "Hebrews", "Heb", []string{"heb"}},
	{59, "James", "Jas", []string{"jas", "jm"}},
	{60, "1 Peter", "1 Pet", []string{"1 pet", "1 pe", "1 pt", "1 p"}},
	{61, "2 Peter", "2 Pet", []string{"2 pet", "2 pe", "2 pt", "2 p"}},
	{62, "1 John", "1 John", []string{"1 john", "1 jn", "1 jhn", "1 jo", "1 j"}},
	{63, "2 John", "2 John", []string{"2 john", "2 jn", "2 jhn", "2 jo", "2 j"}},
	{64, "3 John", "3 John", []string{"3 john", "3 jn", "3 jhn", "3 jo", "3 j"}},
	{65, "Jude", "Jude", []string{"jude", "jde"}},
	{66, "Revelation", "Rev", []string{"rev", "re", "rv", "revelations", "apocalypse", "apoc"}},
}

// bookChapters is the number of chapters of each book (index = book number - 1),
// following the English (KJV) chapter division.
var bookChapters = [66]int{
	50, 40, 27, 36, 34, 24, 21, 4, 31, 24, 22, 25, 29, 36, 10, 13, 10, 42, 150, 31, 12, 8, // Gen-Song
	66, 52, 5, 48, 12, 14, 3, 9, 1, 4, 7, 3, 3, 3, 2, 14, 4, // Isa-Mal
	28, 16, 24, 21, 28, 16, 16, 13, 6, 6, 4, 4, 5, 3, 6, 4, 3, 1, 13, 5, 5, 3, 5, 1, 1, 1, 22, // Matt-Rev
}

// ChapterCount returns the number of chapters in the book with the given
// 1-based number, or 0 for an unknown book.
func ChapterCount(book int) int {
	if book < 1 || book > len(bookChapters) {
		return 0
	}
	return bookChapters[book-1]
}

// BookByNumber returns the book with the given 1-based number.
func BookByNumber(n int) (Book, bool) {
	if n < 1 || n > len(Books) || Books[n-1].Number != n {
		return Book{}, false
	}
	return Books[n-1], true
}

// bookIndex maps a normalised book key (see bookKey) to book numbers.
type bookIndex struct {
	exact map[string]int
	keys  []string // sorted, for prefix matching
}

var books = buildBookIndex(Books)

// buildBookIndex indexes every book's name, abbreviation and aliases. A key that
// would name two books is dropped from the exact map (it can then only be used
// as an ambiguous prefix); TestBookAliasesUnique guards against that.
func buildBookIndex(list []Book) bookIndex {
	idx := bookIndex{exact: map[string]int{}}
	dup := map[string]bool{}
	for _, b := range list {
		for _, name := range append([]string{b.Name, b.Abbrev}, b.Aliases...) {
			k := bookKey(name)
			if k == "" {
				continue
			}
			if n, ok := idx.exact[k]; ok && n != b.Number {
				dup[k] = true
			}
			idx.exact[k] = b.Number
		}
	}
	for k := range dup {
		delete(idx.exact, k)
	}
	for k := range idx.exact {
		idx.keys = append(idx.keys, k)
	}
	sort.Strings(idx.keys)
	return idx
}

// ordinalWords maps a leading book ordinal to its digit.
var ordinalWords = map[string]string{
	"1": "1", "2": "2", "3": "3",
	"i": "1", "ii": "2", "iii": "3",
	"first": "1", "second": "2", "third": "3",
	"1st": "1", "2nd": "2", "3rd": "3",
}

// bookKey normalises a book name for lookup: lower case, periods removed, a
// leading ordinal word turned into a digit, and all spaces removed.
// "I Samuel" -> "1samuel", "1 Cor." -> "1cor", "Song of Songs" -> "songofsongs".
func bookKey(name string) string {
	name = strings.ToLower(strings.ReplaceAll(name, ".", " "))
	words := strings.Fields(name)
	if len(words) > 1 {
		if d, ok := ordinalWords[words[0]]; ok {
			words[0] = d
		}
	}
	return strings.Join(words, "")
}

// FindBook resolves a book name, abbreviation, alias or unambiguous prefix
// ("jn", "1 Cor", "I Samuel", "Song of Songs", "Phile"). Matching ignores case,
// spaces and periods. Errors wrap ErrBadReference.
func FindBook(name string) (Book, error) {
	k := bookKey(name)
	if k == "" {
		return Book{}, fmt.Errorf("%w: missing book name", ErrBadReference)
	}
	if n, ok := books.exact[k]; ok {
		b, _ := BookByNumber(n)
		return b, nil
	}
	// Otherwise an unambiguous prefix of any key.
	var matches []int
	seen := map[int]bool{}
	for i := sort.SearchStrings(books.keys, k); i < len(books.keys) && strings.HasPrefix(books.keys[i], k); i++ {
		if n := books.exact[books.keys[i]]; !seen[n] {
			seen[n] = true
			matches = append(matches, n)
		}
	}
	switch len(matches) {
	case 0:
		return Book{}, fmt.Errorf("%w: unknown book %q", ErrBadReference, name)
	case 1:
	default:
		sort.Ints(matches)
		names := make([]string, len(matches))
		for i, n := range matches {
			names[i] = Books[n-1].Name
		}
		return Book{}, fmt.Errorf("%w: ambiguous book %q (%s)", ErrBadReference, name, strings.Join(names, ", "))
	}
	match := matches[0]
	b, _ := BookByNumber(match)
	return b, nil
}
