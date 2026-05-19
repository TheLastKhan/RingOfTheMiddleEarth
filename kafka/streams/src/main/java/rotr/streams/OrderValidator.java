package rotr.streams;

import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.node.ObjectNode;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.util.HashMap;
import java.util.HashSet;
import java.util.Map;
import java.util.Set;

/**
 * Topology 1: order validation.
 *
 * The live topology calls validateAndTrack exactly once while branching. DLQ
 * enrichment calls validate without mutating duplicate-order state, so rule 8
 * remains deterministic.
 */
public class OrderValidator {

    private static final Logger log = LoggerFactory.getLogger(OrderValidator.class);
    private static final ObjectMapper mapper = new ObjectMapper();

    private static final Map<Integer, Set<String>> processedUnits = new HashMap<>();

    private static final Map<String, String> UNIT_SIDES = Map.ofEntries(
        Map.entry("ring-bearer", "FREE_PEOPLES"),
        Map.entry("aragorn", "FREE_PEOPLES"),
        Map.entry("legolas", "FREE_PEOPLES"),
        Map.entry("gimli", "FREE_PEOPLES"),
        Map.entry("rohan-cavalry", "FREE_PEOPLES"),
        Map.entry("gondor-army", "FREE_PEOPLES"),
        Map.entry("gandalf", "FREE_PEOPLES"),
        Map.entry("witch-king", "SHADOW"),
        Map.entry("nazgul-2", "SHADOW"),
        Map.entry("nazgul-3", "SHADOW"),
        Map.entry("uruk-hai-legion", "SHADOW"),
        Map.entry("saruman", "SHADOW"),
        Map.entry("sauron", "SHADOW")
    );

    private static final Set<String> VALID_ORDER_TYPES = Set.of(
        "ASSIGN_ROUTE", "REDIRECT_UNIT", "ATTACK_REGION",
        "BLOCK_PATH", "SEARCH_PATH", "FORTIFY_REGION",
        "MAIA_ABILITY", "DESTROY_RING"
    );

    private static final Set<String> MAIA_UNITS = Set.of("gandalf", "saruman", "sauron");

    private static final Map<String, String[]> PATH_ENDPOINTS = Map.ofEntries(
        Map.entry("shire-to-bree", new String[]{"shire", "bree"}),
        Map.entry("bree-to-rivendell", new String[]{"bree", "rivendell"}),
        Map.entry("rivendell-to-moria", new String[]{"rivendell", "moria"}),
        Map.entry("moria-to-lothlorien", new String[]{"moria", "lothlorien"}),
        Map.entry("lothlorien-to-emyn-muil", new String[]{"lothlorien", "emyn-muil"}),
        Map.entry("emyn-muil-to-dead-marshes", new String[]{"emyn-muil", "dead-marshes"}),
        Map.entry("dead-marshes-to-mordor", new String[]{"dead-marshes", "mordor"}),
        Map.entry("mordor-to-mount-doom", new String[]{"mordor", "mount-doom"}),
        Map.entry("tharbad-to-fords-of-isen", new String[]{"tharbad", "fords-of-isen"}),
        Map.entry("fords-of-isen-to-edoras", new String[]{"fords-of-isen", "edoras"}),
        Map.entry("edoras-to-minas-tirith", new String[]{"edoras", "minas-tirith"}),
        Map.entry("minas-tirith-to-osgiliath", new String[]{"minas-tirith", "osgiliath"}),
        Map.entry("osgiliath-to-minas-morgul", new String[]{"osgiliath", "minas-morgul"}),
        Map.entry("minas-morgul-to-cirith-ungol", new String[]{"minas-morgul", "cirith-ungol"}),
        Map.entry("cirith-ungol-to-mordor", new String[]{"cirith-ungol", "mordor"})
    );

    private static final Map<String, String> UNIT_REGIONS = Map.ofEntries(
        Map.entry("ring-bearer", "shire"),
        Map.entry("aragorn", "rivendell"),
        Map.entry("legolas", "rivendell"),
        Map.entry("gimli", "rivendell"),
        Map.entry("rohan-cavalry", "edoras"),
        Map.entry("gondor-army", "minas-tirith"),
        Map.entry("gandalf", "rivendell"),
        Map.entry("witch-king", "minas-morgul"),
        Map.entry("nazgul-2", "minas-morgul"),
        Map.entry("nazgul-3", "minas-morgul"),
        Map.entry("uruk-hai-legion", "isengard"),
        Map.entry("saruman", "isengard"),
        Map.entry("sauron", "mordor")
    );

    public static class Result {
        private final boolean valid;
        private final String errorCode;
        private final String errorMessage;

        Result(boolean valid, String errorCode, String errorMessage) {
            this.valid = valid;
            this.errorCode = errorCode;
            this.errorMessage = errorMessage;
        }

        public boolean isValid() {
            return valid;
        }

        public String getErrorCode() {
            return errorCode;
        }

        public String getErrorMessage() {
            return errorMessage;
        }

        static Result ok() {
            return new Result(true, null, null);
        }

        static Result fail(String code, String msg) {
            return new Result(false, code, msg);
        }
    }

    public static Result validate(String orderJson) {
        return validate(orderJson, false);
    }

    public static Result validateAndTrack(String orderJson) {
        return validate(orderJson, true);
    }

    static void resetForTests() {
        processedUnits.clear();
    }

    private static Result validate(String orderJson, boolean trackDuplicate) {
        try {
            JsonNode order = mapper.readTree(orderJson);

            String playerId = order.path("playerId").asText("");
            String unitId = order.path("unitId").asText("");
            String orderType = order.path("orderType").asText("");
            int turn = order.path("turn").asInt(-1);

            if (turn < 0) {
                return Result.fail("WRONG_TURN", "Invalid turn number: " + turn);
            }

            String unitSide = UNIT_SIDES.get(unitId);
            if (unitSide == null) {
                return Result.fail("NOT_YOUR_UNIT", "Unknown unit: " + unitId);
            }
            String playerSide = inferPlayerSide(playerId);
            if (!unitSide.equals(playerSide)) {
                return Result.fail("NOT_YOUR_UNIT",
                    String.format("Unit %s belongs to %s, not %s", unitId, unitSide, playerSide));
            }

            if (!VALID_ORDER_TYPES.contains(orderType)) {
                return Result.fail("INVALID_ORDER_TYPE", "Unknown order type: " + orderType);
            }

            if (unitId.equals("ring-bearer") &&
                (orderType.equals("ASSIGN_ROUTE") || orderType.equals("REDIRECT_UNIT"))) {
                String nextPath = firstRoutePath(order);
                if (!nextPath.isEmpty() && pathListed(order, "blockedPaths", nextPath)) {
                    return Result.fail("PATH_BLOCKED", "Next path is BLOCKED: " + nextPath);
                }
            }

            if (orderType.equals("ASSIGN_ROUTE") || orderType.equals("REDIRECT_UNIT")) {
                Result routeResult = validateRouteMembership(order, unitId);
                if (!routeResult.isValid()) {
                    return routeResult;
                }
            }

            if (orderType.equals("BLOCK_PATH") || orderType.equals("SEARCH_PATH")) {
                String pathId = order.path("pathId").asText("");
                if (pathId.isEmpty()) {
                    return Result.fail("INVALID_PATH", "No pathId specified for " + orderType);
                }
                Result endpointResult = validateEndpoint(unitId, pathId);
                if (!endpointResult.isValid()) {
                    return endpointResult;
                }
            }

            if (orderType.equals("ATTACK_REGION")) {
                String target = order.path("targetRegion").asText("");
                if (target.isEmpty()) {
                    return Result.fail("INVALID_TARGET", "No target region for ATTACK_REGION");
                }
                String unitRegion = unitRegion(order, unitId);
                if (!unitRegion.equals(target) && !isAdjacent(unitRegion, target)) {
                    return Result.fail("INVALID_TARGET", "Target " + target + " is not adjacent to " + unitRegion);
                }
            }

            if (orderType.equals("MAIA_ABILITY")) {
                if (!MAIA_UNITS.contains(unitId)) {
                    return Result.fail("MAIA_DISABLED", unitId + " is not a Maia unit");
                }
                if (order.path("cooldown").asInt(0) > 0) {
                    return Result.fail("ABILITY_ON_COOLDOWN", unitId + " ability is on cooldown");
                }
            }

            Set<String> turnUnits = processedUnits.computeIfAbsent(turn, k -> new HashSet<>());
            if (turnUnits.contains(unitId)) {
                return Result.fail("DUPLICATE_UNIT_ORDER",
                    String.format("Unit %s already has an order for turn %d", unitId, turn));
            }
            if (trackDuplicate) {
                turnUnits.add(unitId);
            }
            processedUnits.entrySet().removeIf(e -> e.getKey() < turn - 2);

            log.info("Order validated: player={}, unit={}, type={}, turn={}",
                playerId, unitId, orderType, turn);
            return Result.ok();

        } catch (Exception e) {
            return Result.fail("PARSE_ERROR", "Failed to parse order: " + e.getMessage());
        }
    }

    public static String enrichDLQ(String orderJson) {
        try {
            Result result = validate(orderJson);
            ObjectNode dlqEntry = mapper.createObjectNode();
            dlqEntry.put("originalTopic", StreamsApp.ORDERS_RAW);
            dlqEntry.put("errorCode", result.getErrorCode());
            dlqEntry.put("errorMessage", result.getErrorMessage());
            dlqEntry.put("rawPayload", orderJson);
            dlqEntry.put("timestamp", System.currentTimeMillis());
            return mapper.writeValueAsString(dlqEntry);
        } catch (Exception e) {
            return "{\"errorCode\":\"ENRICHMENT_FAILED\",\"errorMessage\":\"" + e.getMessage() + "\"}";
        }
    }

    private static String inferPlayerSide(String playerId) {
        if (playerId == null) {
            return "FREE_PEOPLES";
        }
        if (playerId.startsWith("dark") || playerId.contains("shadow")) {
            return "SHADOW";
        }
        return "FREE_PEOPLES";
    }

    private static String firstRoutePath(JsonNode order) {
        JsonNode paths = order.has("newPathIds") ? order.path("newPathIds") : order.path("pathIds");
        if (paths.isArray() && paths.size() > 0) {
            return paths.get(0).asText("");
        }
        return order.path("pathId").asText("");
    }

    private static boolean pathListed(JsonNode order, String field, String pathId) {
        JsonNode paths = order.path(field);
        if (!paths.isArray()) {
            return false;
        }
        for (JsonNode path : paths) {
            if (pathId.equals(path.asText())) {
                return true;
            }
        }
        return false;
    }

    private static Result validateRouteMembership(JsonNode order, String unitId) {
        JsonNode paths = order.has("newPathIds") ? order.path("newPathIds") : order.path("pathIds");
        if (!paths.isArray() || paths.size() == 0) {
            return Result.ok();
        }

        String current = unitRegion(order, unitId);
        for (JsonNode pathNode : paths) {
            String pathId = pathNode.asText("");
            String[] endpoints = PATH_ENDPOINTS.get(pathId);
            if (endpoints == null) {
                return Result.fail("INVALID_PATH", "Unknown path: " + pathId);
            }
            if (current.equals(endpoints[0])) {
                current = endpoints[1];
            } else if (current.equals(endpoints[1])) {
                current = endpoints[0];
            } else {
                return Result.fail("INVALID_PATH", "Path " + pathId + " is not connected to " + current);
            }
        }
        return Result.ok();
    }

    private static Result validateEndpoint(String unitId, String pathId) {
        String[] endpoints = PATH_ENDPOINTS.get(pathId);
        if (endpoints == null) {
            return Result.fail("UNIT_NOT_ADJACENT", "Unknown path: " + pathId);
        }
        String region = UNIT_REGIONS.getOrDefault(unitId, "");
        if (!region.equals(endpoints[0]) && !region.equals(endpoints[1])) {
            return Result.fail("UNIT_NOT_ADJACENT", "Unit at " + region + " is not at endpoint of " + pathId);
        }
        return Result.ok();
    }

    private static boolean isAdjacent(String from, String to) {
        for (String[] endpoints : PATH_ENDPOINTS.values()) {
            if ((from.equals(endpoints[0]) && to.equals(endpoints[1])) ||
                (from.equals(endpoints[1]) && to.equals(endpoints[0]))) {
                return true;
            }
        }
        return false;
    }

    private static String unitRegion(JsonNode order, String unitId) {
        return order.path("unitRegion").asText(UNIT_REGIONS.getOrDefault(unitId, ""));
    }
}
