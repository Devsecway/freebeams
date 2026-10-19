# Bibles

How translations get into a FreeBeams library, what ships inside the app, and the licensing policy.

## Policy

- **Bundled: public domain only.** FreeBeams ships the **King James Version (KJV)** and the **American Standard
  Version (ASV)**. Both are in the public domain. Nothing else is bundled.
- **Copyrighted translations are never bundled** (NIV, NLT, ESV, NKJV, MSG, HCSB, CSB, NASB, …). They can only enter a
  library through the user's own import: *Import Bible File…* or *Import Copyrighted Bibles from EasyWorship…* in the
  Scriptures gear menu, both of which ask the user to confirm they hold a licence. Imported texts stay in that
  computer's library (`library.db`); they are not shared or uploaded.
- `models.IsPublicDomainBible(abbrev, name)` is the single list of translations known to be public domain. Everything
  it does not know is treated as copyrighted. Repository fixtures and tests use KJV text or made-up verses only.

## What is bundled

| Abbrev | Name | Language | Licence | Verses | Books | Source |
|---|---|---|---|---|---|---|
| KJV | King James Version | en | Public domain (in the UK: perpetual Crown letters patent) | 31,102 | 66 | `json/KJV` of the repository below |
| ASV | American Standard Version (1901) | en | Public domain (copyright expired) | 31,086 | 66 | `json/ASV` |

The ASV omits 16 New Testament verses the KJV has (Matthew 17:21, 18:11, 23:14; Mark 7:16, 9:44, 9:46, 11:26, 15:28;
Luke 17:36, 23:17; John 5:4; Acts 8:37, 15:34, 24:7, 28:29; Romans 16:24): its translators followed the critical Greek
text. 31,102 − 16 = 31,086, so the count is the expected ASV versification, not a decoding loss
(`TestBundledTranslations` checks every one of the 16).

**Source**: <https://github.com/hargarpay/bible-translations>, commit `98b32dfcc20cfe58e0e6c5d5bd126b226f40e465`
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

**Embedded size**: KJV 1,259,917 bytes + ASV 1,259,101 bytes = **2,519,018 bytes** (2.4 MiB) of gzip, from about
4.36 MB of JSON each. The data is `go:embed`ded in `internal/bibles/bundled/data/*.json.gz`, so it is inside the Go
dylib on every platform and needs no download or extra files.

## First launch

`internal/app/bundled.go` runs `bundled.Install` off the main thread when the library's `bibles.bundled.version`
setting is missing or older than `bundled.Version`. It adds the translations the library lacks (abbreviation match,
case-insensitive, so a KJV imported from EasyWorship on an older library is left alone), records the version, and
refreshes the Scriptures tab. The footer says "Installing bundled Bibles…" and then "Installed KJV, ASV";
`/state.bundled` reports `{running, done, installed, skipped, version, error}`, and `/state.import.running` stays
true while the bundled install runs so a first-launch wait covers both imports. The EasyWorship import skips the
bundled abbreviations (`easyworship.Options.BundledBibles`, reported as "KJV (bundled)"), so the two imports never race
on the same translation. The default translation stays KJV (`core.DefaultBible`).

To ship another public-domain translation: add its folder to the generator run below, an entry to
`bundled.Translations` (abbrev, name, language, licence, attribution, expected counts), raise `bundled.Version`, and
re-run the generator; `TestBundledTranslations` verifies the data against the manifest. Libraries that recorded an
older version install the new translation at their next launch and skip the ones they have.

```bash
git clone --depth 1 https://github.com/hargarpay/bible-translations /tmp/bible-translations
go run internal/bibles/bundled/gen/main.go -src /tmp/bible-translations/json -out internal/bibles/bundled/data
go test ./internal/bibles/... ./internal/importers/bibletext/
```

The generator refuses to write when the source has warnings, a patch does not apply, or the counts differ from the
manifest. `TestRealTranslations` in `bibletext` decodes the raw repository folders when
`FREEBEAMS_BIBLE_TRANSLATIONS=/tmp/bible-translations` is set (it skips otherwise).

## Import Bible File…

The Scriptures gear menu's *Import Bible File…* opens the native file dialog (files or folders). Accepted:

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
- `internal/bibles/bundled`: manifest, patches, embedded data, `Load`, `Needed`, `Install`; `gen/main.go` regenerates
  the data.
- `internal/importers/easyworship`: `.ewb` reader and the EasyWorship import (`Options.BundledBibles`).
- `internal/app/bundled.go` (first launch), `internal/app/biblefile.go` (Import Bible File…), `internal/ui/scripture.go`
  (gear menu), `core.BundledState` in `/state`.
