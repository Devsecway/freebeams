FreeBeams @VERSION@ for Linux (x86_64)
======================================

Free worship presentation software: songs, scriptures and media on a projector.
https://github.com/Devsecway/freebeams

Run it from this folder
    ./freebeams.sh

Install it (menu entry, icons, .fbs / .fbbible file types)
    ./install.sh                  for you only, into ~/.local (no root needed)
    sudo ./install.sh --system    for everyone, into /usr/local
    ./install.sh --uninstall      remove it again (your library is kept)

Your library (songs, Bibles, themes, settings) lives in ~/.config/FreeBeams;
Bible packages you collect go in ~/Documents/FreeBeams/Bibles.

Needs a 64-bit distribution with glibc 2.35 or newer (Ubuntu 22.04, Debian 12,
Fedora 36 and later), and Vulkan or OpenGL 3.3 graphics.

Files
    freebeams, linux_amd64.so      the program (the Godot engine and the app)
    freebeams.sh                   the launcher (use this one)
    ffmpeg, ffprobe, lib*.so.*     FFmpeg (LGPL), for video and audio playback
    LICENSE.txt                    FreeBeams' licence (MIT)
    THIRD-PARTY-NOTICES.txt        the licences of what FreeBeams includes
    FFMPEG-LICENSE.txt, FFMPEG-NOTICE.txt
