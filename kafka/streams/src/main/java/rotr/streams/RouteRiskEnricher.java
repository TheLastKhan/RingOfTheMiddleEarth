package rotr.streams;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ArrayNode;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.HashSet;
import java.util.Set;

/**
 * Adds the V2 route analysis fields to validated route orders.
 */
public class RouteRiskEnricher {

    private static final Logger log = LoggerFactory.getLogger(RouteRiskEnricher.class);
    private static final ObjectMapper mapper = new ObjectMapper();

    public static String enrich(String orderJson, String worldStateJson) {
        try {
            ObjectNode order = (ObjectNode) mapper.readTree(orderJson);
            String orderType = order.path("orderType").asText("");
            if (!orderType.equals("ASSIGN_ROUTE") && !orderType.equals("REDIRECT_UNIT")) {
                order.putNull("routeRiskScore");
                order.set("threatenedPaths", mapper.createArrayNode());
                order.set("blockedPaths", mapper.createArrayNode());
                return mapper.writeValueAsString(order);
            }

            Risk risk = worldStateJson == null ? new Risk() : calculateRiskFromState(order, worldStateJson);
            order.put("routeRiskScore", risk.score);
            order.set("threatenedPaths", risk.threatenedPaths);
            order.set("blockedPaths", risk.blockedPaths);
            return mapper.writeValueAsString(order);
        } catch (Exception e) {
            log.warn("Risk enrichment failed: {}", e.getMessage());
            return orderJson;
        }
    }

    private static Risk calculateRiskFromState(ObjectNode order, String worldStateJson) {
        Risk risk = new Risk();
        try {
            JsonNode state = mapper.readTree(worldStateJson);
            Set<String> routePaths = extractRoutePaths(order);
            Set<String> routeRegions = new HashSet<>();

            JsonNode paths = state.path("paths");
            if (paths.isArray()) {
                for (JsonNode path : paths) {
                    String pathId = path.path("id").asText("");
                    if (!routePaths.contains(pathId)) {
                        continue;
                    }
                    risk.score += path.path("surveillanceLevel").asInt(0) * 3;
                    String status = path.path("status").asText("OPEN");
                    if ("BLOCKED".equals(status)) {
                        risk.score += 5;
                        risk.blockedPaths.add(pathId);
                    } else if ("THREATENED".equals(status)) {
                        risk.score += 2;
                        risk.threatenedPaths.add(pathId);
                    }
                    addIfPresent(routeRegions, path.path("from").asText(""));
                    addIfPresent(routeRegions, path.path("to").asText(""));
                }
            }

            JsonNode regions = state.path("regions");
            if (regions.isArray()) {
                for (JsonNode region : regions) {
                    String regionId = region.path("id").asText("");
                    if (routeRegions.isEmpty() || routeRegions.contains(regionId)) {
                        risk.score += region.path("threatLevel").asInt(0);
                    }
                }
            }

            JsonNode units = state.path("units");
            if (units.isArray()) {
                for (JsonNode unit : units) {
                    String status = unit.path("status").asText("");
                    String id = unit.path("id").asText("");
                    if ("ACTIVE".equals(status) && (id.contains("nazgul") || id.equals("witch-king"))) {
                        risk.score += 2;
                    }
                }
            }
        } catch (Exception e) {
            log.warn("Risk calculation failed: {}", e.getMessage());
        }
        return risk;
    }

    private static Set<String> extractRoutePaths(ObjectNode order) {
        Set<String> paths = new HashSet<>();
        JsonNode pathIds = order.path("pathIds");
        if (!pathIds.isArray()) {
            pathIds = order.path("newPathIds");
        }
        if (pathIds.isArray()) {
            for (JsonNode node : pathIds) {
                addIfPresent(paths, node.asText(""));
            }
        }
        addIfPresent(paths, order.path("pathId").asText(""));
        return paths;
    }

    private static void addIfPresent(Set<String> values, String value) {
        if (value != null && !value.isBlank()) {
            values.add(value);
        }
    }

    private static class Risk {
        int score = 0;
        ArrayNode threatenedPaths = mapper.createArrayNode();
        ArrayNode blockedPaths = mapper.createArrayNode();
    }
}
