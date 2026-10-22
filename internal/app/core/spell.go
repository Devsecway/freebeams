package core

// Spell checking in the song editor (internal/app/spell.go).

// Settings keys of the spelling checker.
const (
	SettingSpellPersonal = "spell.personal" // []string: Add to Dictionary words
	SettingSpellLive     = "spell.live"     // bool: Check Spelling as I Type (default on)
	SettingSpellLanguage = "spell.language" // string: "" is the checker's default
)

// SpellState is the spelling checker in GET /state (dialog.spell).
type SpellState struct {
	Engine    string   `json:"engine"`    // "macOS" (NSSpellChecker) or "SCOWL en_US"
	Language  string   `json:"language"`  // "" is the checker's default
	Languages []string `json:"languages"` // the checker's languages, its default first
	Live      bool     `json:"live"`      // Check Spelling as I Type
	Personal  []string `json:"personal"`  // the personal dictionary

	// The song editor: its underlined words (in text order), a check in
	// flight or waiting for the typing debounce, the context menu's entries
	// while it is open.
	Flagged  []string `json:"flagged"`
	Checking bool     `json:"checking"`
	Menu     []string `json:"menu,omitempty"`

	// Dialog is the Spelling dialog while it is open.
	Dialog *SpellDialogState `json:"dialog,omitempty"`
}

// SpellDialogState is the open Spelling dialog.
type SpellDialogState struct {
	Word        string   `json:"word"` // the misspelling on show ("" when done)
	Suggestions []string `json:"suggestions"`
	Replacement string   `json:"replacement"` // the change-to field
	Done        bool     `json:"done"`        // the check reached the end
	Changes     int      `json:"changes"`
}
