#!/usr/bin/env bash
# Regenerate THIRD-PARTY-NOTICES.txt at the repository root: the licences of
# everything a FreeBeams package carries besides FreeBeams' own code (LICENSE).
#
#   packaging/notices/make-notices.sh
#
# Sources: packaging/notices/header.txt (the summary), the Go modules linked
# into the app (go list -deps, their LICENSE files from the module cache), the
# Godot engine's LICENSE.txt and COPYRIGHT.txt at the version gd exports with
# (GODOT_SRC, a checkout of that tag, or fetched from GitHub), and SCOWL's
# licence (internal/spell/data). Rerun it after a go.mod or Godot bump and
# commit the result; CI checks that it is current (--check).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"
GODOT_VERSION="${GODOT_VERSION:-$(sed -n 's/^GODOT_VERSION=//p' bin/build-engine)}"
OUT=THIRD-PARTY-NOTICES.txt
check=0
[ "${1:-}" = "--check" ] && check=1

godot_file() { # godot_file NAME: a file of the Godot source at the pinned tag
	if [ -n "${GODOT_SRC:-}" ] && [ -f "$GODOT_SRC/$1" ]; then
		cat "$GODOT_SRC/$1"
	else
		curl -fsSL "https://raw.githubusercontent.com/godotengine/godot/$GODOT_VERSION-stable/$1"
	fi
}
rule() { printf '\n%s\n%s\n\n' "================================================================================" "$1"; }

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
{
	sed "s/@GODOT_VERSION@/$GODOT_VERSION/g" packaging/notices/header.txt

	rule "Godot Engine $GODOT_VERSION (MIT) - LICENSE.txt"
	godot_file LICENSE.txt
	rule "Godot Engine $GODOT_VERSION - COPYRIGHT.txt (the engine's own third-party components)"
	godot_file COPYRIGHT.txt

	# Every non-main module the app links, with its licence file as shipped.
	go list -deps -f '{{if .Module}}{{if not .Module.Main}}{{.Module.Path}} {{.Module.Version}} {{.Module.Dir}}{{end}}{{end}}' . ./internal/... |
		sort -u | while read -r path version dir; do
		lic="$(find "$dir" -maxdepth 1 -type f \( -iname 'license*' -o -iname 'licence*' -o -iname 'copying*' \) | sort | head -n 1)"
		[ -n "$lic" ] || {
			echo "make-notices: no licence file in $path $version" >&2
			exit 1
		}
		rule "Go module $path $version"
		cat "$lic"
	done

	rule "SCOWL word list (spell checking, internal/spell/data)"
	cat internal/spell/data/SCOWL-LICENSE
} >"$tmp"

if [ "$check" = 1 ]; then
	cmp -s "$tmp" "$OUT" || {
		echo "make-notices: $OUT is out of date; run packaging/notices/make-notices.sh" >&2
		exit 1
	}
	echo "$OUT is current"
else
	mv -f "$tmp" "$OUT"
	trap - EXIT
	echo "wrote $OUT ($(wc -c <"$OUT" | tr -d " ") bytes)"
fi
