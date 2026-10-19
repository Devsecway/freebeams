package bibletext

// Translation names a translation by abbreviation (KnownTranslations).
type Translation struct {
	Name     string
	Language string // ISO 639-1
}

// KnownTranslations are the abbreviations of github.com/hargarpay/bible-translations
// with their full names, so a translation folder imports with a readable
// name. Being listed says nothing about licensing: only
// models.IsPublicDomainBible does, and everything else is treated as
// copyrighted (the user confirms they hold a licence before importing).
var KnownTranslations = map[string]Translation{
	"KJ21":     {"21st Century King James Version", "en"},
	"ASV":      {"American Standard Version", "en"},
	"AMP":      {"Amplified Bible", "en"},
	"AMPC":     {"Amplified Bible, Classic Edition", "en"},
	"BRG":      {"BRG Bible", "en"},
	"CSB":      {"Christian Standard Bible", "en"},
	"CEB":      {"Common English Bible", "en"},
	"CJB":      {"Complete Jewish Bible", "en"},
	"CEV":      {"Contemporary English Version", "en"},
	"DARBY":    {"Darby Translation", "en"},
	"DLNT":     {"Disciples' Literal New Testament", "en"},
	"DRA":      {"Douay-Rheims 1899 American Edition", "en"},
	"ERV":      {"Easy-to-Read Version", "en"},
	"EHV":      {"Evangelical Heritage Version", "en"},
	"ESV":      {"English Standard Version", "en"},
	"ESVUK":    {"English Standard Version Anglicised", "en"},
	"EXB":      {"Expanded Bible", "en"},
	"GNV":      {"1599 Geneva Bible", "en"},
	"GW":       {"GOD'S WORD Translation", "en"},
	"GNT":      {"Good News Translation", "en"},
	"HCSB":     {"Holman Christian Standard Bible", "en"},
	"ICB":      {"International Children's Bible", "en"},
	"ISV":      {"International Standard Version", "en"},
	"PHILLIPS": {"J.B. Phillips New Testament", "en"},
	"JUB":      {"Jubilee Bible 2000", "en"},
	"KJV":      {"King James Version", "en"},
	"AKJV":     {"Authorized (King James) Version", "en"},
	"LEB":      {"Lexham English Bible", "en"},
	"TLB":      {"Living Bible", "en"},
	"MSG":      {"The Message", "en"},
	"MEV":      {"Modern English Version", "en"},
	"MOUNCE":   {"Mounce Reverse-Interlinear New Testament", "en"},
	"NOG":      {"Names of God Bible", "en"},
	"NABRE":    {"New American Bible (Revised Edition)", "en"},
	"NASB":     {"New American Standard Bible", "en"},
	"NCV":      {"New Century Version", "en"},
	"NET":      {"New English Translation", "en"},
	"NIRV":     {"New International Reader's Version", "en"},
	"NIV":      {"New International Version", "en"},
	"NIVUK":    {"New International Version - UK", "en"},
	"NKJV":     {"New King James Version", "en"},
	"NLV":      {"New Life Version", "en"},
	"NLT":      {"New Living Translation", "en"},
	"NMB":      {"New Matthew Bible", "en"},
	"NRSV":     {"New Revised Standard Version", "en"},
	"NTE":      {"New Testament for Everyone", "en"},
	"OJB":      {"Orthodox Jewish Bible", "en"},
	"TPT":      {"The Passion Translation", "en"},
	"RGT":      {"Revised Geneva Translation", "en"},
	"RSV":      {"Revised Standard Version", "en"},
	"RSVCE":    {"Revised Standard Version Catholic Edition", "en"},
	"TLV":      {"Tree of Life Version", "en"},
	"VOICE":    {"The Voice", "en"},
	"WEB":      {"World English Bible", "en"},
	"WE":       {"Worldwide English (New Testament)", "en"},
	"WYC":      {"Wycliffe Bible", "en"},
	"YLT":      {"Young's Literal Translation", "en"},
}
