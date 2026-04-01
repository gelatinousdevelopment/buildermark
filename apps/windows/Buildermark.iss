#ifndef AppName
  #define AppName "Buildermark"
#endif

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif

#ifndef AppId
  #define AppId "{{C6E73D0E-070A-42E3-8C94-57A729117A08}"
#endif

#ifndef PublishDir
  #error "PublishDir must be defined"
#endif

#ifndef OutputDir
  #error "OutputDir must be defined"
#endif

#ifndef OutputBaseFilename
  #define OutputBaseFilename "Buildermark-Setup"
#endif

#ifndef ArchitecturesAllowed
  #define ArchitecturesAllowed "x64compatible"
#endif

#ifndef ArchitecturesInstallIn64BitMode
  #define ArchitecturesInstallIn64BitMode "x64compatible"
#endif

#ifndef EnableSigning
  #define EnableSigning "yes"
#endif

[Setup]
AppId={#AppId}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher=Gelatinous Development Studio
DefaultDirName={localappdata}\Programs\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
UninstallDisplayIcon={app}\Buildermark.exe
SetupIconFile=Buildermark\Resources\buildermark.ico
OutputDir={#OutputDir}
OutputBaseFilename={#OutputBaseFilename}
Compression=lzma
SolidCompression=yes
PrivilegesRequired=lowest
ArchitecturesAllowed={#ArchitecturesAllowed}
ArchitecturesInstallIn64BitMode={#ArchitecturesInstallIn64BitMode}
WizardStyle=modern
VersionInfoCompany=Gelatinous Development Studio
VersionInfoDescription={#AppName} Installer
#if EnableSigning == "yes"
SignedUninstaller=yes
SignTool=buildermark
#endif

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "startup"; Description: "Run {#AppName} when you sign in"; GroupDescription: "Additional tasks:"; Flags: unchecked

[Files]
Source: "{#PublishDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\Buildermark.exe"; WorkingDir: "{app}"
Name: "{group}\Uninstall {#AppName}"; Filename: "{uninstallexe}"

[Registry]
Root: HKCU; Subkey: "Software\Buildermark"; ValueType: dword; ValueName: "startAtLogin"; ValueData: "1"; Flags: uninsdeletevalue; Check: WizardIsTaskSelected('startup')
Root: HKCU; Subkey: "Software\Buildermark"; ValueType: dword; ValueName: "startAtLogin"; ValueData: "0"; Flags: uninsdeletevalue; Check: not WizardIsTaskSelected('startup')
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "Buildermark"; ValueData: """{app}\Buildermark.exe"""; Flags: uninsdeletevalue; Tasks: startup
