<#
.SYNOPSIS
  Compila o vpn_specify e, opcionalmente, gera o pacote com o instalador.

.DESCRIPTION
  Sempre gera, na raiz do projeto (ao lado de data\ e windivert\):
    vpn_specify.exe      interface gráfica (sem console)
    vpn_specify-cli.exe  comandos de terminal (up, run, status)

  A versão vem do arquivo VERSION.

.PARAMETER Test
  Roda go vet e os testes antes de compilar.

.PARAMETER Icon
  Regenera os recursos de ícone/versão (rsrc_windows_amd64.syso de
  cmd\vpn_specify e cmd\installer) a partir de assets\resource.rc.tmpl.
  Precisa do windres (MinGW). Use depois de mudar VERSION ou o ícone.

.PARAMETER Dist
  Gera também dist\vpn_specify-<versão>-win64.zip com:
    vpn_specify-setup.exe  instalador offline (programa embutido)
    LEIA-ME.txt            instruções e requisitos
    LICENSE.txt            licença do vpn_specify (MIT)
    licencas\              licenças dos componentes de terceiros

.PARAMETER Linux
  Gera também a versão para Linux (interface e terminal; amd64 e arm64):
  dist\vpn_specify-<versão>-linux-<arq>.tar.gz, com o executável
  vpn_specify, LICENSE e licencas\.

.EXAMPLE
  .\scripts\build.ps1 -Test -Dist -Linux
#>
param(
    [switch]$Test,
    [switch]$Icon,
    [switch]$Dist,
    [switch]$Linux
)
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$utf8 = New-Object System.Text.UTF8Encoding $false

function Invoke-Native {
    param([string]$Exe, [string[]]$Arguments)
    & $Exe @Arguments
    if ($LASTEXITCODE) { throw "$Exe $($Arguments -join ' ') falhou (código $LASTEXITCODE)" }
}

function Write-Template {
    param([string]$Template, [string]$Out, [hashtable]$Values)
    $text = [IO.File]::ReadAllText($Template, $utf8)
    foreach ($k in $Values.Keys) { $text = $text.Replace("{{$k}}", $Values[$k]) }
    [IO.File]::WriteAllText($Out, $text, $utf8)
}

Push-Location $root
try {
    $version = ([IO.File]::ReadAllText((Join-Path $root 'VERSION'))).Trim()
    if ($version -notmatch '^\d+\.\d+\.\d+$') { throw "VERSION inválida: '$version' (use o formato 1.2.3)" }
    $ldVersion = "-X vpn_specify/internal/version.Version=$version"
    Write-Host "vpn_specify $version" -ForegroundColor Cyan

    if ($Icon) {
        $values = @{ VERSION = $version; VERSION_COMMA = (($version -split '\.') + '0') -join ',' }
        $targets = @(
            @{ Dir = 'cmd/vpn_specify'; Description = 'vpn_specify - VPN só para os apps escolhidos'; File = 'vpn_specify.exe' },
            @{ Dir = 'cmd/installer'; Description = 'Instalador do vpn_specify'; File = 'vpn_specify-setup.exe' }
        )
        foreach ($t in $targets) {
            $rc = Join-Path $root 'assets\_build.rc'   # na pasta assets para achar o .ico
            Write-Template 'assets\resource.rc.tmpl' $rc ($values + @{ DESCRIPTION = $t.Description; FILENAME = $t.File })
            try {
                Invoke-Native windres @('-c', '65001', '-O', 'coff', '-F', 'pe-x86-64', '-i', $rc, '-o', "$($t.Dir)/rsrc_windows_amd64.syso")
            } finally { Remove-Item $rc -ErrorAction SilentlyContinue }
        }
        Write-Host 'recursos de ícone/versão regenerados'
    }

    if ($Test) {
        Invoke-Native go @('vet', './...')
        Invoke-Native go @('test', './...')
    }

    Invoke-Native go @('build', '-trimpath', '-ldflags', "-s -w -H=windowsgui $ldVersion", '-o', 'vpn_specify.exe', './cmd/vpn_specify')
    Invoke-Native go @('build', '-trimpath', '-ldflags', "-s -w $ldVersion", '-o', 'vpn_specify-cli.exe', './cmd/vpn_specify')
    Get-Item vpn_specify.exe, vpn_specify-cli.exe | Format-Table Name, Length, LastWriteTime -AutoSize

    if ($Linux) {
        # um executável só (interface gráfica e terminal), sem cgo; o
        # instalador e o -capture são exclusivos do Windows
        $saved = $env:GOOS, $env:GOARCH, $env:CGO_ENABLED
        try {
            foreach ($arch in 'amd64', 'arm64') {
                $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = 'linux', $arch, '0'
                $name = "vpn_specify-$version-linux-$arch"
                $stage = Join-Path 'dist' $name
                Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
                New-Item -ItemType Directory -Force "$stage\licencas" | Out-Null
                Invoke-Native go @('build', '-trimpath', '-ldflags', "-s -w $ldVersion", '-o', "$stage\vpn_specify", './cmd/vpn_specify')
                Copy-Item LICENSE "$stage\LICENSE"
                $mods = & go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}{{end}}' ./cmd/vpn_specify
                if ($LASTEXITCODE) { throw 'go list falhou' }
                foreach ($m in ($mods | Where-Object { $_ } | Sort-Object -Unique)) {
                    $path, $dir = $m -split '\|', 2
                    foreach ($l in (Get-ChildItem $dir -File | Where-Object { $_.Name -match '^(LICEN[CS]E|COPYING|NOTICE)' })) {
                        $suffix = if ($l.Name -match '^NOTICE') { '-NOTICE' } else { '' }
                        Copy-Item $l.FullName ("$stage\licencas\" + ($path -replace '[/\\:]', '_') + "$suffix.txt")
                    }
                }
                $tgz = Join-Path 'dist' "$name.tar.gz"
                Remove-Item $tgz -ErrorAction SilentlyContinue
                # O tar do Windows (bsdtar) grava tudo como 0666, sem o bit +x.
                # Uma especificação mtree define dono e permissões de cada item.
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
                    # o tar do Windows (bsdtar), não o GNU tar do Git, que não lê mtree
                    $bsdtar = Join-Path $env:SystemRoot 'System32\tar.exe'
                    Invoke-Native $bsdtar @('-czf', $tgz, '-C', 'dist', "@$specFile")
                } finally { Remove-Item $specFile -ErrorAction SilentlyContinue }
                Write-Host "Linux ${arch}: $((Resolve-Path $tgz).Path)" -ForegroundColor Green
            }
        } finally {
            $env:GOOS, $env:GOARCH, $env:CGO_ENABLED = $saved
        }
    }

    if ($Dist) {
        $windivert = 'WinDivert.dll', 'WinDivert64.sys', 'LICENSE' | ForEach-Object { Join-Path 'windivert' $_ }
        foreach ($f in $windivert) {
            if (-not (Test-Path $f)) { throw "falta $f (baixe o WinDivert 2.2.2 x64; veja o README)" }
        }

        # 1. arquivos que o instalador vai copiar (embutidos via go:embed)
        $payload = 'cmd\installer\payload'
        Remove-Item $payload -Recurse -Force -ErrorAction SilentlyContinue
        New-Item -ItemType Directory -Force "$payload\windivert" | Out-Null
        Copy-Item vpn_specify.exe, vpn_specify-cli.exe $payload
        Copy-Item $windivert "$payload\windivert"
        $license = ([IO.File]::ReadAllText((Join-Path $root 'LICENSE'), $utf8)) -replace "`r?`n", "`r`n"
        [IO.File]::WriteAllText((Join-Path $root "$payload\LICENSE.txt"), $license, $utf8)

        # 2. instalador
        $name = "vpn_specify-$version-win64"
        $stage = Join-Path 'dist' $name
        Remove-Item $stage -Recurse -Force -ErrorAction SilentlyContinue
        New-Item -ItemType Directory -Force "$stage\licencas" | Out-Null
        try {
            Invoke-Native go @('build', '-tags', 'installer', '-trimpath', '-ldflags', "-s -w -H=windowsgui $ldVersion",
                '-o', "$stage\vpn_specify-setup.exe", './cmd/installer')
        } finally { Remove-Item $payload -Recurse -Force -ErrorAction SilentlyContinue }

        # 3. LEIA-ME (CRLF, para o Bloco de Notas) e licenças
        $readme = Join-Path $stage 'LEIA-ME.txt'
        Write-Template 'assets\dist\LEIA-ME.txt' $readme @{ VERSION = $version }
        [IO.File]::WriteAllText($readme, ([IO.File]::ReadAllText($readme, $utf8) -replace "`r?`n", "`r`n"), $utf8)
        [IO.File]::WriteAllText((Join-Path $root "$stage\LICENSE.txt"), $license, $utf8)
        Copy-Item windivert\LICENSE "$stage\licencas\WinDivert.txt"
        $mods = & go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}}|{{.Dir}}{{end}}{{end}}' ./cmd/vpn_specify ./cmd/installer
        if ($LASTEXITCODE) { throw 'go list falhou' }
        foreach ($m in ($mods | Where-Object { $_ } | Sort-Object -Unique)) {
            $path, $dir = $m -split '\|', 2
            $lic = Get-ChildItem $dir -File | Where-Object { $_.Name -match '^(LICEN[CS]E|COPYING|NOTICE)' }
            foreach ($l in $lic) {
                $suffix = if ($l.Name -match '^NOTICE') { '-NOTICE' } else { '' }
                Copy-Item $l.FullName ("$stage\licencas\" + ($path -replace '[/\\:]', '_') + "$suffix.txt")
            }
        }

        # 4. zip
        $zip = Join-Path 'dist' "$name.zip"
        Remove-Item $zip -ErrorAction SilentlyContinue
        Compress-Archive -Path $stage -DestinationPath $zip -CompressionLevel Optimal
        Write-Host ''
        Get-ChildItem $stage -Recurse -File | Select-Object @{ n = 'Arquivo'; e = { $_.FullName.Substring((Resolve-Path $stage).Path.Length + 1) } }, Length | Format-Table -AutoSize
        Write-Host "Pacote: $((Resolve-Path $zip).Path) ($([math]::Round((Get-Item $zip).Length / 1MB, 1)) MB)" -ForegroundColor Green
    }
}
finally {
    Pop-Location
}
