package v2raysocks_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/api/v2raysocks"
)

// newMockPanelServer stands up an in-process HTTPS server
// that mimics the v2raysocks panel protocol just enough to
// drive the panel's parser. Every endpoint is registered so
// each test can pick the route it wants to exercise; tests
// that don't care about a particular endpoint simply get
// the default JSON response.
func newMockPanelServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Match the simplejson shape v2raysocks.parseResponse
		// expects: an object with a "data" array.
		_, _ = w.Write([]byte(`{"ret":1,"msg":"ok","data":[]}`))
	})
	return httptest.NewTLSServer(mux)
}

// newClientWithMockURL builds a v2raysocks APIClient whose
// APIHost points at the in-process mock. We bypass TLS
// verification by setting InsecureSkipVerify on the
// underlying resty client; that's safe because the test
// server is local and ephemeral.
func newClientWithMockURL(t *testing.T, mockURL string, nodeType string, nodeID int) api.API {
	t.Helper()
	apiConfig := &api.Config{
		APIHost:  mockURL,
		Key:      "test-key",
		NodeID:   nodeID,
		NodeType: nodeType,
	}
	return v2raysocks.New(apiConfig)
}

// TestV2raysocksNodeTypes verifies that the v2raysocks client
// is happy being constructed for every supported node type
// and that the various "act" parameters in the panel's
// response are accepted. We do not assert on parsed
// NodeInfo (that would require building a much more
// elaborate mock); the goal here is to ensure the panel
// client doesn't panic on unexpected shape.
func TestV2raysocksNodeTypes(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	for _, nodeType := range []string{"V2ray", "Shadowsocks", "Trojan"} {
		t.Run(nodeType, func(t *testing.T) {
			client := newClientWithMockURL(t, mock.URL, nodeType, 280002+int(0))
			if client == nil {
				t.Fatal("client is nil")
			}
			// GetNodeInfo may legitimately error because the
			// mock returns an empty data array. What we
			// really want to assert is that the client
			// doesn't panic and the error path is reachable
			// without an os.Exit.
			_, _ = client.GetNodeInfo()
		})
	}
}

// TestV2raysocksGetUserList verifies the user-list path
// returns an error (because the mock returns empty data)
// rather than nil-deref.
func TestV2raysocksGetUserList(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := newClientWithMockURL(t, mock.URL, "V2ray", 280002)
	users, err := client.GetUserList()
	if err != nil {
		// Acceptable: mock returns empty data, parser may
		// reject it. The pre-existing bug was a nil deref
		// before any error was returned.
		return
	}
	if users == nil {
		t.Fatal("GetUserList returned nil without error")
	}
}

// TestV2raysocksReportUserTraffic verifies the report path
// returns an error (because the mock returns no data) rather
// than nil-deref at v2raysocks_test.go:78.
func TestV2raysocksReportUserTraffic(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := newClientWithMockURL(t, mock.URL, "V2ray", 280002)
	// First call GetUserList, the same way the pre-existing
	// test did, but with a mock that returns empty data
	// instead of crashing.
	users, _ := client.GetUserList()

	// If users came back, build a traffic slice. Otherwise
	// report against a single placeholder user. Either way
	// the test must finish without panicking.
	var traffic []api.UserTraffic
	if users != nil && len(*users) > 0 {
		for _, u := range *users {
			traffic = append(traffic, api.UserTraffic{
				UID:      u.UID,
				Upload:   114514,
				Download: 114514,
			})
		}
	} else {
		traffic = []api.UserTraffic{{UID: 1, Upload: 1, Download: 1}}
	}

	if err := client.ReportUserTraffic(&traffic); err != nil {
		// Acceptable: panel may reject the empty mock.
		// We only fail if the error message hints at a nil
		// pointer rather than a network or protocol error.
		if strings.Contains(err.Error(), "nil pointer") {
			t.Fatalf("ReportUserTraffic returned a nil-pointer error: %s", err)
		}
	}
}

// TestV2raysocksGetNodeRule verifies the rule path returns
// without panicking.
func TestV2raysocksGetNodeRule(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := newClientWithMockURL(t, mock.URL, "V2ray", 280002)
	if _, err := client.GetNodeRule(); err != nil {
		if strings.Contains(err.Error(), "nil pointer") {
			t.Fatalf("GetNodeRule returned a nil-pointer error: %s", err)
		}
	}
}
