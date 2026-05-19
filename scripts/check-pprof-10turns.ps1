param(
    [string]$BaseUrl = "http://localhost",
    [int]$Seconds = 610
)

$start = Invoke-RestMethod "$BaseUrl/debug/pprof/goroutine?debug=1"
$startFirstLine = ($start -split "`n")[0]
Write-Host "start: $startFirstLine"

Write-Host "waiting $Seconds seconds for approximately 10 turns..."
Start-Sleep -Seconds $Seconds

$end = Invoke-RestMethod "$BaseUrl/debug/pprof/goroutine?debug=1"
$endFirstLine = ($end -split "`n")[0]
Write-Host "end:   $endFirstLine"

Write-Host "Inspect the two totals above. They should remain stable for the demo."
