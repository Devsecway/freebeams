# Releasing FreeBeams

How a release is built, locally and in CI, what needs credentials, and what is still unverified. Everything
below runs the same scripts in `bin/`; the GitHub Actions workflows only call them.

## The version

`config/version` in `graphics/project.godot` is the single source of truth.

- `bin/version` prints it; `bin/version --check vX.Y.Z` fails unless the tag matches (the release workflow runs it).
- The Godot exports fall back to it: the macOS and Windows presets leave their own version fields empty on purpose,
  so `CFBundleShortVersionString`/`CFBundleVersion` and the Windows file/product versions come from the same place.
- The app logs it at startup: `[freebeams] FreeBeams 0.1.0 (Godot 4.7.2-stable (official))`
  (`internal/app/version.go`, read from `ProjectSettings`; `app.Version()` is available for an About box or the title).
- The artifacts carry it: `FreeBeams-0.1.0-macOS.dmg`, `FreeBeams-0.1.0-windows-amd64.zip`,
  `FreeBeams-0.1.0-Setup.exe`, `FreeBeams-0.1.0-linux-amd64.tar.gz`, `FreeBeams-0.1.0-x86_64.AppImage`.

To bump: edit `config/version`, commit, tag `vX.Y.Z`.

## Icon

`graphics/icon.svg` is the master artwork (our own: an indigo tile, a projector beam and a lyric slide).
`packaging/icons/make-icons.sh` (macOS: QuickLook + iconutil + Pillow) regenerates the committed derivatives
`graphics/icon.png` (Linux, 512 px), `graphics/icon.ico` (Windows) and `graphics/FreeBeams.icns` (macOS).
`project.godot` points `config/icon`, `config/macos_native_icon` and `config/windows_native_icon` at them, and the
export presets use the `.icns`/`.ico`.

## macOS

```bash
GOOS=macos bin/gd build     # releases/darwin/universal/FreeBeams.app (universal: x86_64 + arm64)
bin/release-dmg             # builds, then releases/FreeBeams-<v>-macOS.dmg (SKIP_BUILD=1 to repackage)
```

`gd build` compiles `darwin_arm64.dylib` and `darwin_amd64.dylib`, lipo's them into `darwin_universal.dylib`, and
runs Godot's headless export of the `macOS` preset (ad-hoc signed by Godot). The first run downloads the macOS
export template (124 MB) into `~/Library/Application Support/Godot/export_templates/4.7.2.stable/`.

To check the exported app without a GUI test:

```bash
FREEBEAMS_LIBRARY=$(mktemp) FREEBEAMS_CONTROL_KEY=testkey \
  releases/darwin/universal/FreeBeams.app/Contents/MacOS/FreeBeams &
# stdout prints "Control server: http://127.0.0.1:<port>"
curl -H 'Authorization: Bearer testkey' http://127.0.0.1:<port>/state
curl -X POST -H 'Authorization: Bearer testkey' -d '{}' http://127.0.0.1:<port>/action/quit
```

### Signing and notarizing (needs credentials)

`bin/sign-notarize` deep-signs every Mach-O in the bundle with the hardened runtime and
`packaging/macos/entitlements.plist`, packages the DMG through `bin/release-dmg`, signs it, submits it to Apple's
notary service, staples the ticket to the DMG and the app, and validates. It needs:

| Credential | Local | CI (repository secrets) |
|---|---|---|
| Developer ID Application certificate | in the login keychain, named by `SIGN_IDENTITY` | `MACOS_CERT_P12_BASE64`, `MACOS_CERT_PASSWORD` (imported into a throwaway keychain) |
| Notary access | `NOTARY_PROFILE` from `xcrun notarytool store-credentials` | `APPLE_API_KEY_P8_BASE64`, `APPLE_API_KEY_ID`, `APPLE_API_ISSUER_ID` (App Store Connect API key) |

```bash
SIGN_IDENTITY="Developer ID Application: Name (TEAMID)" NOTARY_PROFILE=freebeams-notary bin/sign-notarize
SKIP_NOTARIZE=1 ...   # sign only; SKIP_BUILD=1 reuses the built app
```

Without credentials the script stops before touching the build, with a message naming what is missing. The
release workflow runs it only when the secrets are set and otherwise uploads an unsigned DMG with a warning.

## Windows

```bash
bin/build-windows     # releases/windows/amd64/, releases/FreeBeams-<v>-windows-amd64.zip, releases/FreeBeams-<v>-Setup.exe
```

Native build on Windows under MSYS2 UCRT64 (`pacman -S mingw-w64-ucrt-x86_64-gcc`), with Go on PATH:
`MSYSTEM=UCRT64 /c/msys64/usr/bin/bash -l bin/build-windows`. It runs `GOOS=windows bin/gd build` (gcc builds
`windows_amd64.dll`, Godot exports the `Windows x86_64` preset), copies the DLL beside `FreeBeams.exe`, bundles any
`lib*.dll` the DLL still needs, brands the exe with `rcedit` (icon, version resource: Godot only does this itself when
rcedit is configured in its editor settings), zips the folder, and builds the installer from
`packaging/windows/freebeams.iss` with Inno Setup (`ISCC`). `SIGNTOOL_CMD="signtool sign ..."` is the Authenticode
hook; it runs on every PE file when set. Cross-building from macOS or Linux needs `zig` on PATH (gd uses `zig cc`)
and produces the zip only.

## Linux

```bash
bin/build-linux       # releases/linux/<arch>/, releases/FreeBeams-<v>-linux-<arch>.tar.gz, releases/FreeBeams-<v>-x86_64.AppImage
```

Linux only, natively (GOARCH must be the host's). Build on an old distro (CI: `ubuntu-22.04`) so the glibc floor
stays low. The tar.gz holds the binary, `linux_<arch>.so`, a launcher (`freebeams.sh`), the icon and the
`.desktop` file; the AppImage is assembled from `packaging/linux/` (`AppRun` preloads the `.so` for the aarch64
static-TLS issue) with `appimagetool`, downloaded into `~/.cache/freebeams/` when missing (`APPIMAGETOOL=` to
point at one, `NO_APPIMAGE=1` to skip).

## CI

- `.github/workflows/ci.yml`: on push to `main` and on pull requests: `gofmt -l`, `go build`, `go vet`, `go test`
  on `macos-15` (the EasyWorship-data tests skip there), plus `shellcheck` over the scripts on Ubuntu.
- `.github/workflows/release.yml`: on a `v*` tag (or by hand, for a dry run without a release): checks the tag
  against `bin/version`, then builds macOS (`macos-15`), Windows (`windows-latest`, MSYS2) and Linux (`ubuntu-22.04`)
  natively, uploads the artifacts, and for a tag creates a **draft** GitHub release with the artifacts and
  `SHA256SUMS.txt` for a maintainer to publish. Godot and the export templates are cached under `$GDPATH/bin` and the
  per-OS template directory, keyed on `go.mod`. `GOTOOLCHAIN` is set to the installed Go version because
  `actions/setup-go` pins `GOTOOLCHAIN=local`, which gd reads as "download nothing".

Cutting a release:

```bash
# 1. bump config/version in graphics/project.godot, commit
# 2. tag and push
git tag v0.1.0 && git push origin v0.1.0
# 3. wait for the Release workflow, review the draft at github.com/<org>/freebeams/releases, publish
```

## Verified on 2026-10-19 (macOS 26.6, Apple M3 Max, Godot 4.7.2, go 1.27.2)

- `GOOS=macos bin/gd build` → `releases/darwin/universal/FreeBeams.app`, universal (`lipo -archs`: `x86_64 arm64`
  for the executable and `Frameworks/darwin_universal.dylib`), `Info.plist`: `CFBundleShortVersionString 0.1.0`,
  `CFBundleIdentifier com.devsecway.freebeams`, `CFBundleIconFile icon.icns`,
  `LSApplicationCategoryType public.app-category.productivity`; ad-hoc signed with the hardened-runtime flag.
- The exported app launches (`FREEBEAMS_LIBRARY=$(mktemp) FREEBEAMS_CONTROL_KEY=testkey .../MacOS/FreeBeams`),
  logs `FreeBeams 0.1.0 (Godot 4.7.2-stable (official))`, answers `GET /state`, and exits on
  `POST /action/quit`.
- `SKIP_BUILD=1 bin/release-dmg` → `releases/FreeBeams-0.1.0-macOS.dmg` (mounts; holds `FreeBeams.app` and
  `Applications`).
- `bin/sign-notarize` without credentials stops with `set SIGN_IDENTITY ...` before touching the build.
- `shellcheck` 0.11.0 is clean over `bin/*`, `packaging/linux/AppRun`, `packaging/icons/make-icons.sh`.

## Not verified here (known gaps)

- **Signing and notarization**: no Developer ID certificate or notary credentials on this machine. The script is
  Bufflehead's proven flow adapted (names, paths, DMG), but its first real run will be in CI with the secrets set.
  `disable-library-validation` is kept in the entitlements to match Godot's preset; try dropping it then.
- **Windows and Linux builds**: `bin/build-windows` and `bin/build-linux` have not been run (no Windows or Linux
  machine, no `zig` for a cross build). They follow Bufflehead's working scripts; expect the first CI run to need
  small fixes (MinGW runtime DLL bundling, rcedit, appimagetool download, Inno Setup path).
- **The workflows** have not run yet (nothing was pushed from this branch). A `workflow_dispatch` run is the dry
  run to do first.
- No styled DMG background (plain `hdiutil` image), no `.fbs` file association in the installers, no Linux
  `.desktop` validation beyond `desktop-file-utils` being installed in CI.
