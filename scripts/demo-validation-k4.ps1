param(
    [switch]$UseLocalMaven
)

if ($UseLocalMaven) {
    Push-Location "kafka/streams"
    try {
        mvn test
    } finally {
        Pop-Location
    }
    exit $LASTEXITCODE
}

Write-Host "Running K4 Kafka Streams validation tests in Maven Docker image..."
docker run --rm `
    -v "${PWD}/kafka/streams:/app" `
    -w /app `
    maven:3.9-eclipse-temurin-17 `
    mvn test -B

exit $LASTEXITCODE
