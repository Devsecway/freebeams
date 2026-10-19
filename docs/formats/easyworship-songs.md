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
