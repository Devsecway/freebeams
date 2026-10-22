# EasyWorship song library (Songs.db / SongWords.db)

Location: `/Users/Shared/Softouch/Easyworship/Default/v6.1/Databases/Data` (macOS). On Windows it is
`C:\Users\Public\Documents\Softouch\Easyworship\Default\v6.1\Databases\Data`.
Reader: `internal/importers/easyworship` (RTF: `internal/importers/rtf`).
The databases are opened read-only and immutable (`file:...?mode=ro&immutable=1`), so EasyWorship's data is never written.

## Songs.db, table `song`

| column | use |
|---|---|
| `rowid` | key, joined to `word.song_id` |
| `title`, `author`, `copyright`, `administrator`, `tags` | metadata. `author` joins several names with spaces (`Henry F. Lyte William H. Monk`). |
| `reference_number` | CCLI song number |
| `song_uid`, `song_item_uid`, `song_rev_uid` | EW identifiers (`PCI-<sha1>` for the bundled library). Not imported. |
| `description`, `provider_id`, `vendor_id`, `presentation_id`, `layout_revision`, `revision` | ignored |

The text columns are declared `COLLATE UTF8_U_CI`, which is a custom EW collation. A plain `sqlite3` CLI fails
on `WHERE`/`ORDER BY` with "no such collation sequence". The importer avoids comparing those columns in SQL
and also registers a compatible case-insensitive `UTF8_U_CI` collation with the modernc driver.

The local library has 223 public-domain hymns. None has a CCLI number, tags or an administrator.

## SongWords.db, table `word`

`song_id` → `song.rowid`, `words` = lyrics as **RTF**. `slide_uids`, `slide_layout_revisions` and `slide_revisions` are ignored.

Every blob has the same shape:

```
{\rtf1\ansi\deff0\deftab254{\fonttbl{\f0\fnil\fcharset0 Arial;}{\f1\fnil\fcharset0 Verdana;}}{\colortbl...}
\paperw12240\paperh15840\margl1880\margr1880\margt1440\margb1440
{\*\pnseclvl1\pnucrm\pnstart1\pnhang\pnindent720{\pntxtb}{\pntxta{.}}}   (x9, list numbering)
{\pard\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Verse 1\par
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Amazing grace how sweet the sound,\par
...
\li0\fi0\ri0\sb0\sl\sa0 \par                                    <- blank line = slide break
\li0\fi0\ri0\sb0\sl\sa0 \plain\f1\fntnamaut Verse 2\par
...last line}
}
```

Findings (all 223 blobs surveyed):

- **No EW-specific section markup.** The only control words are standard RTF:
  `\par \pard \plain \f \fntnamaut \li \fi \ri \sb \sl \sa`, the table/`\pn*` groups, and page setup.
  `\fntnamaut` is a font-name autodetect flag. It is not a section marker. There are no `\sdslidemarker`-like words.
- Section labels are ordinary paragraphs: `Verse 1`..`Verse 11`, `Chorus 1`..`Chorus 3`.
  Every song is labelled. EW writes `Chorus 1` even when a song has only one chorus.
- A blank paragraph (`\par` with no text, sometimes `\plain\f1\fntnamaut \par`) is a slide break.
  It usually separates sections. Sometimes it falls inside one, e.g. *A New Name In Glory*'s
  `Chorus 1` spans two slides.
- The text is 7-bit ASCII here: no `\'hh` or `\u` escapes. The RTF converter still supports them, along with `\ansicpg`/`\fcharset`
  codepages, `\uN`+`\ucN`, `\~ \- \_`, `\bin` and ignorable destinations, for libraries in other languages.
- Lines often end with a trailing space before `\par`. The last line has no `\par`.

## Section mapping (FreeBeams)

`parseLyrics` in `internal/importers/easyworship/lyrics.go` works like this:

- A line that is exactly a label becomes a section header. Accepted forms: `Verse 2`, `Chorus`, `Refrain`, `Pre-Chorus`, `Bridge`,
  `Tag`, `Intro`, `Ending`/`Outro`/`Coda`, short forms `V1`, `C`, `B`, `PC`, optionally in `[...]` or followed by `:`.
  The label keeps its number as written: `Chorus 1` becomes Kind `chorus`, Number 1, Label `Chorus 1`.
- A blank line ends a slide. The lines after it, up to the next label, become another
  `models.Section` with the same Kind/Number/Label. A two-slide chorus is therefore two
  consecutive `Chorus 1` sections.
- Text without any labels: blank-line stanzas become `Verse 1..n`. In labelled text, stanzas
  before the first label become an unnumbered `Verse`.
- Lines are trimmed (NBSP is treated as a space). Labels never appear in `Lines`. Empty sections are dropped.

Import (`ImportInto`) skips songs already in the library with the same title and author
(case-insensitive, whitespace-collapsed). It saves the rest in one transaction with `store.SaveSongs`.

## Per-song themes: not imported

FreeBeams songs can have a theme of their own (`Song.ThemeID`, see `docs/PHASE2-STATUS.md` "Per-song themes").
The importer leaves it at 0 (the default song theme) because EasyWorship's link could not be verified.
Surveyed on 2026-10-22 against the local data (`Songs.db`, `SongWords.db`, `Themes.db`, `PresentationLayouts.db`):

- **Songs.db `song.presentation_id` is 0 for all 223 songs**, and `layout_revision` is 1 for every song.
  No song in this library has a theme or layout of its own, so there is no real example of the link.
- **Themes.db `file_item`** lists 110 themes: 36 with `presentation_type` 12, 37 with type 13, 37 with type 14. The
  same title appears under several types (`Fall Skies 1` is 12, 13 and 14). Each row's `presentation_id` points at a
  `presentation` row in PresentationLayouts.db. The meaning of the types is not documented. Guessing a song, scripture
  or presentation type from the titles would be speculation.
- **PresentationLayouts.db** stores a theme as a `presentation` with `slide` rows (a type-12 theme has six, the
  others one), each `slide` with `element` rows (geometry in `x`/`y`/`width`/`height`, `element_type`, and background,
  foreground and shape `resource` ids). `element_property` holds mostly override flags (`mofText`, `mofDims`,
  `mofBackground`), not fonts or colors. Backgrounds are `resource_image` / `resource_video` rows that name the file
  (`<images>Echo Morph Still.jpg`, `<videos>EWM-…HD720.mp4`). The `file` table is empty, so the media itself is not in
  these databases. Where a theme keeps its font, size, color, outline and shadow was not found. `resource_text`
  (RTF) has only 3 rows for 110 themes, and the per-element properties are flags.
- `SongWords.db` `word.slide_uids` / `slide_layout_revisions` reference per-slide layouts by uid. They are filled for
  every song, but nothing ties them to a theme in this library.

Two things would have to be decoded and checked before a mapping could be trusted: the song → theme link
(`presentation_id` pointing into PresentationLayouts.db and its `slide.theme_id` / `modified_theme_id`, with a real
non-zero example), and an EW theme → `models.Theme` conversion (background media, the text element's box, the font
and color, outline and shadow). Neither can be verified here, so no EW theme or song theme is imported.
EasyWorship's own Profiles ▸ Utilities ▸ "Reset Songs to Default Theme" suggests the link exists in libraries
where operators assigned themes. A library with such songs would make the first half testable.
