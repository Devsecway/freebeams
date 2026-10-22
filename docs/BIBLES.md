# Bibles

How translations get into a FreeBeams library, what ships inside the app, the Bible package format, the Bible
Manager, and the licensing policy.

## Policy

- **Bundled: public domain only.** FreeBeams ships the **King James Version (KJV)**, the **American Standard
  Version (ASV)** and the **Reina-Valera Antigua (RVA, 1909)**. All three are in the public domain. Nothing else is
  bundled.
- **Copyrighted translations are never bundled** (NIV, NLT, ESV, NKJV, MSG, HCSB, CSB, NASB, …). They can only enter a
  library through the user's own import: the Bible Manager's *Add* of a file in the user's Bibles folder, *Add from
  File…* / *Import Bible File…*, or *Import Copyrighted Bibles from EasyWorship…*, all of which ask the user to
  confirm they hold a licence. Imported texts stay in that computer's library (`library.db`), and a package the
  user saves (*Save as Package*) stays in that computer's Bibles folder; they are not shared, uploaded or bundled.
- `models.IsPublicDomainBible(abbrev, name)` is the single list of translations known to be public domain. Everything
  it does not know is treated as copyrighted. Repository fixtures and tests use KJV text or made-up verses only.

## What is bundled

| Abbrev | Name | Language | Licence | Verses | Books | Source |
|---|---|---|---|---|---|---|
| KJV | King James Version | en | Public domain (in the UK: perpetual Crown letters patent) | 31,102 | 66 | `json/KJV` of the repository below |
| ASV | American Standard Version (1901) | en | Public domain (copyright expired) | 31,086 | 66 | `json/ASV` |
| RVA | Reina-Valera Antigua (1909) | es | Public domain (copyright expired) | 31,102 | 66 | EasyWorship 8 `RVA.ewb` |

The ASV omits 16 New Testament verses the KJV has (Matthew 17:21, 18:11, 23:14; Mark 7:16, 9:44, 9:46, 11:26, 15:28;
Luke 17:36, 23:17; John 5:4; Acts 8:37, 15:34, 24:7, 28:29; Romans 16:24): its translators followed the critical Greek
text. 31,102 − 16 = 31,086, so the count is the expected ASV versification, not a decoding loss
(`TestBundledTranslations` checks every one of the 16).

**Source of the KJV and ASV**: <https://github.com/hargarpay/bible-translations>, commit `98b32dfcc20cfe58e0e6c5d5bd126b226f40e465`
(2021-01-21). The repository is a scrape of BibleGateway pages by its `app.py` into
`json/<ABBREV>/{old,new}/<Book_Name>/<chapter>.json`, one JSON object of verse number → text per chapter. It carries
no LICENSE file of its own; we rely on the public-domain status of the two texts, which the arrangement into JSON does
not change. The repository also holds 55 copyrighted translations: **none of them is used, copied or bundled**.
Peculiarities of the source and how `bibletext` handles them:

- BibleGateway renders the KJV's small-caps LORD as "Lord" and both texts' curly quotes as straight apostrophes.
- ASV footnote markers (`[a]`…`[ai]`) are removed; `[[a]forty]` in 1 Samuel 13:1 becomes "forty" (footnote marker
  dropped, bracket italics keep the word).
- 18 psalms carry their heading in verse 1 (`[a]36 The transgression…`): a leading chapter number is dropped from
  verse 1; no-break spaces become spaces.
- ASV Matthew 18:24 had a footnote body spliced into the verse ("ten thousand This talent was probably worth about
  £200, or $1000.talents."). It is corrected at bundle time by `bundled.Patches`, the only text change we make, and
  `TestPatchesApplied` checks it.

**Source of the RVA**: the EasyWorship 8 Bible plugin file `RVA.ewb` (`/Users/Shared/Softouch/Easyworship.v8/Plugins/
Bibles/RVA.ewb` on macOS), decoded by `internal/importers/easyworship` (`docs/formats/ewb.md`). The streams are UTF-8,
so the accents come through as written (`JEHOVÁ es mi pastor; nada me faltará.`, `amó Dios al mundo … unigénito`);
the result is identical, verse for verse, to the RVA that the EasyWorship import put into a library (31,102 verses
compared). The 1909 spelling is kept as published: `á` as a preposition, `fué`, `crió`, and the circumflexes the
edition puts on some proper names (`Châm`, `Nachôr`, `Melchîsedec`). EasyWorship's file carries the Reina-Valera
1960 copyright notice by mistake; the bundle states "Public Domain". Two source defects are patched by
`bundled.Patches`: a stray `> ` before Proverbs 10:1 and the edition's closing `FIN.` after Revelation 22:21.

**Embedded size**: KJV 1,259,957 bytes + ASV 1,259,131 bytes + RVA 1,292,070 bytes = **3,811,158 bytes** (3.6 MiB),
up from 2,519,018 bytes for the KJV and ASV as `.json.gz`: the release grows by **1,292,140 bytes** (1.23 MiB), the
RVA plus 70 bytes of package headers on the KJV and ASV. The data is `go:embed`ded as Bible packages in
`internal/bibles/bundled/data/*.fbbible`, so it is inside the Go dylib on every platform and needs no download or
extra files.

## First launch

`internal/app/bundled.go` runs `bundled.Install` off the main thread when the library's `bibles.bundled.version`
setting is missing or older than `bundled.Version` (2 since the RVA was added). It adds the translations the
library lacks (abbreviation match, case-insensitive, so a KJV or RVA imported from EasyWorship on an older library
is left alone), skips the built-ins the operator removed in the Bible Manager (the `bibles.bundled.removed` setting,
reported as "ASV (removed)"), records the version, and refreshes the Scriptures tab. The footer says "Installing
bundled Bibles…" and then "Installed KJV, ASV, RVA";
`/state.bundled` reports `{running, done, installed, skipped, version, error}`, and `/state.import.running` stays
true while the bundled install runs so a first-launch wait covers both imports. The EasyWorship import skips the
bundled abbreviations (`easyworship.Options.BundledBibles`, reported as "KJV (bundled)"), so the two imports never race
on the same translation. The default translation is the KJV (`core.DefaultBible`, see "The default Bible" below).

To ship another public-domain translation: add an entry to `bundled.Translations` (abbrev, name, language,
licence, attribution, `Origin`, expected counts), copy any `.fbbible` to its file name as a placeholder (the package
must compile), raise `bundled.Version`, and run the generator; `TestBundledTranslations` verifies the data against
the manifest and fails for anything `models.IsPublicDomainBible` does not accept. Libraries that recorded an older
version install the new translation at their next launch and skip the ones they have.

```bash
git clone --depth 1 https://github.com/hargarpay/bible-translations /tmp/bible-translations
go run internal/bibles/bundled/gen/main.go -src /tmp/bible-translations/json \
    -ewb /Users/Shared/Softouch/Easyworship.v8/Plugins/Bibles -out internal/bibles/bundled/data
go test ./internal/bibles/... ./internal/importers/bibletext/
```

A source whose flag is left out is re-packed from the committed data (`bundled.Load`), so `-ewb … -only RVA`
regenerates the RVA alone; the output is reproducible (the KJV and ASV re-pack to the same bytes). The generator
refuses to write when a translation is not public domain, the source has warnings, a patch does not apply, or the
counts differ from the manifest. `TestRealTranslations` in `bibletext` decodes the raw repository folders when
`FREEBEAMS_BIBLE_TRANSLATIONS=/tmp/bible-translations` is set (it skips otherwise).

## Bible packages (`.fbbible`)

One translation in one file, read and written by `internal/bibles/biblepkg` (pure Go). A package is the **gzip of
the single-file Bible JSON** below, whose **first line is a header** with the metadata:

```
{"format":"freebeams-bible","version":1,"abbrev":"KJV","name":"King James Version","language":"en","copyright":"Public Domain","verses":31102,
"books":{"Genesis":{"1":{"1":"In the beginning God created the heaven and the earth.","2":"…"},
"2":{…}},
…}}
```

- `format` is always `freebeams-bible`; `version` is the format version (1; a newer one is refused with a clear
  message). `abbrev` is required; `name`, `language` (ISO 639-1) and `copyright` (the notice, or "Public Domain")
  are optional; `verses` is the verse count.
- The rest is exactly the single-file Bible JSON that *Import Bible File…* reads (books by canonical English name,
  chapters and verses in order, one chapter per line), so `bibletext.Decode` reads the body and one reader serves
  the built-ins, the Bibles folder and *Save as Package*.
- `ReadHeader` inflates only the first line, so listing a folder of packages is cheap. `Read` checks the header
  against the books (the verse count must match: a damaged file is an error, never a short Bible), and gzip's CRC
  catches truncation (`TestEveryTruncationFails` tries every prefix of a package).
- The writer uses best compression and no timestamp: the same Bible gives the same bytes. A whole Bible is about
  1.26–1.29 MB. Any tool can write one: gzip a header line plus the JSON (`test/smoke.sh` does it in Python).
- The built-ins are packages too (`internal/bibles/bundled/data/*.fbbible`).

## The Bible Manager

Scriptures ▸ the translations list's **+**, or its gear ▸ **Manage Bibles…**. One list of every translation
FreeBeams can use, merged by abbreviation:

| Column | Values |
|---|---|
| Name | "King James Version (KJV)" |
| Language | English, Spanish, … (from the ISO code) |
| Source | **Built-in** (embedded), **Bibles folder** (a file there; it wins over "Library only"), **Library only** (installed, no package anywhere) |
| Status | Installed / Not installed |
| Size | the package or file size; "—" for Library only |

The line under the list shows the selected row's verse count, copyright and file. Buttons:

- **Add** installs the selected translation in a goroutine (read the package or file, then one transaction for the
  verses, with a progress bar under the list); the UI stays live. A built-in installs without a question (public
  domain). Anything else that `models.IsPublicDomainBible` does not know asks the licence question first (the
  `core.BibleFileQuestion` wording, "Add Bible"). Double-click or Enter on a row that is not installed adds it too.
- **Remove…** deletes the translation from the library after a question ("Remove Bible"): `store.DeleteBible`
  removes the `bibles` row, its verses and its search-index rows in one transaction. A built-in can be removed:
  it stays in the list as "Not installed" and can be added again; the removal is recorded so a newer bundle does
  not bring it back. Schedule items keep the slides they were made with, so a saved or open schedule still shows the
  removed translation's verses.
- **Save as Package** writes an installed translation that has no package (imported from a file or from
  EasyWorship, e.g. AMP, ESV, NIV, NKJV, NLT) to the Bibles folder as `<ABBREV>.fbbible`; it can then be removed
  and added again, or copied to another computer. These packages stay on the user's machine and are never bundled.
- **Add from File…** is the old *Import Bible File…* flow (native dialog, question, import).
- **Open Bibles Folder** shows the folder in the Finder or Explorer. **Refresh** lists it again (it is also listed
  each time the manager opens).

The translations list itself has the same removal: right-click a translation ▸ **Remove Bible…**, or press
Delete / Backspace in the focused list; both ask first.

### The Bibles folder

`~/Documents/FreeBeams/Bibles` (Windows: `%USERPROFILE%\Documents\FreeBeams\Bibles`; Linux:
`$XDG_DOCUMENTS_DIR/FreeBeams/Bibles` when that variable is set, else `~/Documents/FreeBeams/Bibles`), created the
first time the manager opens. It is in Documents, not in the hidden application-data folder, so it is easy to find
and to drop files into. `FREEBEAMS_BIBLES_DIR` overrides it (the smoke test uses a temporary folder).
`appdirs.Bibles` picks it (a Documents folder that Windows redirects, e.g. to OneDrive, is not followed).

Everything `internal/bibles/biblefolder` understands appears in the manager: `.fbbible` packages (listed from their
header), Bible JSON files in either layout (`.json`), EasyWorship `.ewb` files (listed from their header and verse
records) and bible-translations folders (`<ABBREV>/{old,new}/…`). Hidden files and other types are ignored. A file
that cannot be read, or that repeats an abbreviation found in a better file (a package beats an `.ewb` beats JSON
beats a folder), is reported in the manager's status line and the log, and left out.

### The default Bible

The Scriptures tab starts on, and falls back to, `core.DefaultBible`: the first **installed built-in in bundle
order** (KJV, then ASV, then RVA), else the first translation in the list (sorted by abbreviation). When the
translation selected in the Scriptures tab is removed, the tab switches to that default at once and looks the
current reference up again there; removing the KJV makes the ASV the default, and adding the KJV back makes it the
default again the next time a fallback is needed (an explicit selection is never changed). With no translation
left, the footer says "No Bibles installed".

### Control and `/state`

| Action | Body | Effect |
|---|---|---|
| `bibles-available` | | lists the Bibles folder again; the reply's `/state.bibleManager.entries` has every row |
| `bible-manager` | `{"action": "open" \| "close"}` (or `mode`, or `on`) | shows / hides the manager |
| `bible-select` | `{abbrev}` | selects a row |
| `bible-add` | `{abbrev \| path, confirm}` | adds a row (a path in the Bibles folder is its row; any other path goes through Import Bible File…); a copyrighted one asks unless `confirm` |
| `bible-remove` | `{abbrev, confirm}` | removes an installed translation; asks unless `confirm` |
| `bible-save-package` | `{abbrev}` | Save as Package |
| `bible-open-folder` | | Open Bibles Folder |

`/state.bibleManager` is `{open, folder, scanning, scanned, task ("" | add | remove | save), taskBible, progress,
status, error, selected, problems, entries}`; each entry is `{abbrev, name, language, copyright, source (builtin |
folder | library), installed, verses, size, path, builtin, hasPackage, publicDomain}`. Screenshots:
`/screenshot?window=bible-manager`; focus targets `bible-list` and `bible-manager-list`.

## Import Bible File…

The Scriptures gear menu's *Import Bible File…* (and the Bible Manager's *Add from File…*) opens the native file
dialog (files or folders). Accepted:

- a **Bible package** (`.fbbible`, above);

- a translation **folder** in the bible-translations layout (`<ABBREV>/{old,new}/<Book>/<n>.json`); the abbreviation is
  the folder name, known abbreviations get their full name;
- any **chapter file** inside such a folder (the whole translation is imported);
- a **single-file Bible JSON**, the form the bundled data uses:
  ```json
  {"abbrev": "KJV", "name": "King James Version", "language": "en", "copyright": "Public Domain",
   "books": {"Genesis": {"1": {"1": "In the beginning God created the heaven and the earth.", "2": "…"}}}}
  ```
  Book keys are names, abbreviations or canonical numbers (`"1"`…`"66"`, `"Psalm"`, `"Song_of_Solomon"`, `"1 Cor"`);
  a verse key `"1-2"` (verses the translation combines) stores the text under verse 1;
- the **array layout** of <https://github.com/thiagobodruk/bible> and the `en_kjv.json`-style files derived from it
  (`internal/importers/bibletext/array.go`):
  ```json
  [{"abbrev": "gn", "name": "Gênesis", "chapters": [["In the beginning God created…", "The earth was…"], ["…"]]}, …]
  ```
  66 books in canonical order, chapters and verses positional, no header. The file has no abbreviation of its own, so
  it comes from the **file name**: `en_esv.json` → ESV, language `en`; `NKJV.json` → NKJV (`TranslationFromFilename`);
  known abbreviations get their full name. The repository's book abbreviations and names are Portuguese (`jn` is Jonah
  there, `jo` John, `sl` Psalms), so a whole Bible resolves its books by position; a file with fewer books by name,
  then abbreviation. The files are BibleGateway scrapes and `CleanArrayText` repairs what the scrape broke: bracketed
  cross-references (`[Heb 11:3]`, `[Ex 17:6; Num 20:11]`, the Amplified Bible's inline footnotes) are removed while
  other brackets stay (the AMP's amplifications are its text, `[greatly]`, `[me]`), `( Elohim)` closes up, a small-caps
  LORD that lost its trailing space is separated again (`Lordis` → `Lord is`; `Lordship` is left alone), rows of dashes
  go. What it cannot repair: the NKJV scrape also glued italicised (supplied) words to their neighbours (`itwas`,
  `thereis`), which only a dictionary could undo;
- an EasyWorship **`.ewb`** file (`docs/formats/ewb.md`).

The app parses the file off the main thread, then asks: it names the translation, its verse count and copyright
notice (or "public domain"), and states that the user is responsible for holding a licence if the text is
copyrighted. On OK the translation is written in one transaction and the translations list refreshes. A translation
whose abbreviation is already installed is reported ("KJV is already installed") and not imported twice; parse and
store errors go to the message dialog. Malformed entries (unknown book, bad verse key, empty text, duplicate) are
skipped with a note in the log, never a failure.

Control: `import-bible-file {path, confirm}` (`confirm: true` skips the question; without it the question is on
screen and `accept` / `dismiss` answer it). `/state.bibles` lists the new abbreviation.

## Where the code is

- `internal/models/bible_license.go`: `IsPublicDomainBible`.
- `internal/importers/bibletext`: the JSON reader/writer (pure Go, tested on fixtures and on the raw repository).
- `internal/bibles/biblepkg`: the `.fbbible` reader and writer.
- `internal/bibles/biblefolder`: the Bibles folder listing and the any-format reader (`Read`).
- `internal/bibles/bundled`: manifest, patches, embedded packages, `Load`, `Header`, `Needed`, `Install`,
  `SetRemoved`; `gen/main.go` regenerates the data.
- `internal/appdirs`: `Bibles` (the folder). `internal/store/bibles.go`: `DeleteBible`, `AllVerses`,
  `BibleVerseCounts`, `CreateBibleWithVersesProgress`.
- `internal/app/biblemanager.go` (the manager's state, tasks, projection and control actions),
  `internal/app/core/biblemanager.go` (rows, texts, `/state.bibleManager`), `internal/ui/biblemanager.go` (the
  dialog).
- `internal/importers/easyworship`: `.ewb` reader (`ReadBible`, `ReadBibleInfo`) and the EasyWorship import
  (`Options.BundledBibles`).
- `internal/app/bundled.go` (first launch), `internal/app/biblefile.go` (Import Bible File…), `internal/ui/scripture.go`
  (gear menu), `core.BundledState` in `/state`.
