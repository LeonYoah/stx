# STX 重启脚本：可选构建 + 可选前端，前端 PM2 名为 stx-ui
# - 默认前端使用生产模式启动
# - 可选通过 -FrontendDev 改为 pnpm run dev，避免前端开发时重复构建
# 用法：
#   .\scripts\restart.ps1                    # 默认：构建前后端并启动
#   .\scripts\restart.ps1 -NoBuild           # 仅重启，不构建
#   .\scripts\restart.ps1 -FrontendDev       # 前端改用 pnpm run dev 启动（跳过前端 build）
#   .\scripts\restart.ps1 -NoFrontend        # 仅后端（可配合 -NoBuild 仅重启后端）
#   .\scripts\restart.ps1 -StopFrontend     # 仅停止前端 (pm2 stop stx-ui)

param(
    [switch]$NoBuild,      # 不构建，仅重启已有进程/二进制
    [switch]$FrontendDev,  # 前端使用 pnpm run dev 启动
    [switch]$NoFrontend,   # 不构建、不启动前端
    [switch]$StopFrontend  # 只停止前端并退出
)

$ErrorActionPreference = "Stop"
$ProjectRoot = Split-Path -Parent $PSScriptRoot
if (-not (Test-Path (Join-Path $ProjectRoot "go.mod"))) {
    Write-Error "未在项目根找到 go.mod，请于项目根目录执行: .\scripts\restart.ps1"
    exit 1
}

Set-Location $ProjectRoot
$ConfigPath = if ($env:CONFIG_PATH) { $env:CONFIG_PATH } else { Join-Path $ProjectRoot "config.yaml" }
$AppExternalUrl = if ($env:APP_EXTERNAL_URL) { $env:APP_EXTERNAL_URL } else { "http://127.0.0.1:8000" }
$PM2_UI = "stx-ui"
$FrontendPort = if ($env:FRONTEND_PORT) { $env:FRONTEND_PORT } else { "80" }
$BackendBaseUrl = if ($env:NEXT_PUBLIC_BACKEND_BASE_URL) { $env:NEXT_PUBLIC_BACKEND_BASE_URL } else { "http://127.0.0.1:8000" }
$FrontendDir = Join-Path $ProjectRoot "frontend"
$FrontendStandaloneDir = Join-Path $FrontendDir "dist-standalone"
$script:FrontendEntry = $null
$script:FrontendRuntimeDir = $FrontendStandaloneDir

function Sync-AppExternalUrl {
    param(
        [string]$Path,
        [string]$ExternalUrl
    )

    if (-not (Test-Path $Path)) {
        Write-Host "未找到配置文件 $Path，跳过 external_url 同步." -ForegroundColor Yellow
        return
    }

    $lines = Get-Content $Path
    $appStart = -1
    $appIndent = ""

    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match '^(\s*)app:\s*$') {
            $appStart = $i
            $appIndent = $matches[1]
            break
        }
    }

    if ($appStart -lt 0) {
        return
    }

    $insertAt = $appStart + 1
    $found = $false
    for ($i = $appStart + 1; $i -lt $lines.Count; $i++) {
        $line = $lines[$i]
        $trimmed = $line.Trim()
        if ($trimmed -and -not $trimmed.StartsWith('#')) {
            $indent = ([regex]::Match($line, '^\s*')).Value
            if ($indent.Length -le $appIndent.Length) {
                break
            }
        }

        if ($line -match '^\s*external_url:\s*') {
            $lines[$i] = "$appIndent  external_url: `"$ExternalUrl`""
            $found = $true
            break
        }
        $insertAt = $i + 1
    }

    if (-not $found) {
        $updated = New-Object System.Collections.Generic.List[string]
        for ($i = 0; $i -lt $lines.Count; $i++) {
            $updated.Add($lines[$i])
            if ($i -eq ($insertAt - 1)) {
                $updated.Add("$appIndent  external_url: `"$ExternalUrl`"")
            }
        }
        if ($lines.Count -eq 0) {
            $updated.Add("$appIndent  external_url: `"$ExternalUrl`"")
        }
        $lines = $updated.ToArray()
    }

    Set-Content -Path $Path -Value $lines -Encoding UTF8
    Write-Host "      已同步 app.external_url = $ExternalUrl" -ForegroundColor Gray
}

function Resolve-StandaloneServerEntry {
    param(
        [string]$StandaloneRoot
    )

    $rootEntry = Join-Path $StandaloneRoot "server.js"
    if (Test-Path $rootEntry) {
        return $rootEntry
    }

    $nestedEntry = Get-ChildItem -Path $StandaloneRoot -Filter "server.js" -File -Recurse -ErrorAction SilentlyContinue |
        Sort-Object FullName |
        Select-Object -First 1

    if ($null -ne $nestedEntry) {
        return $nestedEntry.FullName
    }

    return $null
}

function Prepare-FrontendStandalone {
    param(
        [switch]$SkipBuild
    )

    if (-not (Test-Path (Join-Path $FrontendDir "package.json"))) {
        Write-Host "      未找到 frontend/package.json，跳过前端." -ForegroundColor Yellow
        return $false
    }

    Set-Location $FrontendDir

    if (-not $SkipBuild) {
        Write-Host "      构建前端（next build）..." -ForegroundColor Gray
        pnpm run build
        if ($LASTEXITCODE -ne 0) {
            Set-Location $ProjectRoot
            return $false
        }
    }

    $nextStandaloneDir = Join-Path $FrontendDir ".next\standalone"
    $nextStandaloneEntry = Resolve-StandaloneServerEntry -StandaloneRoot $nextStandaloneDir
    if (-not $nextStandaloneEntry -or -not (Test-Path $nextStandaloneEntry)) {
        Write-Host "      未找到 .next/standalone 下的 server.js，请确认 next.config.ts 已配置 output: 'standalone'" -ForegroundColor Yellow
        Set-Location $ProjectRoot
        return $false
    }

    $entryRelativePath = [System.IO.Path]::GetRelativePath($nextStandaloneDir, $nextStandaloneEntry)
    $runtimeRelativeDir = Split-Path -Path $entryRelativePath -Parent

    Write-Host "      组装 standalone 运行目录 (entry: $entryRelativePath) ..." -ForegroundColor Gray
    if (Test-Path $FrontendStandaloneDir) {
        Remove-Item $FrontendStandaloneDir -Recurse -Force
    }
    New-Item -ItemType Directory -Force -Path $FrontendStandaloneDir | Out-Null
    Copy-Item (Join-Path $nextStandaloneDir "*") $FrontendStandaloneDir -Recurse -Force

    $script:FrontendEntry = Join-Path $FrontendStandaloneDir $entryRelativePath
    if ([string]::IsNullOrWhiteSpace($runtimeRelativeDir) -or $runtimeRelativeDir -eq ".") {
        $script:FrontendRuntimeDir = $FrontendStandaloneDir
    } else {
        $script:FrontendRuntimeDir = Join-Path $FrontendStandaloneDir $runtimeRelativeDir
    }

    $staticDir = Join-Path $FrontendDir ".next\static"
    if (Test-Path $staticDir) {
        $targetNextDir = Join-Path $script:FrontendRuntimeDir ".next"
        New-Item -ItemType Directory -Force -Path $targetNextDir | Out-Null
        Copy-Item $staticDir (Join-Path $targetNextDir "static") -Recurse -Force
    }

    $publicDir = Join-Path $FrontendDir "public"
    if (Test-Path $publicDir) {
        Copy-Item $publicDir (Join-Path $script:FrontendRuntimeDir "public") -Recurse -Force
    }

    Set-Location $ProjectRoot

    if (-not (Test-Path $script:FrontendEntry)) {
        Write-Host "      standalone 产物不完整: $script:FrontendEntry 不存在" -ForegroundColor Yellow
        return $false
    }

    return $true
}

Sync-AppExternalUrl -Path $ConfigPath -ExternalUrl $AppExternalUrl

if ($StopFrontend) {
    Write-Host "停止前端 (pm2 stop $PM2_UI) ..." -ForegroundColor Cyan
    pm2 stop $PM2_UI 2>$null; pm2 status
    Write-Host "完成." -ForegroundColor Green
    exit 0
}

$step = 0
$total = 1
if (-not $NoBuild) { $total += 2 }
if (-not $NoFrontend) { $total += 1 }

if (-not $NoBuild) {
    $step++; Write-Host "[$step/$total] 构建 stx ..." -ForegroundColor Cyan
    go build -o stx .
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
    Write-Host "      stx 构建完成." -ForegroundColor Green

    $step++; Write-Host "[$step/$total] 构建 stx-agent ..." -ForegroundColor Cyan
    Set-Location (Join-Path $ProjectRoot "agent")
    go build -o stx-agent ./cmd
    if ($LASTEXITCODE -ne 0) { Set-Location $ProjectRoot; exit $LASTEXITCODE }
    Set-Location $ProjectRoot
    Write-Host "      stx-agent 构建完成." -ForegroundColor Green

    $LibAgent = Join-Path $ProjectRoot "lib\agent"
    if (Test-Path $LibAgent) {
        $agentBin = Join-Path $ProjectRoot "agent\stx-agent"
        if ($env:OS -eq "Windows_NT") { $agentBin = $agentBin + ".exe" }
        if (Test-Path $agentBin) {
            $dest = Join-Path $LibAgent "stx-agent-windows-amd64.exe"
            if ($env:OS -ne "Windows_NT") { $dest = Join-Path $LibAgent "stx-agent-linux-amd64" }
            Copy-Item $agentBin $dest -Force -ErrorAction SilentlyContinue
            Write-Host "      已同步 agent 到 lib/agent." -ForegroundColor Gray
        }
    }
}

$step++; Write-Host "[$step/$total] 停止已有 stx 进程 ..." -ForegroundColor Cyan
Get-Process -Name "stx" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
Start-Sleep -Seconds 1
Write-Host "      已停止." -ForegroundColor Green

$step++; Write-Host "[$step/$total] 启动 stx api ..." -ForegroundColor Cyan
$binName = "stx"
if ($env:OS -eq "Windows_NT") { $binName = "stx.exe" }
$bin = Join-Path $ProjectRoot $binName
if (-not (Test-Path $bin)) {
    Write-Error "未找到 $binName，请先执行一次无 -NoBuild 的 restart 或手动构建"
    exit 1
}
Start-Process -FilePath $bin -ArgumentList "api" -WorkingDirectory $ProjectRoot -NoNewWindow
Write-Host "      已启动 (API 默认 http://localhost:8000，日志见 config.yaml 中 log.file_path)." -ForegroundColor Green

if (-not $NoFrontend) {
    $step++; Write-Host "[$step/$total] 前端 ..." -ForegroundColor Cyan
    if (-not (Test-Path (Join-Path $FrontendDir "package.json"))) {
        Write-Host "      未找到 frontend/package.json，跳过前端." -ForegroundColor Yellow
    } else {
        Set-Location $FrontendDir
        if ($FrontendDev) {
            $env:HOSTNAME = "0.0.0.0"
            $env:PORT = "$FrontendPort"
            $env:NEXT_PUBLIC_BACKEND_BASE_URL = $BackendBaseUrl
            pm2 delete $PM2_UI 2>$null
            pm2 start "pnpm exec next dev --turbopack --hostname 0.0.0.0 --port $FrontendPort" --name $PM2_UI --update-env
            if ($LASTEXITCODE -ne 0) { Set-Location $ProjectRoot; exit $LASTEXITCODE }
            pm2 status
            Set-Location $ProjectRoot
            Write-Host "      前端开发模式已启动 (PM2: $PM2_UI，端口 $FrontendPort)." -ForegroundColor Green
        } else {
            Set-Location $ProjectRoot
            if (Prepare-FrontendStandalone -SkipBuild:$NoBuild) {
                $env:HOSTNAME = "0.0.0.0"
                $env:PORT = "$FrontendPort"
                $env:NEXT_PUBLIC_BACKEND_BASE_URL = $BackendBaseUrl
                pm2 delete $PM2_UI 2>$null
                pm2 start $script:FrontendEntry --name $PM2_UI --cwd $script:FrontendRuntimeDir --update-env
                if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
                pm2 status
                Write-Host "      前端 standalone 已启动 (PM2: $PM2_UI，端口 $FrontendPort)." -ForegroundColor Green
            } else {
                Write-Host "      前端启动已跳过." -ForegroundColor Yellow
            }
        }
    }
} else {
    Write-Host "      跳过前端 (NoFrontend)." -ForegroundColor Gray
}

Write-Host "完成." -ForegroundColor Green
