#!/bin/sh
# Install FreeBeams from this folder (the Linux tar.gz) into your home folder,
# or system-wide, with its menu entry, icons and file types.
#
#   ./install.sh                  for you only: ~/.local (no root needed)
#   sudo ./install.sh --system    for everyone: /usr/local
#   ./install.sh --prefix DIR     somewhere else
#   ./install.sh --uninstall      remove it again (add --system / --prefix as installed)
#
# The program goes to <prefix>/lib/freebeams, a `freebeams` command to
# <prefix>/bin, the menu entry, icons, file types (.fbs schedules, .fbbible Bible
# packages) and AppStream metadata to <prefix>/share. Your library
# (~/.config/FreeBeams) is never touched: uninstalling keeps it.
set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"
PREFIX="$HOME/.local"
uninstall=0
while [ $# -gt 0 ]; do
	case "$1" in
	--system) PREFIX=/usr/local ;;
	--prefix)
		[ $# -ge 2 ] || { echo "install.sh: --prefix needs a directory" >&2; exit 2; }
		PREFIX="$2"
		shift
		;;
	--uninstall) uninstall=1 ;;
	-h | --help)
		sed -n '2,13p' "$0"
		exit 0
		;;
	*)
		echo "install.sh: unknown option $1 (see --help)" >&2
		exit 2
		;;
	esac
	shift
done

APPDIR="$PREFIX/lib/freebeams"
SHARE="$PREFIX/share"
ID=com.devsecway.freebeams

refresh() {
	if command -v update-mime-database >/dev/null 2>&1; then
		update-mime-database "$SHARE/mime" >/dev/null 2>&1 || true
	fi
	if command -v update-desktop-database >/dev/null 2>&1; then
		update-desktop-database "$SHARE/applications" >/dev/null 2>&1 || true
	fi
	if command -v gtk-update-icon-cache >/dev/null 2>&1; then
		gtk-update-icon-cache -q -t "$SHARE/icons/hicolor" >/dev/null 2>&1 || true
	fi
}

if [ "$uninstall" = 1 ]; then
	rm -rf "$APPDIR"
	rm -f "$PREFIX/bin/freebeams" "$SHARE/applications/$ID.desktop" "$SHARE/metainfo/$ID.metainfo.xml" \
		"$SHARE/mime/packages/freebeams-mime.xml"
	find "$SHARE/icons/hicolor" \( -name 'freebeams.png' -o -name 'freebeams.svg' -o -name 'application-x-freebeams-*.png' \) \
		-exec rm -f {} + 2>/dev/null || true
	refresh
	echo "FreeBeams removed from $PREFIX (your library in ~/.config/FreeBeams is kept)."
	exit 0
fi

[ -x "$HERE/freebeams" ] || { echo "install.sh: run it from the unpacked FreeBeams folder" >&2; exit 1; }
mkdir -p "$APPDIR" "$PREFIX/bin" "$SHARE/applications" "$SHARE/metainfo" "$SHARE/mime/packages"
# The program and everything it loads from its own folder.
for f in "$HERE"/*; do
	case "$(basename "$f")" in
	install.sh | share | README.txt) ;;
	*) cp -f "$f" "$APPDIR/" ;;
	esac
done
cp -f "$HERE/README.txt" "$APPDIR/" 2>/dev/null || true
ln -sf "$APPDIR/freebeams.sh" "$PREFIX/bin/freebeams"
sed "s|^Exec=.*|Exec=\"$APPDIR/freebeams.sh\" %F|" "$HERE/share/applications/$ID.desktop" >"$SHARE/applications/$ID.desktop"
cp -f "$HERE/share/metainfo/$ID.metainfo.xml" "$SHARE/metainfo/"
cp -f "$HERE/share/mime/packages/freebeams-mime.xml" "$SHARE/mime/packages/"
cp -R "$HERE/share/icons" "$SHARE/"
refresh
echo "FreeBeams is installed in $APPDIR."
echo "Start it from your applications menu, or run: $PREFIX/bin/freebeams"
case ":$PATH:" in
*":$PREFIX/bin:"*) ;;
*) echo "($PREFIX/bin is not on your PATH; the menu entry works regardless.)" ;;
esac
