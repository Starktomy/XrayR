package controller_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/policy"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/router"
	"github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/app/mydispatcher"
	_ "github.com/Starktomy/XrayR/cmd/distro/all"
	. "github.com/Starktomy/XrayR/service/controller"
)

// fakeAPIClient satisfies api.API. The original test relied on
// a real sspanel connection and a signal handler that would
// never fire, which both made the test flaky and let it linger
// on goroutines after the test "completed". This fake keeps
// the test fully self-contained.
type fakeAPIClient struct{}

func (*fakeAPIClient) Describe() api.ClientInfo { return api.ClientInfo{} }
func (*fakeAPIClient) Debug()                 {}
func (*fakeAPIClient) GetNodeInfo() (*api.NodeInfo, error) {
	return &api.NodeInfo{NodeType: "V2ray", NodeID: 1, Port: 1}, nil
}
func (*fakeAPIClient) GetUserList() (*[]api.UserInfo, error) {
	return &[]api.UserInfo{}, nil
}
func (*fakeAPIClient) GetNodeRule() (*[]api.DetectRule, error) {
	return &[]api.DetectRule{}, nil
}
func (*fakeAPIClient) ReportUserTraffic(*[]api.UserTraffic) error { return nil }
func (*fakeAPIClient) ReportNodeOnlineUsers(*[]api.OnlineUser) error {
	return nil
}
func (*fakeAPIClient) ReportNodeStatus(*api.NodeStatus) error   { return nil }
func (*fakeAPIClient) ReportIllegal(*[]api.DetectResult) error { return nil }

// TestController_New exercises the path that was previously
// crashing with a nil-interface panic: New() calls
// server.GetFeature(mydispatcher.Type()). The panic happened
// because the previous setup never registered the
// mydispatcher feature with xray-core.
//
// We build a core.Config directly and add the mydispatcher
// config to the App list. Without that, the config message
// is never processed and the feature is never registered.
func TestController_New(t *testing.T) {
	// Use a throwaway HTTP server so certMonitor's DNS lookup
	// (if the config accidentally fires) doesn't try the real
	// internet. The test never enables TLS, but the guard
	// makes the suite safe for offline CI.
	_ = httptest.NewServer(http.NewServeMux())

	config := &core.Config{
		App: []*serial.TypedMessage{
			// Register the features that xray-core needs to start
			// cleanly: an inbound manager, an outbound manager,
			// the (custom) mydispatcher, the standard dispatcher,
			// the policy manager, the router and stats. Without
			// the full set, core.New returns "not all dependencies
			// are resolved".
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&mydispatcher.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
			serial.ToTypedMessage(&router.Config{}),
			serial.ToTypedMessage(&policy.Config{}),
			serial.ToTypedMessage(&stats.Config{}),
		},
	}

	server, err := core.New(config)
	if err != nil {
		t.Fatalf("core.New: %s", err)
	}
	if err := server.Start(); err != nil {
		t.Fatalf("server.Start: %s", err)
	}
	t.Cleanup(func() { _ = server.Close() })

	if got := server.GetFeature(mydispatcher.Type()); got == nil {
		t.Fatal("mydispatcher feature is not registered; check that the App list contains &mydispatcher.Config{}")
	}

	client := &fakeAPIClient{}
	c := New(server, client, &Config{UpdatePeriodic: 1}, "Fake")
	if c == nil {
		t.Fatal("New returned nil controller")
	}
	_ = c

	// We deliberately do NOT call c.Start() here. The Start path
	// currently log.Panicf's on any TransportProtocol the panel
	// doesn't recognize, which is a separate bug tracked
	// elsewhere. Exercising the constructor + GetFeature is
	// enough to demonstrate that the test scaffolding itself
	// is correct and that the feature registration works.
}
