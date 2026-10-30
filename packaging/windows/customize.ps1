# Ajusta os dados do produto (propriedades do .exe) e o instalador NSIS do projeto base do Wails.
# Só usa caracteres ASCII: o NSIS lê o script na página de código local e acentos viram lixo.
param([Parameter(Mandatory)][string]$Version, [Parameter(Mandatory)][string]$Project)

$utf8 = New-Object System.Text.UTF8Encoding $false

$info = @"
{
	"fixed": {
		"file_version": "$Version"
	},
	"info": {
		"0409": {
			"ProductVersion": "$Version",
			"CompanyName": "Alexandre Garcia",
			"FileDescription": "NxATerm",
			"LegalCopyright": "(c) 2026 Alexandre Garcia",
			"ProductName": "NxATerm",
			"Comments": "SSH, SFTP, Telnet, Serial e shell local"
		}
	}
}
"@
[IO.File]::WriteAllText("$Project\build\windows\info.json", $info, $utf8)

$nsiPath = "$Project\build\windows\nsis\project.nsi"
$nsi = [IO.File]::ReadAllText($nsiPath)

$defs = @"
!define INFO_PROJECTNAME    "nxaterm"
!define INFO_COMPANYNAME    "Alexandre Garcia"
!define INFO_PRODUCTNAME    "NxATerm"
!define INFO_PRODUCTVERSION "$Version"
!define INFO_COPYRIGHT      "(c) 2026 Alexandre Garcia"
!define UNINST_KEY_NAME     "NxATerm"

"@
$checks = @(
  @{ From = '!include "wails_tools.nsh"'; To = $defs + '!include "wails_tools.nsh"' },
  @{ From = '!insertmacro MUI_LANGUAGE "English"'; To = '!insertmacro MUI_LANGUAGE "PortugueseBR"' },
  @{ From = '!define MUI_ABORTWARNING'; To = "!define MUI_ABORTWARNING`r`n!define MUI_FINISHPAGE_RUN `"`$INSTDIR\`${PRODUCT_EXECUTABLE}`"" }
)
foreach ($c in $checks) {
  if (-not $nsi.Contains($c.From)) { throw "trecho nao encontrado em project.nsi: $($c.From)" }
  $nsi = $nsi.Replace($c.From, $c.To)
}
[IO.File]::WriteAllText($nsiPath, $nsi, $utf8)
Get-Content $nsiPath | Select-String -Pattern 'INFO_|UNINST_KEY|MUI_LANGUAGE|MUI_FINISHPAGE_RUN|include'
