package pricing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
)

// AppBasePriceStatus reports whether an app price schedule has a price for its
// base territory, using the same schedule reads as `asc pricing current`.
type AppBasePriceStatus struct {
	// Configured is false when App Store Connect reports that the app's price
	// schedule was never created.
	Configured bool
	// BaseTerritory is the schedule's base territory ID, such as USA.
	BaseTerritory string
	// HasPrice is true when the base territory has a manual price that is
	// active now or scheduled to start later. A free price counts.
	HasPrice bool
}

// FetchAppBasePriceStatus reads the base territory and manual prices of the
// price schedule scheduleID. A schedule that App Store Connect reports as never
// configured returns Configured=false without an error; every other request or
// decoding failure is returned so callers can treat the price as unverified.
func FetchAppBasePriceStatus(ctx context.Context, client *asc.Client, scheduleID string) (AppBasePriceStatus, error) {
	scheduleID = strings.TrimSpace(scheduleID)
	if client == nil || scheduleID == "" {
		return AppBasePriceStatus{}, fmt.Errorf("app price schedule ID is required")
	}

	baseTerritoryResp, err := getAppPriceScheduleBaseTerritoryWithTimeout(ctx, client, scheduleID)
	if err != nil {
		if isAppPriceScheduleNotConfigured(err) {
			return AppBasePriceStatus{}, nil
		}
		return AppBasePriceStatus{}, fmt.Errorf("get base territory: %w", err)
	}
	baseTerritory := strings.ToUpper(strings.TrimSpace(baseTerritoryResp.Data.ID))
	if baseTerritory == "" {
		return AppBasePriceStatus{}, fmt.Errorf("base territory missing from response")
	}

	rawPrices := 0
	entries, _, _, err := fetchAppSchedulePriceEntries(ctx, func(callCtx context.Context, opts ...asc.AppPriceSchedulePricesOption) (*asc.AppPricesResponse, error) {
		resp, err := client.GetAppPriceScheduleManualPrices(callCtx, scheduleID, opts...)
		if resp != nil {
			rawPrices += len(resp.Data)
		}
		return resp, err
	})
	if err != nil {
		if isAppPriceScheduleNotConfigured(err) {
			return AppBasePriceStatus{BaseTerritory: baseTerritory}, nil
		}
		return AppBasePriceStatus{}, fmt.Errorf("fetch manual prices: %w", err)
	}

	status := AppBasePriceStatus{
		Configured:    true,
		BaseTerritory: baseTerritory,
		HasPrice:      hasCurrentOrScheduledPrice(entries, baseTerritory, time.Now().UTC()),
	}
	if !status.HasPrice && len(entries) < rawPrices {
		// Some manual prices could not be attributed to a territory, so the base
		// territory may still have one.
		return AppBasePriceStatus{}, fmt.Errorf("could not resolve the territory of %d manual price(s)", rawPrices-len(entries))
	}
	return status, nil
}

func hasCurrentOrScheduledPrice(entries []appPriceEntry, territoryID string, now time.Time) bool {
	territoryID = strings.ToUpper(strings.TrimSpace(territoryID))
	today := dateOnlyUTC(now)
	for _, entry := range entries {
		if entry.TerritoryID != territoryID {
			continue
		}
		if entry.EndAt == nil || !entry.EndAt.Before(today) {
			return true
		}
	}
	return false
}
