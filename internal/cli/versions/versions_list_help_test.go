package versions

import (
	"strings"
	"testing"
)

// Apple reports some live versions only through appVersionState
// READY_FOR_DISTRIBUTION, so the help must not send users to READY_FOR_SALE
// alone to find the live version.
func TestVersionsListHelpFindsLiveVersionsUnderBothStateSpellings(t *testing.T) {
	help := VersionsListCommand().LongHelp
	for _, want := range []string{
		"appVersionState READY_FOR_DISTRIBUTION",
		"appStoreState\nREADY_FOR_SALE",
		`asc versions list --app "123456789" --state READY_FOR_DISTRIBUTION --latest`,
		`asc versions list --app "123456789" --state READY_FOR_SALE --latest`,
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("expected versions list help to contain %q, got:\n%s", want, help)
		}
	}
	if strings.Contains(help, "combine it with --state READY_FOR_SALE") {
		t.Fatalf("help still presents READY_FOR_SALE alone as the live state:\n%s", help)
	}
}
