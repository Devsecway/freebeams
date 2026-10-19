# EasyWorship `.ewb` Bible format

Reverse-engineered from the EasyWorship 8 Bible plugins in
`/Users/Shared/Softouch/Easyworship.v8/Plugins/Bibles` (KJV, RVA, HCSB, `format_version` 1).
Decoder: `internal/importers/easyworship/ewb.go`. Always open these files read-only
(`file:...?mode=ro&immutable=1`).

An `.ewb` file is a SQLite database with four tables.

## `header` (one row)

| column | example (KJV) | notes |
|---|---|---|
| `id` | `17` | bible id; RVA = 36, HCSB = 14. It is also stored in every verse record and in the stream prefix. |
| `key` | `0` | |
| `name` | `King James Version` | |
| `abbrev_name` | `KJV` | |
| `lang_code` | `EN`, `ES` | upper-case ISO 639-1 |
| `plugin_version`, `format_version` | `1`/`2`, `1` | |
| `copyright`, `license` | NULL or RTF | Riched20 RTF (`\ansicpg1252`, `\'hh` escapes). It is not reliable: the RVA (1909) file carries the Reina-Valera **1960** notice. |

## `books` (one row per book, `rowid` 1..66 in canonical order)

| column | notes |
|---|---|
| `name`, `abbrev_name`, `alt_name` | localised name (`Génesis`), short form (`Gn`), English name for non-English bibles (`Genesis`) |
| `book_info` | byte 0 = chapter count, then one byte per chapter = verse count (Genesis: `32 1F 19 ...` = 50 chapters, 31, 25, ... verses). Informational only. It is wrong in HCSB 2 Samuel and Psalms, so the decoder does not use it. |
| `verse_info` | one 8-byte record per verse, in order (see below) |

## `streams` (one row per book, same `rowid` as `books`)

`stream` is a zlib stream (`78 DA`). It inflates to UTF-8 text:

```
[2 characters prefix] [verse 1 text][verse 2 text]...
```

The prefix is `U+0000` followed by the character with code = header `id` (KJV: `00 11`).
The verses are concatenated with no separator. Each one normally ends with a space
(`"In the beginning God created the heaven and the earth. "`).

## `verse_info` records

Each record is two little-endian `uint32`:

```
a = offset << 10 | length                                  offset, length in CHARACTERS (code points),
                                                           relative to the text after the 2-char prefix
b = bibleID << 25 | book << 18 | chapter << 10 | verse << 2   low 2 bits are 0 in every known file
```

| field | bits | range |
|---|---|---|
| length | a[0..9] | ≤ 1023 characters |
| offset | a[10..31] | ≤ 4 M characters |
| verse | b[2..9] | 1..255 |
| chapter | b[10..17] | 1..255 |
| book | b[18..24] | canonical 1..66 |
| bible id | b[25..31] | = `header.id` |

Example, KJV Genesis: `37000000 04040422` = offset 0, length 55, Gen 1:1, id 17.
`91DC0000 08040422` = offset 55, length 145, Gen 1:2.

Offsets are in characters, not bytes. This matters for RVA, where accented letters take
2 bytes in UTF-8. Records are contiguous: `offset[i+1] = offset[i] + length[i]`, and the
last record ends exactly at the end of the stream.

## `words`

A search index (`word`, `verse_info INT32A`). Ignored.

## Decoding rules (FreeBeams)

- Book number comes from the record (`b` bits 18..24). If that is out of range, it falls back to the row position.
- Records must be in increasing (chapter, verse) order. HCSB 2 Samuel stores 24:9 as **2:9**.
  When an out-of-order record's verse number continues the previous verse (`v == prev+1`),
  the previous chapter is kept. Otherwise the record is dropped. Records with zero chapter/verse,
  zero length or out-of-bounds offsets are dropped too.
- Text: `[` and `]` are removed but the words are kept. KJV (14,233 verses) and HCSB use brackets
  to mark words the translators supplied (printed in italics): `darkness [was] upon` becomes
  `darkness was upon`. Whitespace is collapsed and trimmed.
- A stream that is not valid UTF-8 is decoded as Windows-1252 (defensive; not seen so far).
- Public-domain detection uses the abbreviation or name (KJV, ASV, WEB, YLT, Darby, BBE, RVA,
  Douay-Rheims, Geneva, Webster, LSG 1910, Luther 1912, ...). Public-domain Bibles are stored
  with copyright `Public Domain`. All other Bibles keep the header notice as plain text and
  are only imported when the user opts in.

## Verified

| | KJV | RVA | HCSB |
|---|---|---|---|
| verses | 31,102 | 31,102 | 31,099 |
| Gen 1:1 | In the beginning God created the heaven and the earth. | EN el principio crió Dios los cielos y la tierra. | In the beginning God created the heavens and the earth. |
| Ps 23:1 | The LORD is my shepherd; I shall not want. | JEHOVÁ es mi pastor; nada me faltará. | |
| John 3:16 | For God so loved the world, that he gave his only begotten Son, ... everlasting life. | Porque de tal manera amó Dios al mundo, ... | |
| Rev 22:21 | The grace of our Lord Jesus Christ be with you all. Amen. | ... Amén. FIN. | |
| Ps 119 | 176 verses | | |
| 3 John | 14 verses | | |

Known source quirks that we keep as they are: RVA capitalises the first word of each chapter (`EN el principio`).
RVA Prov 10:1 starts with `> `. RVA Song 8:14 ends with a stray `Libro Anterior (ECCLESIASTÉS`.
