# dmgbuild settings for the FreeBeams DMG (bin/release-dmg passes -D app=...,
# -D root=... and the volume name). No Finder scripting: dmgbuild writes the
# window layout (.DS_Store) itself, so the build runs unattended and in CI.
import os

app = defines["app"]  # noqa: F821 (dmgbuild provides defines)
root = defines["root"]  # noqa: F821

# lzma: the smallest of hdiutil's compressed formats (macOS 10.15+; the app needs 12+).
format = "ULMO"
filesystem = "HFS+"

files = [app]
symlinks = {"Applications": "/Applications"}
hide_extension = ["FreeBeams.app"]

# The volume's own icon (Finder sidebar, desktop) is the app icon.
icon = os.path.join(root, "graphics", "FreeBeams.icns")

# The window: the background carries the instruction and the arrow between
# the two icons (packaging/branding/make-art.py draws it for these positions).
background = os.path.join(root, "packaging", "macos", "dmg-background.tiff")
window_rect = ((200, 120), (660, 400))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
icon_size = 112
text_size = 13
icon_locations = {"FreeBeams.app": (170, 220), "Applications": (490, 220)}

# The MIT licence as the disk image's licence agreement, shown when it is opened.
license = {
    "default-language": "en_US",
    "licenses": {"en_US": os.path.join(root, "LICENSE")},
}
