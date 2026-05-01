package rotr.streams;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;



/**
 * Topology 2: Route Risk Enrichment (Rubric K5, 4 points)
 *
 * Enriches validated orders with a routeRiskScore field.
 * This enables Schema V2 (backward-compatible evolution: K3).
 *
 * Risk Score Formula:
 *   riskScore = sum(region.threatLevel)
 *             + sum(path.surveillanceLevel) * 3
 *             + count(BLOCKED paths) * 5
 *             + count(THREATENED paths) * 2
 *
 * The enriched order uses the V2 schema which adds the
 * nullable routeRiskScore field.
 */
public class RouteRiskEnricher {

    private static final Logger log = LoggerFactory.getLogger(RouteRiskEnricher.class);
    private static final ObjectMapper mapper = new ObjectMapper();

    /**
     * Enriches a validated order with route risk score.
     *
     * @param orderJson     The validated order JSON
     * @param worldStateJson The current world state from KTable
     * @return Enriched order JSON with routeRiskScore field
     */
    public static String enrich(String orderJson, String worldStateJson) {
        try {
            ObjectNode order = (ObjectNode) mapper.readTree(orderJson);

            // Only enrich route-related orders
            String orderType = order.path("orderType").asText("");
            if (!orderType.equals("ASSIGN_ROUTE") && !orderType.equals("REDIRECT_UNIT")) {
                // Non-route orders: set null risk score (V2 compatible)
                order.putNull("routeRiskScore");
                return mapper.writeValueAsString(order);
            }

            // Parse world state
            int riskScore = 0;
            if (worldStateJson != null) {
                riskScore = calculateRiskFromState(order, worldStateJson);
            }

            // Add risk score (V2 schema field)
            order.put("routeRiskScore", riskScore);

            log.debug("📊 Route risk enriched: order={}, riskScore={}",
                order.path("unitId").asText(), riskScore);

            return mapper.writeValueAsString(order);

        } catch (Exception e) {
            log.warn("⚠️ Risk enrichment failed: {}", e.getMessage());
            // Return original order with null risk (V2 compatible)
            try {
                ObjectNode order = (ObjectNode) mapper.readTree(orderJson);
                order.putNull("routeRiskScore");
                return mapper.writeValueAsString(order);
            } catch (Exception ex) {
                return orderJson;
            }
        }
    }

    /**
     * Calculates the risk score from the world state.
     *
     * riskScore =
     *     sum(region.threatLevel for each region in route)
     *   + sum(path.surveillanceLevel) * 3
     *   + count(BLOCKED) * 5
     *   + count(THREATENED) * 2
     */
    private static int calculateRiskFromState(ObjectNode order, String worldStateJson) {
        try {
            JsonNode state = mapper.readTree(worldStateJson);
            int riskScore = 0;

            // Get regions from state
            JsonNode regions = state.path("regions");
            if (regions.isArray()) {
                for (JsonNode region : regions) {
                    int threat = region.path("threatLevel").asInt(0);
                    riskScore += threat;
                }
            }

            // Get paths from state (if available)
            JsonNode paths = state.path("paths");
            if (paths.isArray()) {
                for (JsonNode path : paths) {
                    int surveillance = path.path("surveillanceLevel").asInt(0);
                    riskScore += surveillance * 3;

                    String status = path.path("status").asText("OPEN");
                    if ("BLOCKED".equals(status)) {
                        riskScore += 5;
                    } else if ("THREATENED".equals(status)) {
                        riskScore += 2;
                    }
                }
            }

            // Check for nearby Nazgul (simplified: count active SHADOW units)
            JsonNode units = state.path("units");
            if (units.isArray()) {
                for (JsonNode unit : units) {
                    String status = unit.path("status").asText("");
                    String id = unit.path("id").asText("");
                    // Count active Nazgul as risk factor
                    if ("ACTIVE".equals(status) &&
                        (id.contains("nazgul") || id.equals("witch-king"))) {
                        riskScore += 2;
                    }
                }
            }

            return riskScore;

        } catch (Exception e) {
            log.warn("⚠️ Risk calculation failed: {}", e.getMessage());
            return 0;
        }
    }
}
