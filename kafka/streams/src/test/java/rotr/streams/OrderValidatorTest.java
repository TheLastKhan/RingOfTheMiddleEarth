package rotr.streams;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.assertEquals;
import static org.junit.jupiter.api.Assertions.assertFalse;
import static org.junit.jupiter.api.Assertions.assertTrue;

class OrderValidatorTest {

    @BeforeEach
    void reset() {
        OrderValidator.resetForTests();
    }

    @Test
    void rule1RejectsWrongTurn() {
        assertError("""
            {"orderType":"FORTIFY_REGION","playerId":"light-player","unitId":"aragorn","turn":-1}
            """, "WRONG_TURN");
    }

    @Test
    void rule2RejectsWrongSideUnit() {
        assertError("""
            {"orderType":"FORTIFY_REGION","playerId":"dark-player","unitId":"aragorn","turn":0}
            """, "NOT_YOUR_UNIT");
    }

    @Test
    void rule3RejectsBlockedRingBearerNextPath() {
        assertError("""
            {"orderType":"ASSIGN_ROUTE","playerId":"light-player","unitId":"ring-bearer","turn":0,
             "pathIds":["shire-to-bree"],"blockedPaths":["shire-to-bree"]}
            """, "PATH_BLOCKED");
    }

    @Test
    void rule4RejectsRoutePathOutsideConnectedRoute() {
        assertError("""
            {"orderType":"ASSIGN_ROUTE","playerId":"light-player","unitId":"ring-bearer","turn":0,
             "pathIds":["rivendell-to-moria"]}
            """, "INVALID_PATH");
    }

    @Test
    void rule5RejectsBlockWhenUnitNotAtEndpoint() {
        assertError("""
            {"orderType":"BLOCK_PATH","playerId":"dark-player","unitId":"witch-king","turn":0,
             "pathId":"shire-to-bree"}
            """, "UNIT_NOT_ADJACENT");
    }

    @Test
    void rule6RejectsNonAdjacentAttackTarget() {
        assertError("""
            {"orderType":"ATTACK_REGION","playerId":"light-player","unitId":"aragorn","turn":0,
             "unitRegion":"rivendell","targetRegion":"mordor"}
            """, "INVALID_TARGET");
    }

    @Test
    void rule7RejectsMaiaAbilityOnCooldown() {
        assertError("""
            {"orderType":"MAIA_ABILITY","playerId":"light-player","unitId":"gandalf","turn":0,
             "cooldown":2}
            """, "ABILITY_ON_COOLDOWN");
    }

    @Test
    void rule8RejectsDuplicateUnitOrder() {
        String order = """
            {"orderType":"FORTIFY_REGION","playerId":"light-player","unitId":"aragorn","turn":0}
            """;
        assertTrue(OrderValidator.validateAndTrack(order).isValid());

        OrderValidator.Result duplicate = OrderValidator.validateAndTrack(order);
        assertFalse(duplicate.isValid());
        assertEquals("DUPLICATE_UNIT_ORDER", duplicate.getErrorCode());
    }

    @Test
    void validOrderPasses() {
        OrderValidator.Result result = OrderValidator.validateAndTrack("""
            {"orderType":"ASSIGN_ROUTE","playerId":"light-player","unitId":"ring-bearer","turn":0,
             "pathIds":["shire-to-bree","bree-to-rivendell"]}
            """);
        assertTrue(result.isValid());
    }

    private static void assertError(String orderJson, String expectedCode) {
        OrderValidator.Result result = OrderValidator.validateAndTrack(orderJson);
        assertFalse(result.isValid());
        assertEquals(expectedCode, result.getErrorCode());
    }
}
