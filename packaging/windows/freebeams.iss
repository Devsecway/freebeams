; Inno Setup script: packages the Windows bundle (releases\windows\amd64\:
; FreeBeams.exe + windows_amd64.dll + ffmpeg/ffprobe + the licence files) into
; FreeBeams-<version>-Setup.exe.
;
; A graphics.gd app is multi-file: FreeBeams.exe loads windows_amd64.dll (the
; Go side of the app) from its own folder. The installer keeps them together so
; the exe is never moved away from its library.
;
; What it does:
;   - shows the MIT licence (LICENSE) and installs per user without admin
;     rights by default (an administrator may choose all users);
;   - Start-menu shortcut, optional desktop shortcut;
;   - registers .fbs (schedules), the legacy .fws, and .fbbible (Bible
;     packages) with their own icons and "Open with FreeBeams"; FreeBeams opens
;     a schedule, or the Bible Manager with the package (internal/app/openfiles.go);
;   - the uninstaller removes the program, the file associations and the
;     cache, and keeps the library (songs, Bibles, themes, settings) unless the
;     user says to remove it too. Documents\FreeBeams (the Bibles folder) is
;     the user's own and always stays.
;
; Built by bin/build-windows:
;   iscc /DAppVersion=<x.y.z> /DSourceDir=<bundle> /O<outdir> packaging\windows\freebeams.iss

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceDir
  #define SourceDir SourcePath + "\..\..\releases\windows\amd64"
#endif
#define Art SourcePath
#define Root SourcePath + "\..\.."
#define AppURL "https://github.com/Devsecway/freebeams"

[Setup]
; Stable AppId so a newer version upgrades the existing install in place.
AppId={{73EBEA48-4AC6-45E6-93A8-A184C83D15F5}
AppName=FreeBeams
AppVersion={#AppVersion}
AppVerName=FreeBeams {#AppVersion}
AppPublisher=Devsecway
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
AppCopyright=Copyright (c) 2026 Devsecway
AppComments=Free worship presentation software
VersionInfoVersion={#AppVersion}.0
VersionInfoCompany=Devsecway
VersionInfoDescription=FreeBeams {#AppVersion} Setup
VersionInfoProductName=FreeBeams
VersionInfoProductVersion={#AppVersion}
VersionInfoCopyright=Copyright (c) 2026 Devsecway
DefaultDirName={autopf}\FreeBeams
DefaultGroupName=FreeBeams
DisableProgramGroupPage=yes
LicenseFile={#Root}\LICENSE
; Per-user install by default (church laptops often have no admin rights): with
; PrivilegesRequired=lowest the {auto*} constants and HKA resolve to per-user
; locations. Someone with admin rights can still choose an all-users install.
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog
UninstallDisplayIcon={app}\FreeBeams.exe
UninstallDisplayName=FreeBeams {#AppVersion}
OutputBaseFilename=FreeBeams-{#AppVersion}-Setup
SetupIconFile={#Root}\graphics\icon.ico
WizardStyle=modern
WizardImageFile={#Art}\wizard-large-100.bmp,{#Art}\wizard-large-125.bmp,{#Art}\wizard-large-150.bmp,{#Art}\wizard-large-175.bmp,{#Art}\wizard-large-200.bmp
WizardSmallImageFile={#Art}\wizard-small-100.bmp,{#Art}\wizard-small-125.bmp,{#Art}\wizard-small-150.bmp,{#Art}\wizard-small-175.bmp,{#Art}\wizard-small-200.bmp
Compression=lzma2/ultra64
SolidCompression=yes
LZMAUseSeparateProcess=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
ChangesAssociations=yes
CloseApplications=yes
RestartApplications=no

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional shortcuts:"; Flags: unchecked
Name: "associate"; Description: "Open FreeBeams &schedules (.fbs) and Bible packages (.fbbible) with FreeBeams"; GroupDescription: "File types:"

[Files]
Source: "{#SourceDir}\*"; DestDir: "{app}"; Flags: recursesubdirs ignoreversion
Source: "{#Art}\schedule.ico"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#Art}\bible.ico"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\FreeBeams"; Filename: "{app}\FreeBeams.exe"; Comment: "Free worship presentation software"
Name: "{group}\FreeBeams Licence"; Filename: "{app}\LICENSE.txt"
Name: "{group}\Uninstall FreeBeams"; Filename: "{uninstallexe}"
Name: "{autodesktop}\FreeBeams"; Filename: "{app}\FreeBeams.exe"; Tasks: desktopicon

[Registry]
; ProgIDs: the icon and the open command of each file type.
Root: HKA; Subkey: "Software\Classes\FreeBeams.Schedule"; ValueType: string; ValueName: ""; ValueData: "FreeBeams Schedule"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Classes\FreeBeams.Schedule\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\schedule.ico,0"
Root: HKA; Subkey: "Software\Classes\FreeBeams.Schedule\shell\open"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "FreeBeams"
Root: HKA; Subkey: "Software\Classes\FreeBeams.Schedule\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\FreeBeams.exe"" ""%1"""
Root: HKA; Subkey: "Software\Classes\FreeBeams.BiblePackage"; ValueType: string; ValueName: ""; ValueData: "FreeBeams Bible Package"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Classes\FreeBeams.BiblePackage\DefaultIcon"; ValueType: string; ValueName: ""; ValueData: "{app}\bible.ico,0"
Root: HKA; Subkey: "Software\Classes\FreeBeams.BiblePackage\shell\open"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "FreeBeams"
Root: HKA; Subkey: "Software\Classes\FreeBeams.BiblePackage\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\FreeBeams.exe"" ""%1"""
; The extensions are FreeBeams' own: it is their default handler when the
; task is chosen, and always in their "Open with" list.
Root: HKA; Subkey: "Software\Classes\.fbs"; ValueType: string; ValueName: ""; ValueData: "FreeBeams.Schedule"; Flags: uninsdeletevalue; Tasks: associate
Root: HKA; Subkey: "Software\Classes\.fbs"; ValueType: string; ValueName: "Content Type"; ValueData: "application/x-freebeams-schedule"; Flags: uninsdeletevalue; Tasks: associate
Root: HKA; Subkey: "Software\Classes\.fbs\OpenWithProgids"; ValueType: string; ValueName: "FreeBeams.Schedule"; ValueData: ""; Flags: uninsdeletevalue
Root: HKA; Subkey: "Software\Classes\.fws"; ValueType: string; ValueName: ""; ValueData: "FreeBeams.Schedule"; Flags: uninsdeletevalue; Tasks: associate
Root: HKA; Subkey: "Software\Classes\.fws\OpenWithProgids"; ValueType: string; ValueName: "FreeBeams.Schedule"; ValueData: ""; Flags: uninsdeletevalue
Root: HKA; Subkey: "Software\Classes\.fbbible"; ValueType: string; ValueName: ""; ValueData: "FreeBeams.BiblePackage"; Flags: uninsdeletevalue; Tasks: associate
Root: HKA; Subkey: "Software\Classes\.fbbible"; ValueType: string; ValueName: "Content Type"; ValueData: "application/x-freebeams-bible"; Flags: uninsdeletevalue; Tasks: associate
Root: HKA; Subkey: "Software\Classes\.fbbible\OpenWithProgids"; ValueType: string; ValueName: "FreeBeams.BiblePackage"; ValueData: ""; Flags: uninsdeletevalue
; "Open with" names FreeBeams for these types.
Root: HKA; Subkey: "Software\Classes\Applications\FreeBeams.exe"; ValueType: string; ValueName: "FriendlyAppName"; ValueData: "FreeBeams"; Flags: uninsdeletekey
Root: HKA; Subkey: "Software\Classes\Applications\FreeBeams.exe\SupportedTypes"; ValueType: string; ValueName: ".fbs"; ValueData: ""
Root: HKA; Subkey: "Software\Classes\Applications\FreeBeams.exe\SupportedTypes"; ValueType: string; ValueName: ".fws"; ValueData: ""
Root: HKA; Subkey: "Software\Classes\Applications\FreeBeams.exe\SupportedTypes"; ValueType: string; ValueName: ".fbbible"; ValueData: ""
Root: HKA; Subkey: "Software\Classes\Applications\FreeBeams.exe\shell\open\command"; ValueType: string; ValueName: ""; ValueData: """{app}\FreeBeams.exe"" ""%1"""

[Run]
Filename: "{app}\FreeBeams.exe"; Description: "Launch FreeBeams"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Derived files only (thumbnails, posters, transcodes): always safe to remove.
Type: filesandordirs; Name: "{localappdata}\FreeBeams"

[Code]
// The library (songs, Bibles, themes, settings) lives in %APPDATA%\FreeBeams.
// It stays unless the user asks for it to go, so a reinstall or an upgrade
// finds it again.
procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Data: String;
begin
  if CurUninstallStep <> usPostUninstall then
    exit;
  Data := ExpandConstant('{userappdata}\FreeBeams');
  if (not DirExists(Data)) or UninstallSilent then
    exit;
  if MsgBox('Also remove your FreeBeams library?' + #13#10 + #13#10 +
      'It holds your songs, installed Bibles, themes and settings (' + Data + ').' + #13#10 +
      'Choose No to keep it for a later install. Your Documents\FreeBeams folder is not touched.',
      mbConfirmation, MB_YESNO or MB_DEFBUTTON2) = IDYES then
  begin
    if not DelTree(Data, True, True, True) then
      MsgBox('Some files in ' + Data + ' could not be removed.', mbError, MB_OK);
  end;
end;
