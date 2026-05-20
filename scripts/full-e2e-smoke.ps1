param(
    [string]$BaseUrl = "http://localhost",
    [int[]]$EnginePorts = @(8080, 8082, 8083),
    [string]$StoppedEngine = "go-engine-1",
    [int]$SsePort = 8080
)

$ErrorActionPreference = "Stop"

function Assert-True($Condition, $Message) {
    if (-not $Condition) {
        throw $Message
    }
}

function Post-Json($Url, $Body) {
    $json = $Body | ConvertTo-Json -Depth 10 -Compress
    try {
        $response = Invoke-WebRequest -UseBasicParsing -Method Post -Uri $Url -ContentType "application/json" -Body $json
        return [pscustomobject]@{ Code = [int]$response.StatusCode; Body = $response.Content }
    } catch {
        $response = $_.Exception.Response
        if ($null -eq $response) {
            throw
        }
        $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
        return [pscustomobject]@{ Code = [int]$response.StatusCode; Body = $reader.ReadToEnd() }
    }
}

function Get-State($Port) {
    Invoke-RestMethod -TimeoutSec 5 -Uri "http://localhost:$Port/game/state?playerId=light-player&side=FREE_PEOPLES"
}

function Get-RingRegion($State) {
    ($State.units | Where-Object { $_.id -eq "ring-bearer" } | Select-Object -First 1).currentRegion
}

function Wait-Converged($ExpectedTurn, $ExpectedRegion, $Ports, $Label) {
    $deadline = (Get-Date).AddSeconds(20)
    do {
        $rows = @()
        $ok = $true
        foreach ($port in $Ports) {
            try {
                $state = Get-State $port
                $ring = Get-RingRegion $state
                $rows += "port=$port turn=$($state.turn) ring=$ring"
                if ([int]$state.turn -ne $ExpectedTurn -or $ring -ne $ExpectedRegion) {
                    $ok = $false
                }
            } catch {
                $rows += "port=$port ERROR $($_.Exception.Message)"
                $ok = $false
            }
        }
        if ($ok) {
            Write-Host "PASS $Label"
            $rows | ForEach-Object { Write-Host "  $_" }
            return
        }
        Start-Sleep -Seconds 1
    } while ((Get-Date) -lt $deadline)

    $rows | ForEach-Object { Write-Host "  $_" }
    throw "Engines did not converge for $Label"
}

function Advance-Turn($Url = $BaseUrl) {
    Invoke-RestMethod -Method Post -Uri "$Url/game/advance-turn" | Out-Null
}

Write-Host "Full E2E smoke test starting..."

Invoke-RestMethod -TimeoutSec 5 -Uri "$BaseUrl/health" | Out-Null

Post-Json "$BaseUrl/game/start" @{ lightPlayerId = "light-player"; darkPlayerId = "dark-player" } | Out-Null
Wait-Converged 1 "the-shire" $EnginePorts "new game session replay"

$routeOrder = @{
    playerId   = "light-player"
    playerSide = "FREE_PEOPLES"
    turn       = 1
    unitId     = "ring-bearer"
    orderType  = "ASSIGN_ROUTE"
    pathIds    = @("shire-to-bree", "bree-to-weathertop", "weathertop-to-rivendell")
}

$jobs = 1..8 | ForEach-Object {
    Start-Job -ScriptBlock {
        param($Url, $Body)
        $json = $Body | ConvertTo-Json -Depth 10 -Compress
        try {
            $response = Invoke-WebRequest -UseBasicParsing -Method Post -Uri $Url -ContentType "application/json" -Body $json
            [pscustomobject]@{ Code = [int]$response.StatusCode; Body = $response.Content }
        } catch {
            $response = $_.Exception.Response
            $reader = New-Object System.IO.StreamReader($response.GetResponseStream())
            [pscustomobject]@{ Code = [int]$response.StatusCode; Body = $reader.ReadToEnd() }
        }
    } -ArgumentList "$BaseUrl/order", $routeOrder
}

$responses = $jobs | Wait-Job | Receive-Job
$jobs | Remove-Job
$accepted = @($responses | Where-Object { $_.Code -eq 202 }).Count
$rejected = @($responses | Where-Object { $_.Code -eq 400 }).Count
Write-Host "Concurrent duplicate orders: accepted=$accepted rejected=$rejected"
Assert-True ($accepted -eq 1) "Expected exactly one accepted duplicate order batch request."
Assert-True ($rejected -eq 7) "Expected seven duplicate order rejections."

Advance-Turn
Wait-Converged 2 "bree" $EnginePorts "turn 2 movement replay"

docker compose stop $StoppedEngine | Out-Null
try {
    Advance-Turn
    $remainingPorts = $EnginePorts | Where-Object { $_ -ne 8080 }
    Wait-Converged 3 "weathertop" $remainingPorts "failover after stopping $StoppedEngine"
} finally {
    docker compose start $StoppedEngine | Out-Null
}

Wait-Converged 3 "weathertop" $EnginePorts "restarted engine catches latest session"

$sseJob = Start-Job -ScriptBlock {
    param($Port)
    curl.exe -s -N --max-time 8 "http://localhost:$Port/events?playerId=light-player&side=FREE_PEOPLES" 2>$null
} -ArgumentList $SsePort
Start-Sleep -Seconds 1
Advance-Turn "http://localhost:$SsePort"
Wait-Job $sseJob | Out-Null
$sseOutput = Receive-Job $sseJob
Remove-Job $sseJob
$sseText = ($sseOutput -join "`n")
Assert-True ($sseText -match "event: game\.") "SSE stream did not include a game event."
Write-Host "PASS SSE event stream"

Wait-Converged 4 "rivendell" $EnginePorts "post-SSE turn replay"

Write-Host "Full E2E smoke test passed."
