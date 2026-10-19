package models

import "strings"

// publicDomainBibleAbbrevs are translations known to be in the public domain,
// by abbreviation (upper case).
var publicDomainBibleAbbrevs = map[string]bool{
	"KJV": true, "AV": true, "KJVA": true, "AKJV": true,
	"ASV": true, "WEB": true, "WEBBE": true, "WEBUS": true, "YLT": true,
	"DARBY": true, "DBY": true, "BBE": true, "DRA": true, "DRB": true,
	"GNV": true, "GENEVA": true, "WBT": true, "WEBSTER": true, "WYC": true,
	"RVA": true, "RV1909": true, "RVA1909": true, "RVR1909": true, "SRV": true,
	"LSG": true, "LUT1912": true, "LUTH1912": true, "ELB1905": true, "SVV": true,
}

// publicDomainBibleNames are lower-case full names of public-domain translations.
var publicDomainBibleNames = map[string]bool{
	"king james version":                 true,
	"authorized version":                 true,
	"authorised version":                 true,
	"authorized (king james) version":    true,
	"american standard version":          true,
	"world english bible":                true,
	"young's literal translation":        true,
	"darby translation":                  true,
	"darby bible":                        true,
	"bible in basic english":             true,
	"douay-rheims":                       true,
	"douay-rheims bible":                 true,
	"douay-rheims 1899 american edition": true,
	"geneva bible":                       true,
	"1599 geneva bible":                  true,
	"webster bible":                      true,
	"webster's bible":                    true,
	"wycliffe bible":                     true,
	"reina-valera antigua":               true,
	"reina valera antigua":               true,
	"reina-valera 1909":                  true,
	"reina valera 1909":                  true,
	"louis segond 1910":                  true,
	"luther 1912":                        true,
}

// IsPublicDomainBible reports whether a translation is known to be in the
// public domain, by abbreviation or full name (case-insensitive). Unknown
// translations are treated as copyrighted (e.g. HCSB, NIV, NKJV, RVR1960):
// a copyrighted text may only enter the library through the user's own
// import of a file they hold a licence for.
func IsPublicDomainBible(abbrev, name string) bool {
	if publicDomainBibleAbbrevs[strings.ToUpper(strings.TrimSpace(abbrev))] {
		return true
	}
	return publicDomainBibleNames[strings.ToLower(strings.Join(strings.Fields(name), " "))]
}
