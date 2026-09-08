package simulation

import (
	"fmt"
	"strings"
)

// This explains displayed accounting without diagnosing raw physical excess
// from rounded inputs. It never changes an overlap, gap, threshold or severity.
func energyPathAllocationOverlapMessage(endUse, carrier string, value float64, unit, period string) string {
	message := fmt.Sprintf("Rounded reported allocation totals have %g %s of positive overlap for %s %s.", value, unit, endUse, carrier)
	if strings.EqualFold(strings.TrimSpace(period), "annual") {
		message += " Annual sums positive overlaps from constituent periods without cancelling gaps; this is not necessarily the net annual difference."
	}
	return message + " Compare the net residual and original source precision/ownership before interpreting this as physical excess. Direct observations are retained; no negative remainder is allocated."
}
