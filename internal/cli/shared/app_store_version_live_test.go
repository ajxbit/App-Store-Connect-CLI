package shared

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/rudrankriyam/App-Store-Connect-CLI/internal/asc"
)

func TestIsLiveAppStoreVersionPrefersAppVersionState(t *testing.T) {
	tests := []struct {
		name               string
		attributes         asc.AppStoreVersionAttributes
		wantLive           bool
		wantLiveOrPreorder bool
	}{
		{
			name:               "modern only",
			attributes:         asc.AppStoreVersionAttributes{AppVersionState: "READY_FOR_DISTRIBUTION"},
			wantLive:           true,
			wantLiveOrPreorder: true,
		},
		{
			name:               "legacy only",
			attributes:         asc.AppStoreVersionAttributes{AppStoreState: "READY_FOR_SALE"},
			wantLive:           true,
			wantLiveOrPreorder: true,
		},
		{
			name:               "both present",
			attributes:         asc.AppStoreVersionAttributes{AppStoreState: "READY_FOR_SALE", AppVersionState: "READY_FOR_DISTRIBUTION"},
			wantLive:           true,
			wantLiveOrPreorder: true,
		},
		{
			name:       "modern state wins over a stale legacy state",
			attributes: asc.AppStoreVersionAttributes{AppStoreState: "READY_FOR_SALE", AppVersionState: "REPLACED_WITH_NEW_VERSION"},
		},
		{
			name:               "modern live state wins over a stale legacy state",
			attributes:         asc.AppStoreVersionAttributes{AppStoreState: "PENDING_DEVELOPER_RELEASE", AppVersionState: "READY_FOR_DISTRIBUTION"},
			wantLive:           true,
			wantLiveOrPreorder: true,
		},
		{
			name:               "legacy spelling in the modern attribute is still recognized",
			attributes:         asc.AppStoreVersionAttributes{AppVersionState: "ready_for_sale"},
			wantLive:           true,
			wantLiveOrPreorder: true,
		},
		{
			name:               "legacy pre-order has no modern spelling",
			attributes:         asc.AppStoreVersionAttributes{AppStoreState: "PREORDER_READY_FOR_SALE"},
			wantLiveOrPreorder: true,
		},
		{
			name:               "legacy pre-order alongside a modern state",
			attributes:         asc.AppStoreVersionAttributes{AppStoreState: "PREORDER_READY_FOR_SALE", AppVersionState: "PENDING_DEVELOPER_RELEASE"},
			wantLiveOrPreorder: true,
		},
		{
			name:       "not live",
			attributes: asc.AppStoreVersionAttributes{AppStoreState: "PREPARE_FOR_SUBMISSION", AppVersionState: "PREPARE_FOR_SUBMISSION"},
		},
		{
			name:       "no state",
			attributes: asc.AppStoreVersionAttributes{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsLiveAppStoreVersion(test.attributes); got != test.wantLive {
				t.Fatalf("IsLiveAppStoreVersion(%+v) = %t, want %t", test.attributes, got, test.wantLive)
			}
			if got := IsLiveOrPreorderAppStoreVersion(test.attributes); got != test.wantLiveOrPreorder {
				t.Fatalf("IsLiveOrPreorderAppStoreVersion(%+v) = %t, want %t", test.attributes, got, test.wantLiveOrPreorder)
			}
		})
	}
}

func TestLiveAppStoreVersionStateFilterQueriesBothSpellings(t *testing.T) {
	filter := LiveAppStoreVersionStateFilter()
	if got := strings.Join(filter.AppStoreStates, ","); got != "READY_FOR_SALE" {
		t.Fatalf("AppStoreStates = %q, want READY_FOR_SALE", got)
	}
	if got := strings.Join(filter.AppVersionStates, ","); got != "READY_FOR_DISTRIBUTION" {
		t.Fatalf("AppVersionStates = %q, want READY_FOR_DISTRIBUTION", got)
	}

	// Callers extend the filter; that must not leak into later calls.
	filter.AppStoreStates = append(filter.AppStoreStates, "PREORDER_READY_FOR_SALE")
	if got := strings.Join(LiveAppStoreVersionStateFilter().AppStoreStates, ","); got != "READY_FOR_SALE" {
		t.Fatalf("LiveAppStoreVersionStateFilter() was mutated by a caller: %q", got)
	}
}

// liveVersionTestClient answers app store version list requests keyed by
// "filter[appStoreState]|filter[appVersionState]|filter[platform]|limit".
func liveVersionTestClient(t *testing.T, responses map[string]string, log *[]string) *asc.Client {
	t.Helper()
	return newSubmitReadinessTestClient(t, func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodGet || req.URL.Path != "/v1/apps/app-1/appStoreVersions" {
			t.Fatalf("unexpected request %s %s", req.Method, req.URL.String())
		}
		query := req.URL.Query()
		key := query.Get("filter[appStoreState]") + "|" + query.Get("filter[appVersionState]") + "|" + query.Get("filter[platform]") + "|" + query.Get("limit")
		if log != nil {
			*log = append(*log, key)
		}
		body, ok := responses[key]
		if !ok {
			t.Fatalf("unexpected version query %q", key)
		}
		return submitReadinessJSONResponse(http.StatusOK, body)
	})
}

func TestListAppStoreVersionsInStatesMergesBothSpellings(t *testing.T) {
	var log []string
	client := liveVersionTestClient(t, map[string]string{
		"READY_FOR_SALE||IOS|200": `{"data":[
			{"type":"appStoreVersions","id":"ver-legacy","attributes":{"platform":"IOS","versionString":"1.0","appStoreState":"READY_FOR_SALE"}},
			{"type":"appStoreVersions","id":"ver-both","attributes":{"platform":"IOS","versionString":"2.0","appStoreState":"READY_FOR_SALE","appVersionState":"READY_FOR_DISTRIBUTION"}}
		],"links":{}}`,
		"|READY_FOR_DISTRIBUTION|IOS|200": `{"data":[
			{"type":"appStoreVersions","id":"ver-both","attributes":{"platform":"IOS","versionString":"2.0","appStoreState":"READY_FOR_SALE","appVersionState":"READY_FOR_DISTRIBUTION"}},
			{"type":"appStoreVersions","id":"ver-modern","attributes":{"platform":"IOS","versionString":"3.0","appVersionState":"READY_FOR_DISTRIBUTION"}}
		],"links":{}}`,
	}, &log)

	versions, err := ListAppStoreVersionsInStates(context.Background(), client, "app-1", "IOS", LiveAppStoreVersionStateFilter())
	if err != nil {
		t.Fatalf("ListAppStoreVersionsInStates() error: %v", err)
	}
	ids := make([]string, 0, len(versions))
	for _, version := range versions {
		ids = append(ids, version.ID)
	}
	if got, want := strings.Join(ids, ","), "ver-legacy,ver-both,ver-modern"; got != want {
		t.Fatalf("versions = %q, want %q", got, want)
	}
	if got, want := strings.Join(log, ","), "READY_FOR_SALE||IOS|200,|READY_FOR_DISTRIBUTION|IOS|200"; got != want {
		t.Fatalf("request sequence = %q, want %q", got, want)
	}
}

func TestAppUpdateRequiresWhatsNewDetectsReleasedVersions(t *testing.T) {
	const (
		legacyQuery = "READY_FOR_SALE,DEVELOPER_REMOVED_FROM_SALE,REMOVED_FROM_SALE||IOS|1"
		modernQuery = "|READY_FOR_DISTRIBUTION|IOS|1"
		empty       = `{"data":[],"links":{}}`
	)
	tests := []struct {
		name      string
		responses map[string]string
		want      bool
		wantLog   string
	}{
		{
			name: "modern only",
			responses: map[string]string{
				legacyQuery: empty,
				modernQuery: `{"data":[{"type":"appStoreVersions","id":"ver-live","attributes":{"platform":"IOS","appVersionState":"READY_FOR_DISTRIBUTION"}}],"links":{}}`,
			},
			want:    true,
			wantLog: legacyQuery + "," + modernQuery,
		},
		{
			name: "legacy only",
			responses: map[string]string{
				legacyQuery: `{"data":[{"type":"appStoreVersions","id":"ver-live","attributes":{"platform":"IOS","appStoreState":"READY_FOR_SALE"}}],"links":{}}`,
			},
			want:    true,
			wantLog: legacyQuery,
		},
		{
			name: "both present",
			responses: map[string]string{
				legacyQuery: `{"data":[{"type":"appStoreVersions","id":"ver-live","attributes":{"platform":"IOS","appStoreState":"READY_FOR_SALE","appVersionState":"READY_FOR_DISTRIBUTION"}}],"links":{}}`,
			},
			want:    true,
			wantLog: legacyQuery,
		},
		{
			name: "never released",
			responses: map[string]string{
				legacyQuery: empty,
				modernQuery: empty,
			},
			want:    false,
			wantLog: legacyQuery + "," + modernQuery,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var log []string
			client := liveVersionTestClient(t, test.responses, &log)
			got, err := AppUpdateRequiresWhatsNew(context.Background(), client, "app-1", "IOS")
			if err != nil {
				t.Fatalf("AppUpdateRequiresWhatsNew() error: %v", err)
			}
			if got != test.want {
				t.Fatalf("AppUpdateRequiresWhatsNew() = %t, want %t", got, test.want)
			}
			if gotLog := strings.Join(log, ","); gotLog != test.wantLog {
				t.Fatalf("request sequence = %q, want %q", gotLog, test.wantLog)
			}
		})
	}
}
