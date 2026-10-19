; Inno Setup script: packages the Windows bundle (releases\windows\amd64\:
; FreeWorship.exe + windows_amd64.dll + any runtime DLLs) into one Setup.exe
; with Start Menu / desktop shortcuts and an uninstaller.
;
; A graphics.gd app is multi-file: FreeWorship.exe loads windows_amd64.dll (the
; Go side of the app) from its own folder. The installer keeps them together so
; the exe is never moved away from its library.
;
; Built by bin/build-windows:
;   iscc /DAppVersion=<x.y.z> /O<outdir> packaging\windows\freeworship.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir SourcePath + "\..\..\releases\windows\amd64"
#endif

[Setup]
; Stable AppId so a newer version upgrades the existing install in place.
AppId={{73EBEA48-4AC6-45E6-93A8-A184C83D15F5}
AppName=FreeWorship
AppVersion={#AppVersion}
AppVerName=FreeWorship {#AppVersion}
AppPublisher=FreeWorship contributors
AppPublisherURL=https://github.com/Devsecway/freeworship
DefaultDirName={autopf}\FreeWorship
DefaultGroupName=FreeWorship
DisableProgramGroupPage=yes
; Per-user install by default (church laptops often have no admin rights): with
; PrivilegesRequired=lowest the {auto*} constants resolve to per-user locations.
; Someone with admin rights can still choose an all-users install.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
UninstallDisplayIcon={app}\FreeWorship.exe
OutputBaseFilename=FreeWorship-{#AppVersion}-Setup
#ifdef IconFile
SetupIconFile={#IconFile}
#endif
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional icons:"

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: recursesubdirs ignoreversion

[Icons]
Name: "{group}\FreeWorship"; Filename: "{app}\FreeWorship.exe"
Name: "{group}\Uninstall FreeWorship"; Filename: "{uninstallexe}"
Name: "{autodesktop}\FreeWorship"; Filename: "{app}\FreeWorship.exe"; Tasks: desktopicon

[Run]
Filename: "{app}\FreeWorship.exe"; Description: "Launch FreeWorship"; Flags: nowait postinstall skipifsilent
