param(
    [string]$BaseUrl = "http://localhost:8080",
    [string]$KafkaContainer = "rotr-kafka-1"
)

$ErrorActionPreference = "Stop"

function Count-GameOver {
    param([string]$EventId)
    $oldPreference = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    $messages = & docker exec $KafkaContainer kafka-console-consumer `
        --bootstrap-server kafka-1:29092 `
        --topic game.broadcast `
        --from-beginning `
        --timeout-ms 5000 `
        --isolation-level read_committed 2>$null
    $ErrorActionPreference = $oldPreference
    return @($messages | Select-String $EventId).Count
}

function Post-Json {
    param([string]$Path, [hashtable]$Body)
    $json = $Body | ConvertTo-Json -Depth 20 -Compress
    Invoke-RestMethod -Method Post -Uri "$BaseUrl$Path" -ContentType "application/json" -Body $json | Out-Null
}

Write-Host "Transactional GameOver exactly-once smoke test..."

$eventId = "game-over-FREE_PEOPLES-10"
$before = Count-GameOver $eventId
Write-Host "before count: $before"

Post-Json "/game/start" @{ lightPlayerId = "light-player"; darkPlayerId = "dark-player" }
Start-Sleep -Seconds 3

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

Post-Json "/order" @{
    playerId = "light-player"
    playerSide = "FREE_PEOPLES"
    turn = 1
    unitId = "ring-bearer"
    orderType = "ASSIGN_ROUTE"
    pathIds = $route
}

for ($i = 1; $i -le 9; $i++) {
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
    Start-Sleep -Milliseconds 200
}

$state = Invoke-RestMethod -Uri "$BaseUrl/game/state?playerId=light-player&side=FREE_PEOPLES"
if ([int]$state.turn -ne 10) {
    throw "Expected turn 10 before DestroyRing, got $($state.turn)"
}

Post-Json "/order" @{
    playerId = "light-player"
    playerSide = "FREE_PEOPLES"
    turn = 10
    unitId = "ring-bearer"
    orderType = "DESTROY_RING"
}
Invoke-RestMethod -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
Start-Sleep -Seconds 3

$afterWin = Count-GameOver $eventId
Write-Host "after win count: $afterWin"
if ($afterWin -ne ($before + 1)) {
    throw "Expected exactly one new committed GameOver; before=$before after=$afterWin"
}

Invoke-RestMethod -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
Invoke-RestMethod -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
Start-Sleep -Seconds 2

$afterExtraAdvance = Count-GameOver $eventId
Write-Host "after extra advance count: $afterExtraAdvance"
if ($afterExtraAdvance -ne $afterWin) {
    throw "GameOver was produced again after game end; afterWin=$afterWin afterExtra=$afterExtraAdvance"
}

Write-Host "Transactional GameOver exactly-once smoke test passed."
