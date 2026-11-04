<#
.SYNOPSIS
  Builds vpn_specify and, optionally, the package with the installer.

.DESCRIPTION
  Always builds, in the project root (next to data\ and windivert\):
    vpn_specify.exe      graphical interface (no console)
    vpn_specify-cli.exe  terminal commands (up, run, status, update)

  The version comes from the VERSION file.

.PARAMETER Test
  Runs go vet and the tests before building.

.PARAMETER Icon
  Regenerates the icon/version resources (rsrc_windows_amd64.syso of
  cmd\vpn_specify and cmd\installer) from assets\resource.rc.tmpl.
  Needs windres (MinGW). Use it after changing VERSION or the icon.

.PARAMETER Dist
  Also builds dist\vpn_specify-<version>-win64.zip with:
    vpn_specify-setup.exe  offline installer (program embedded)
    README.txt             instructions and requirements
    LICENSE.txt            vpn_specify license (MIT)
    licenses\              licenses of the third-party components

.PARAMETER Linux
  Also builds the Linux version (GUI and terminal; amd64 and arm64):
  dist\vpn_specify-<version>-linux-<arch>.tar.gz, with the vpn_specify
  executable, LICENSE and licenses\.

.PARAMETER Android
  Also builds dist\vpn_specify-<version>-android-arm64.apk. Needs the Android
  SDK and NDK. Signed with android\keystore.properties if it exists; otherwise
  with the debug key, which is only for testing.

.EXAMPLE
  .\scripts\build.ps1 -Test -Dist -Linux -Android
#>
param(
    [switch]$Test,
    [switch]$Icon,
    [switch]$Dist,
    [switch]$Linux,
    [switch]$Android
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$utf8 = New-Object System.Text.UTF8Encoding $false
# the Windows tar (bsdtar), not Git's GNU tar: it writes zip and reads mtree
$bsdtar = Join-Path $env:SystemRoot 'System32\tar.exe'

function Invoke-Native {
    param([string]$Exe, [string[]]$Arguments)
    & $Exe @Arguments
    if ($LASTEXITCODE) { throw "$Exe $($Arguments -join ' ') failed (exit code $LASTEXITCODE)" }
}

function Write-Template {
    param([string]$Template, [string]$Out, [hashtable]$Values)
    $text = [IO.File]::ReadAllText($Template, $utf8)
    foreach ($k in $Values.Keys) { $text = $text.Replace("{{$k}}", $Values[$k]) }
    [IO.File]::WriteAllText($Out, $text, $utf8)
}

# Copy-ThirdPartyLicenses copies the license files of the Go modules used by
# the given packages into $dest.
function Copy-ThirdPartyLicenses {
    param([string]$Dest, [string[]]$Packages)
    $mods = & go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}{{end}}' @Packages
    if ($LASTEXITCODE) { throw 'go list failed' }
    foreach ($m in ($mods | Where-Object { $_ } | Sort-Object -Unique)) {
        $path, $dir = $m -split '\|', 2
        foreach ($l in (Get-ChildItem $dir -File | Where-Object { $_.Name -match '^(LICEN[CS]E|COPYING|NOTICE)' })) {
            $suffix = if ($l.Name -match '^NOTICE') { '-NOTICE' } else { '' }
            Copy-Item $l.FullName (Join-Path $Dest (($path -replace '[/\\:]', '_') + "$suffix.txt"))
        }
    }
}

Push-Location $root
try {
    $version = ([IO.File]::ReadAllText((Join-Path $root 'VERSION'))).Trim()
    if ($version -notmatch '^\d+\.\d+\.\d+$') { throw "invalid VERSION: '$version' (use the 1.2.3 format)" }
    $ldVersion = "-X vpn_specify/internal/version.Version=$version"
    Write-Host "vpn_specify $version" -ForegroundColor Cyan

    if ($Icon) {
        $values = @{ VERSION = $version; VERSION_COMMA = (($version -split '\.') + '0') -join ',' }
        $targets = @(
            @{ Dir = 'cmd/vpn_specify'; Description = 'vpn_specify - VPN only for the apps you choose'; File = 'vpn_specify.exe' },
            @{ Dir = 'cmd/installer'; Description = 'vpn_specify installer'; File = 'vpn_specify-setup.exe' }
        )
        foreach ($t in $targets) {
            $rc = Join-Path $root 'assets\_build.rc'   # inside assets so the .ico is found
            Write-Template 'assets\resource.rc.tmpl' $rc ($values + @{ DESCRIPTION = $t.Description; FILENAME = $t.File })
            try {
                Invoke-Native windres @('-c', '65001', '-O', 'coff', '-F', 'pe-x86-64', '-i', $rc, '-o', "$($t.Dir)/rsrc_windows_amd64.syso")
            } finally { Remove-Item $rc -ErrorAction SilentlyContinue }
        }
        Write-Host 'icon/version resources regenerated'
    }

    if ($Test) {
        Invoke-Native go @('vet', './...')
        Invoke-Native go @('test', './...')
    }

    Invoke-Native go @('build', '-trimpath', '-ldflags', "-s -w -H=windowsgui $ldVersion", '-o', 'vpn_specify.exe', './cmd/vpn_specify')
    Invoke-Native go @('build', '-trimpath', '-ldflags', "-s -w $ldVersion", '-o', 'vpn_specify-cli.exe', './cmd/vpn_specify')
    Get-Item vpn_specify.exe, vpn_specify-cli.exe | Format-Table Name, Length, LastWriteTime -AutoSize

    if ($Linux) {
        # a single executable (GUI and terminal), without cgo; the installer
        # and -capture are Windows-only
        $saved = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
        try {
            foreach ($arch in 'amd64', 'arm64') {
                $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = 'linux', $arch, '0'
                $name = "vpn_specify-$version-linux-$arch"
                $stage = Join-Path 'dist' $name
                Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
                New-Item -ItemType Directory -Force "$stage\licenses" | Out-Null
                Invoke-Native go @('build', '-trimpath', '-ldflags', "-s -w $ldVersion", '-o', "$stage\vpn_specify", './cmd/vpn_specify')
                Copy-Item LICENSE "$stage\LICENSE"
                Copy-ThirdPartyLicenses "$stage\licenses" @('./cmd/vpn_specify')
                $tgz = Join-Path 'dist' "$name.tar.gz"
                Remove-Item $tgz -ErrorAction SilentlyContinue
                # bsdtar stores everything as 0666, without the +x bit. An
                # mtree spec sets the owner and permissions of each entry.
                $distAbs = (Resolve-Path 'dist').Path
                $spec = @('#mtree', "./$name type=dir mode=0755 uid=0 gid=0")
                foreach ($item in (Get-ChildItem $stage -Recurse | Sort-Object FullName)) {
                    $rel = './' + $item.FullName.Substring($distAbs.Length + 1).Replace('\', '/')
                    if ($item.PSIsContainer) {
                        $spec += "$rel type=dir mode=0755 uid=0 gid=0"
                    } else {
                        $mode = if ($item.Name -eq 'vpn_specify') { '0755' } else { '0644' }
                        $spec += "$rel type=file mode=$mode uid=0 gid=0 contents=$rel"
                    }
                }
                $specFile = Join-Path $distAbs "$name.mtree"
                [IO.File]::WriteAllText($specFile, (($spec -join "`n") + "`n"), $utf8)
                try {
                    Invoke-Native $bsdtar @('-czf', $tgz, '-C', 'dist', "@$specFile")
                } finally { Remove-Item $specFile -ErrorAction SilentlyContinue }
                Write-Host "Linux ${arch}: $((Resolve-Path $tgz).Path)" -ForegroundColor Green
            }
        } finally {
            $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $saved
        }
    }

    if ($Android) {
        $savedJava, $savedSdk = $env:JAVA_HOME, $env:ANDROID_HOME
        try {
            # Gradle needs a JDK it supports: the one in Android Studio is one
            $studioJdk = Join-Path $env:ProgramFiles 'Android\Android Studio\jbr'
            if (-not $env:JAVA_HOME -and (Test-Path $studioJdk)) { $env:JAVA_HOME = $studioJdk }
            if (-not $env:ANDROID_HOME) { $env:ANDROID_HOME = Join-Path $env:LOCALAPPDATA 'Android\Sdk' }
            Push-Location android
            try {
                Invoke-Native '.\gradlew.bat' @('assembleRelease', '--console=plain')
            } finally { Pop-Location }
            New-Item -ItemType Directory -Force dist | Out-Null
            $apk = Join-Path 'dist' "vpn_specify-$version-android-arm64.apk"
            Copy-Item 'android\app\build\outputs\apk\release\app-release.apk' $apk -Force
            Write-Host "Android: $((Resolve-Path $apk).Path) ($([math]::Round((Get-Item $apk).Length / 1MB, 1)) MB)" -ForegroundColor Green
            if (-not (Test-Path 'android\keystore.properties')) {
                Write-Warning 'Signed with the debug key: for testing only. To publish, see android\keystore.properties.example.'
            }
        } finally {
            $env:JAVA_HOME, $env:ANDROID_HOME = $savedJava, $savedSdk
        }
    }

    if ($Dist) {
        $windivert = 'WinDivert.dll', 'WinDivert64.sys', 'LICENSE' | ForEach-Object { Join-Path 'windivert' $_ }
        foreach ($f in $windivert) {
            if (-not (Test-Path $f)) { throw "$f is missing (download WinDivert 2.2.2 x64; see the README)" }
        }

        # 1. files the installer copies (embedded with go:embed)
        $payload = 'cmd\installer\payload'
        Remove-Item $payload -Recurse -Force -ErrorAction SilentlyContinue
        New-Item -ItemType Directory -Force "$payload\windivert" | Out-Null
        Copy-Item vpn_specify.exe, vpn_specify-cli.exe $payload
        Copy-Item $windivert "$payload\windivert"
        $license = ([IO.File]::ReadAllText((Join-Path $root 'LICENSE'), $utf8)) -replace "`r?`n", "`r`n"
        [IO.File]::WriteAllText((Join-Path $root "$payload\LICENSE.txt"), $license, $utf8)

        # 2. installer
        $name = "vpn_specify-$version-win64"
        $stage = Join-Path 'dist' $name
        Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
        New-Item -ItemType Directory -Force "$stage\licenses" | Out-Null
        try {
            Invoke-Native go @('build', '-tags', 'installer', '-trimpath', '-ldflags', "-s -w -H=windowsgui $ldVersion",
                '-o', "$stage\vpn_specify-setup.exe", './cmd/installer')
        } finally { Remove-Item $payload -Recurse -Force -ErrorAction SilentlyContinue }

        # 3. README (CRLF, for Notepad) and licenses
        $readme = Join-Path $stage 'README.txt'
        Write-Template 'assets\dist\README.txt' $readme @{ VERSION = $version }
        [IO.File]::WriteAllText($readme, ([IO.File]::ReadAllText($readme, $utf8) -replace "`r?`n", "`r`n"), $utf8)
        [IO.File]::WriteAllText((Join-Path $root "$stage\LICENSE.txt"), $license, $utf8)
        Copy-Item windivert\LICENSE "$stage\licenses\WinDivert.txt"
        Copy-ThirdPartyLicenses "$stage\licenses" @('./cmd/vpn_specify', './cmd/installer')

        # 4. zip (-a: format from the .zip name). PowerShell 5.1's
        # Compress-Archive writes paths with "\", which breaks the zip standard.
        $zip = Join-Path 'dist' "$name.zip"
        Remove-Item $zip -ErrorAction SilentlyContinue
        Invoke-Native $bsdtar @('-a', '-c', '-f', $zip, '-C', 'dist', $name)
        Write-Host ''
        Get-ChildItem $stage -Recurse -File | Select-Object @{ n = 'File'; e = { $_.FullName.Substring((Resolve-Path $stage).Path.Length + 1) } }, Length | Format-Table -AutoSize
        Write-Host "Package: $((Resolve-Path $zip).Path) ($([math]::Round((Get-Item $zip).Length / 1MB, 1)) MB)" -ForegroundColor Green
    }
}
finally {
    Pop-Location
}
