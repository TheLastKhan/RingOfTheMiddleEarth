#!/bin/bash
# ═══════════════════════════════════════════════════════
# Ring of the Middle Earth — Kafka Topic Creation Script
# Creates all 10 required topics with correct config
# ═══════════════════════════════════════════════════════

set -e

KAFKA_BROKER="${KAFKA_BROKER:-kafka-1:9092}"

echo "⏳ Waiting for Kafka to be ready..."
cub kafka-ready -b "$KAFKA_BROKER" 1 120

echo "📋 Creating 10 topics..."

# ── game.orders.raw ──
# Raw orders from browser. Key: playerId. Same player's orders stay ordered.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.orders.raw \
  --partitions 3 \
  --replication-factor 3 \
  --config retention.ms=3600000 \
  --config cleanup.policy=delete

# ── game.orders.validated ──
# Validated orders. Key: unitId. Same unit's orders processed sequentially.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.orders.validated \
  --partitions 6 \
  --replication-factor 3 \
  --config retention.ms=3600000 \
  --config cleanup.policy=delete

# ── game.events.unit ──
# Unit movement, damage, respawn events. Key: unitId.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.events.unit \
  --partitions 6 \
  --replication-factor 3 \
  --config retention.ms=604800000 \
  --config cleanup.policy=delete

# ── game.events.region ──
# Region control, battle events. Key: regionId.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.events.region \
  --partitions 6 \
  --replication-factor 3 \
  --config retention.ms=604800000 \
  --config cleanup.policy=delete

# ── game.events.path ──
# Path status, surveillance events. Key: pathId.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.events.path \
  --partitions 6 \
  --replication-factor 3 \
  --config retention.ms=604800000 \
  --config cleanup.policy=delete

# ── game.session ──
# Log-compacted: always holds latest game state. Single partition.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.session \
  --partitions 1 \
  --replication-factor 3 \
  --config cleanup.policy=compact

# ── game.broadcast ──
# WorldStateSnapshot for both sides. Single partition.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.broadcast \
  --partitions 1 \
  --replication-factor 3 \
  --config retention.ms=3600000 \
  --config cleanup.policy=delete

# ── game.ring.position ──
# RingBearerMoved events. Light Side SSE only. Single partition.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.ring.position \
  --partitions 1 \
  --replication-factor 3 \
  --config retention.ms=3600000 \
  --config cleanup.policy=delete

# ── game.ring.detection ──
# RingBearerDetected/Spotted. Dark Side SSE only. Key: playerId.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.ring.detection \
  --partitions 2 \
  --replication-factor 3 \
  --config retention.ms=3600000 \
  --config cleanup.policy=delete

# ── game.dlq ──
# Dead Letter Queue for invalid orders. Key: errorCode.
kafka-topics --bootstrap-server "$KAFKA_BROKER" --create --if-not-exists \
  --topic game.dlq \
  --partitions 3 \
  --replication-factor 3 \
  --config retention.ms=604800000 \
  --config cleanup.policy=delete

echo "✅ All 10 topics created successfully!"

# ── Register Avro schemas ──
echo "📝 Registering Avro schemas..."
SCHEMA_REGISTRY="${SCHEMA_REGISTRY:-http://schema-registry:8081}"

register_schema() {
  local subject="$1"
  local schema_file="$2"
  echo "  Registering: $subject"
  local schema_json
  schema_json=$(sed ':a;N;$!ba;s/\\/\\\\/g;s/"/\\"/g;s/\n//g' "$schema_file")
  curl -s -X POST -H "Content-Type: application/vnd.schemaregistry.v1+json" \
    -d "{\"schema\": \"$schema_json\"}" \
    "$SCHEMA_REGISTRY/subjects/$subject/versions" > /dev/null 2>&1 || true
}

SCHEMA_DIR="/schemas"

register_schema "game.orders.raw-value"       "$SCHEMA_DIR/OrderSubmitted.avsc"
register_schema "game.orders.validated-value"  "$SCHEMA_DIR/OrderValidated.avsc"
register_schema "game.events.unit-value"       "$SCHEMA_DIR/UnitMoved.avsc"
register_schema "game.events.path-value"       "$SCHEMA_DIR/PathStatusChanged.avsc"
register_schema "game.events.region-value"     "$SCHEMA_DIR/RegionControlChanged.avsc"
register_schema "game.ring.position-value"     "$SCHEMA_DIR/RingBearerMoved.avsc"
register_schema "game.ring.detection-value"    "$SCHEMA_DIR/RingBearerDetected.avsc"
register_schema "game.broadcast-value"         "$SCHEMA_DIR/WorldStateSnapshot.avsc"
register_schema "game.dlq-value"               "$SCHEMA_DIR/DLQEntry.avsc"

echo "✅ All schemas registered!"
echo "🚀 Kafka initialization complete."
