package rotr.streams;

import org.apache.kafka.common.serialization.Serdes;
import org.apache.kafka.streams.KafkaStreams;
import org.apache.kafka.streams.StreamsBuilder;
import org.apache.kafka.streams.StreamsConfig;
import org.apache.kafka.streams.Topology;
import org.apache.kafka.streams.kstream.*;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.Properties;
import java.util.concurrent.CountDownLatch;

/**
 * Ring of the Middle Earth — Kafka Streams Application
 *
 * Two topologies running in the same JVM:
 *   Topology 1: Order Validation (8 rules)  → K4 (10 points)
 *   Topology 2: Route Risk Enrichment       → K5 (4 points)
 *
 * Input:  game.orders.raw
 * Output: game.orders.validated (valid orders)
 *         game.dlq (invalid orders)
 */
public class StreamsApp {

    private static final Logger log = LoggerFactory.getLogger(StreamsApp.class);

    // ── Topic names ──
    static final String ORDERS_RAW       = "game.orders.raw";
    static final String ORDERS_VALIDATED  = "game.orders.validated";
    static final String DLQ              = "game.dlq";
    static final String EVENTS_UNIT      = "game.events.unit";
    static final String EVENTS_REGION    = "game.events.region";
    static final String EVENTS_PATH      = "game.events.path";
    static final String BROADCAST        = "game.broadcast";

    public static void main(String[] args) {
        log.info("═══════════════════════════════════════");
        log.info("    RING OF THE MIDDLE EARTH");
        log.info("    Kafka Streams — Topology 1 + 2");
        log.info("═══════════════════════════════════════");

        Properties props = new Properties();
        props.put(StreamsConfig.APPLICATION_ID_CONFIG, "rotr-streams");
        props.put(StreamsConfig.BOOTSTRAP_SERVERS_CONFIG,
                System.getenv().getOrDefault("KAFKA_BROKERS", "localhost:9092"));
        props.put(StreamsConfig.DEFAULT_KEY_SERDE_CLASS_CONFIG, Serdes.StringSerde.class);
        props.put(StreamsConfig.DEFAULT_VALUE_SERDE_CLASS_CONFIG, Serdes.StringSerde.class);
        // Exactly-once semantics
        props.put(StreamsConfig.PROCESSING_GUARANTEE_CONFIG, StreamsConfig.EXACTLY_ONCE_V2);

        Topology topology = buildTopology();
        log.info("Topology description:\n{}", topology.describe());

        KafkaStreams streams = new KafkaStreams(topology, props);

        // Graceful shutdown
        CountDownLatch latch = new CountDownLatch(1);
        Runtime.getRuntime().addShutdownHook(new Thread(() -> {
            log.info("🛑 Shutting down Kafka Streams...");
            streams.close();
            latch.countDown();
        }));

        try {
            streams.start();
            log.info("🚀 Kafka Streams started successfully");
            latch.await();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
        }
    }

    /**
     * Builds both topologies in one StreamsBuilder.
     */
    static Topology buildTopology() {
        StreamsBuilder builder = new StreamsBuilder();

        // ════════════════════════════════════════════
        // TOPOLOGY 1: Order Validation (8 rules)
        // Input:  game.orders.raw
        // Output: game.orders.validated (valid)
        //         game.dlq (invalid)
        // ════════════════════════════════════════════

        KStream<String, String> rawOrders = builder.stream(ORDERS_RAW);

        // Split: valid orders vs invalid orders (modern API, replaces deprecated branch())
        var splitOrders = rawOrders.split(Named.as("order-"))
            .branch(
                (key, value) -> OrderValidator.validate(value).isValid(),
                Branched.as("valid")
            )
            .defaultBranch(Branched.as("invalid"));

        KStream<String, String> validOrders   = splitOrders.get("order-valid");
        KStream<String, String> invalidOrders = splitOrders.get("order-invalid");

        // Valid → game.orders.validated
        validOrders.to(ORDERS_VALIDATED);

        // Invalid → game.dlq with error enrichment
        invalidOrders
            .mapValues(value -> OrderValidator.enrichDLQ(value))
            .to(DLQ);

        // ════════════════════════════════════════════
        // TOPOLOGY 2: Route Risk Enrichment
        // Reads validated orders + world state
        // Enriches with routeRiskScore (Schema V2)
        // ════════════════════════════════════════════

        // Build KTable from broadcast topic (world state)
        KTable<String, String> worldState = builder.table(BROADCAST);

        // Read validated orders
        KStream<String, String> validated = builder.stream(ORDERS_VALIDATED);

        // Left-join with world state to enrich
        KStream<String, String> enriched = validated.leftJoin(
            worldState,
            (orderJson, stateJson) -> RouteRiskEnricher.enrich(orderJson, stateJson)
        );

        // Write enriched orders back (V2 schema with routeRiskScore)
        enriched.to(ORDERS_VALIDATED, Produced.with(Serdes.String(), Serdes.String()));

        return builder.build();
    }
}
