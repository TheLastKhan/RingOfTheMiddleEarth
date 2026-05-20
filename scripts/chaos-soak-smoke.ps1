param(
    [string]$BaseUrl = "http://localhost",
    [int[]]$EnginePorts = @(8080, 8082, 8083),
    [string[]]$EngineServices = @("go-engine-1", "go-engine-2", "go-engine-3"),
    [int]$Rounds = 6,
    [int]$StopSeconds = 2
)

$ErrorActionPreference = "Stop"

function Assert-True($Condition, $Message) {
    if (-not $Condition) {
        throw $Message
    }
}

function Post-Json($Url, $Body) {
    $json = $Body | ConvertTo-Json -Depth 20 -Compress
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

function Get-PprofFirstLine {
    try {
        $profile = Invoke-RestMethod -TimeoutSec 5 -Uri "$BaseUrl/debug/pprof/goroutine?debug=1"
        return ($profile -split "`n")[0]
    } catch {
        return "pprof unavailable: $($_.Exception.Message)"
    }
}

function Wait-Converged($ExpectedTurn, $ExpectedRegion, $Ports, $Label) {
    $deadline = (Get-Date).AddSeconds(25)
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

function Advance-Turn {
    Invoke-RestMethod -TimeoutSec 10 -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
}

function Invoke-Compose($ComposeArgs) {
    & docker compose @ComposeArgs
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose $($ComposeArgs -join ' ') failed with exit code $LASTEXITCODE"
    }
}

Write-Host "Chaos/soak smoke starting: rounds=$Rounds stopSeconds=$StopSeconds"

Invoke-RestMethod -TimeoutSec 5 -Uri "$BaseUrl/health" | Out-Null
Assert-True ((Invoke-RestMethod -TimeoutSec 5 -Uri "$BaseUrl/analysis/routes") -ne $null) "/analysis/routes returned null"
Assert-True ((Invoke-RestMethod -TimeoutSec 5 -Uri "$BaseUrl/analysis/intercept") -ne $null) "/analysis/intercept returned null"

$startPprof = Get-PprofFirstLine
Write-Host "pprof start: $startPprof"

$route = @(
    "shire-to-bree",
    "bree-to-weathertop",
    "weathertop-to-rivendell",
    "rivendell-to-moria",
    "moria-to-lothlorien",
    "lothlorien-to-emyn-muil",
    "emyn-muil-to-ithilien",
    "ithilien-to-cirith-ungol",
    "cirith-ungol-to-mount-doom"
)
$regions = @(
    "the-shire",
    "bree",
    "weathertop",
    "rivendell",
    "moria",
    "lothlorien",
    "emyn-muil",
    "ithilien",
    "cirith-ungol",
    "mount-doom"
)

Post-Json "$BaseUrl/game/start" @{ lightPlayerId = "light-player"; darkPlayerId = "dark-player" } | Out-Null
Wait-Converged 1 "the-shire" $EnginePorts "fresh session replay"

$orderResponse = Post-Json "$BaseUrl/order" @{
    playerId = "light-player"
    playerSide = "FREE_PEOPLES"
    turn = 1
    unitId = "ring-bearer"
    orderType = "ASSIGN_ROUTE"
    pathIds = $route
}
Assert-True ($orderResponse.Code -eq 202) "Expected ASSIGN_ROUTE to be accepted, got HTTP $($orderResponse.Code): $($orderResponse.Body)"

try {
    for ($round = 1; $round -le $Rounds; $round++) {
        $serviceIndex = ($round - 1) % $EngineServices.Count
        $service = $EngineServices[$serviceIndex]
        $stoppedPort = $EnginePorts[$serviceIndex]
        $remainingPorts = $EnginePorts | Where-Object { $_ -ne $stoppedPort }

        Write-Host "Round ${round}: stopping $service, advancing turn through nginx..."
        Invoke-Compose @("stop", $service) | Out-Null
        Start-Sleep -Seconds $StopSeconds
        Invoke-RestMethod -TimeoutSec 5 -Uri "$BaseUrl/health" | Out-Null

        Advance-Turn

        $expectedTurn = $round + 1
        $regionIndex = [Math]::Min($round, $regions.Count - 1)
        $expectedRegion = $regions[$regionIndex]
        Wait-Converged $expectedTurn $expectedRegion $remainingPorts "round $round surviving engines"

        Write-Host "Round ${round}: restarting $service..."
        Invoke-Compose @("start", $service) | Out-Null
        Wait-Converged $expectedTurn $expectedRegion $EnginePorts "round $round restarted engine catch-up"
    }
} finally {
    foreach ($service in $EngineServices) {
        Invoke-Compose @("start", $service) | Out-Null
    }
}

Invoke-RestMethod -TimeoutSec 5 -Uri "$BaseUrl/health" | Out-Null
$endPprof = Get-PprofFirstLine
Write-Host "pprof end:   $endPprof"
Write-Host "Chaos/soak smoke test passed."
