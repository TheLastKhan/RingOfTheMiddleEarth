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

Write-Host "Transactional GameOver exactly-once dark victory smoke test..."

$eventId = "game-over-SHADOW-6"
$before = Count-GameOver $eventId
Write-Host "before count: $before"

Post-Json "/game/start" @{ lightPlayerId = "light-player"; darkPlayerId = "dark-player" }
Start-Sleep -Seconds 3

# This route deliberately sends the Ring Bearer into Mordor, where Sauron starts.
# Frodo is repelled in combat and destroyed, producing a SHADOW GameOver.
$route = @(
    "shire-to-bree",
    "bree-to-rivendell",
    "rivendell-to-lothlorien",
    "lothlorien-to-emyn-muil",
    "emyn-muil-to-dead-marshes",
    "dead-marshes-to-mordor"
)

Post-Json "/order" @{
    playerId = "light-player"
    playerSide = "FREE_PEOPLES"
    turn = 1
    unitId = "ring-bearer"
    orderType = "ASSIGN_ROUTE"
    pathIds = $route
}

for ($i = 1; $i -le 6; $i++) {
    Invoke-RestMethod -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
    Start-Sleep -Milliseconds 250
}

Start-Sleep -Seconds 3

$state = Invoke-RestMethod -Uri "$BaseUrl/game/state?playerId=light-player&side=FREE_PEOPLES"
if ($state.gameOver -ne $true) {
    throw "Expected gameOver=true after Mordor interception."
}
if ($state.winner -ne "SHADOW") {
    throw "Expected SHADOW winner, got $($state.winner)"
}

$afterWin = Count-GameOver $eventId
Write-Host "after win count: $afterWin"
if ($afterWin -ne ($before + 1)) {
    throw "Expected exactly one new committed SHADOW GameOver; before=$before after=$afterWin"
}

Invoke-RestMethod -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
Invoke-RestMethod -Method Post -Uri "$BaseUrl/game/advance-turn" | Out-Null
Start-Sleep -Seconds 2

$afterExtraAdvance = Count-GameOver $eventId
Write-Host "after extra advance count: $afterExtraAdvance"
if ($afterExtraAdvance -ne $afterWin) {
    throw "SHADOW GameOver was produced again after game end; afterWin=$afterWin afterExtra=$afterExtraAdvance"
}

Write-Host "Transactional GameOver exactly-once dark victory smoke test passed."
