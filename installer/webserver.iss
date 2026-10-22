; Inno Setup Script for Webserver Stack
; Targets: Windows 10/11 x64

#define MyAppName "webserver"
#define MyAppVersion "0.1.0"
#define MyAppPublisher "Chakron Work"
#define MyAppURL "https://github.com/chakronwork/webserver-stack"
#define MyAppExeName "webserver.exe"

[Setup]
AppId={{9C89B15E-5D3E-4B07-B459-00B2939B23E4}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
OutputDir=..\dist\installer
OutputBaseFilename=webserver-setup
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=lowest
PrivilegesRequiredOverridesAllowed=dialog

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Files]
; Main executable & configurations
Source: "..\webserver.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\stack.toml"; DestDir: "{app}"; Flags: onlyifdoesntexist
Source: "..\versions.toml"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\templates\*"; DestDir: "{app}\templates"; Flags: ignoreversion recursesubdirs createallsubdirs
Source: "..\licenses\*"; DestDir: "{app}\licenses"; Flags: ignoreversion recursesubdirs createallsubdirs

[Dirs]
Name: "{app}\bin"
Name: "{app}\conf"
Name: "{app}\conf\vhosts"
Name: "{app}\conf\ssl"
Name: "{app}\www"
Name: "{app}\data"
Name: "{app}\data\mysql"
Name: "{app}\logs"
Name: "{app}\tmp"
Name: "{app}\sessions"
Name: "{app}\backups"

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\{#MyAppName} Doctor"; Filename: "{app}\{#MyAppExeName}"; Parameters: "doctor"
Name: "{group}\{cm:UninstallProgram,{#MyAppName}}"; Filename: "{uninstallexe}"
Name: "{autodesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; Description: "{cm:LaunchProgram,{#StringChange(MyAppName, '&', '&&')}}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; Only clean up temporary/generated cache files. NEVER delete data/, www/, or user stack.toml!
Type: filesandordirs; Name: "{app}\tmp"
Type: filesandordirs; Name: "{app}\logs"
Type: filesandordirs; Name: "{app}\sessions"

[Code]
// Check if Visual C++ 2015-2022 (x64) is installed
function IsVCRedistInstalled: Boolean;
var
  Installed: Cardinal;
begin
  Result := False;
  if RegQueryDWordValue(HKLM, 'SOFTWARE\Microsoft\VisualStudio\14.0\VC\Runtimes\X64', 'Installed', Installed) then
  begin
    Result := (Installed = 1);
  end;
end;

function InitializeSetup(): Boolean;
var
  ErrorCode: Integer;
begin
  Result := True;
  if not IsVCRedistInstalled then
  begin
    if MsgBox('Visual C++ 2015-2022 (x64) Redistributable is required but was not detected.' + #13#10 +
              'Would you like to open the official download page now?', mbConfirmation, MB_YESNO) = IDYES then
    begin
      ShellExec('open', 'https://aka.ms/vs/17/release/vc_redist.x64.exe', '', '', SW_SHOWNORMAL, ewNoWait, ErrorCode);
    end;
  end;
end;
