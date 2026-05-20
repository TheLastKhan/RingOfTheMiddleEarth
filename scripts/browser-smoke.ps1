param(
    [string]$UiUrl = "http://localhost:3000",
    [string]$OutputDir = ".tmp",
    [switch]$RequireScreenshot
)

$ErrorActionPreference = "Stop"

function Assert-Contains($Text, $Needle, $Message) {
    if ($Text -notmatch [regex]::Escape($Needle)) {
        throw $Message
    }
}

Write-Host "Browser/UI smoke starting for $UiUrl"

$response = Invoke-WebRequest -UseBasicParsing -TimeoutSec 10 -Uri $UiUrl
$html = $response.Content

Assert-Contains $html 'id="login-screen"' "UI login screen markup was not found."
Assert-Contains $html 'id="map-canvas"' "Map canvas markup was not found."
Assert-Contains $html 'id="conn-status"' "Connection status markup was not found."
Assert-Contains $html 'id="advance-turn-btn"' "Manual advance-turn button was not found."
Assert-Contains $html 'new EventSource' "SSE client wiring was not found."

Write-Host "PASS static UI contract"

if (-not $RequireScreenshot) {
    Write-Host "SKIP Playwright screenshot: pass -RequireScreenshot to run this optional browser capture."
    Write-Host "Browser/UI smoke test passed."
    exit 0
}

$npx = Get-Command npx.cmd -ErrorAction SilentlyContinue
if ($null -eq $npx) {
    throw "npx.cmd was not found, so Playwright screenshot smoke cannot run."
}

New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
$lightShot = Join-Path $OutputDir "ui-light.png"
$darkShot = Join-Path $OutputDir "ui-dark.png"

try {
    & npx.cmd playwright screenshot --wait-for-timeout=3000 "${UiUrl}?side=light" $lightShot
    if ($LASTEXITCODE -ne 0) {
        throw "Playwright light screenshot failed with exit code $LASTEXITCODE. Run: npx playwright install chromium"
    }

    & npx.cmd playwright screenshot --wait-for-timeout=3000 "${UiUrl}?side=dark" $darkShot
    if ($LASTEXITCODE -ne 0) {
        throw "Playwright dark screenshot failed with exit code $LASTEXITCODE. Run: npx playwright install chromium"
    }

    $lightInfo = Get-Item $lightShot
    $darkInfo = Get-Item $darkShot
    if ($lightInfo.Length -le 0 -or $darkInfo.Length -le 0) {
        throw "Playwright produced an empty screenshot."
    }

    Write-Host "PASS Playwright screenshots: $lightShot, $darkShot"
} catch {
    throw
}

Write-Host "Browser/UI smoke test passed."
