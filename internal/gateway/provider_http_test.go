package gateway

import "testing"

func TestProviderHTTPRouteHasOnePrivateDestination(t *testing.T) {
	origin := "http://api:3003"
	route := Route{Target: "ar-11111111111111111111111111111111", Protocol: "http", Endpoint: origin + "/internal/gateway/provider"}
	if !validRouteForAPI(route, origin) {
		t.Fatal("configured private API route rejected")
	}
	for _, endpoint := range []string{
		"http://127.0.0.1:3003/internal/gateway/provider",
		"http://api:3003/internal/gateway/request",
		"http://api:3003/internal/gateway/provider?target=other",
		"http://user:password@api:3003/internal/gateway/provider",
		"https://external.example/internal/gateway/provider",
	} {
		changed := route
		changed.Endpoint = endpoint
		if validRouteForAPI(changed, origin) {
			t.Fatalf("unreviewed private route accepted: %s", endpoint)
		}
	}
	for _, protocol := range []string{"mcp", "a2a"} {
		changed := route
		changed.Protocol = protocol
		if validRouteForAPI(changed, origin) {
			t.Fatal("private HTTP exception leaked into external protocol")
		}
	}
}
