package controller

import (
	"reflect"
	"testing"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/common/mylego"
	"github.com/Starktomy/XrayR/service/nodebuilder"
)

func TestCompareUserListEmpty(t *testing.T) {
	old := &[]api.UserInfo{}
	new := &[]api.UserInfo{}
	deleted, added := compareUserList(old, new)
	if len(deleted) != 0 || len(added) != 0 {
		t.Errorf("expected no diff on empty input, got deleted=%v added=%v", deleted, added)
	}
}

func TestCompareUserListAllAdded(t *testing.T) {
	old := &[]api.UserInfo{}
	new := &[]api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
	}
	_, added := compareUserList(old, new)
	if !reflect.DeepEqual(added, *new) {
		t.Errorf("expected all-new users to be reported as added, got %v", added)
	}
}

func TestCompareUserListAllDeleted(t *testing.T) {
	old := &[]api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
	}
	new := &[]api.UserInfo{}
	deleted, _ := compareUserList(old, new)
	if !reflect.DeepEqual(deleted, *old) {
		t.Errorf("expected all-old users to be reported as deleted, got %v", deleted)
	}
}

func TestCompareUserListNoChange(t *testing.T) {
	users := []api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x"},
	}
	old := &users
	new := &[]api.UserInfo{
		{UID: 1, Email: "alice@x"},
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x"},
	}
	deleted, added := compareUserList(old, new)
	if len(deleted) != 0 || len(added) != 0 {
		t.Errorf("expected no diff on identical input, got deleted=%v added=%v", deleted, added)
	}
}

func TestCompareUserListMixedDiff(t *testing.T) {
	old := &[]api.UserInfo{
		{UID: 1, Email: "alice@x", SpeedLimit: 100},
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x", SpeedLimit: 100},
	}
	new := &[]api.UserInfo{
		// alice unchanged
		{UID: 1, Email: "alice@x", SpeedLimit: 100},
		// bob removed
		// carol's SpeedLimit changed -> should appear as
		// delete + add (matches pre-existing behaviour)
		{UID: 3, Email: "carol@x", SpeedLimit: 200},
		// dan added
		{UID: 4, Email: "dan@x"},
	}
	deleted, added := compareUserList(old, new)
	wantDeleted := []api.UserInfo{
		{UID: 2, Email: "bob@x"},
		{UID: 3, Email: "carol@x", SpeedLimit: 100},
	}
	wantAdded := []api.UserInfo{
		{UID: 3, Email: "carol@x", SpeedLimit: 200},
		{UID: 4, Email: "dan@x"},
	}
	if !reflect.DeepEqual(deleted, wantDeleted) {
		t.Errorf("deleted: got %v, want %v", deleted, wantDeleted)
	}
	if !reflect.DeepEqual(added, wantAdded) {
		t.Errorf("added: got %v, want %v", added, wantAdded)
	}
}

func TestCompareUserListUnsorted(t *testing.T) {
	// The algorithm sorts internally, so input order must
	// not matter. Provide both lists in a different order
	// than the algorithm would produce and confirm the diff
	// is the same.
	old := &[]api.UserInfo{
		{UID: 3, Email: "carol@x"},
		{UID: 1, Email: "alice@x"},
	}
	new := &[]api.UserInfo{
		{UID: 4, Email: "dan@x"},
		{UID: 1, Email: "alice@x"},
	}
	deleted, added := compareUserList(old, new)
	if len(deleted) != 1 || deleted[0].UID != 3 {
		t.Errorf("deleted: got %v", deleted)
	}
	if len(added) != 1 || added[0].UID != 4 {
		t.Errorf("added: got %v", added)
	}
}

func TestControllerGettersSetters(t *testing.T) {
	c := &Controller{}
	ni := &api.NodeInfo{Port: 8080, NodeType: "V2ray"}
	c.SetNodeInfo(ni)
	if got := c.GetNodeInfo(); got != ni {
		t.Errorf("GetNodeInfo: got %v, want %v", got, ni)
	}

	ul := &[]api.UserInfo{{UID: 1, Email: "test@example.com"}}
	c.SetUserList(ul)
	if got := c.GetUserList(); got != ul {
		t.Errorf("GetUserList: got %v, want %v", got, ul)
	}

	tag := "V2ray_127.0.0.1_8080"
	c.SetTag(tag)
	if got := c.GetTag(); got != tag {
		t.Errorf("GetTag: got %s, want %s", got, tag)
	}
}

func TestControllerBuildTags(t *testing.T) {
	c := &Controller{
		config: &Config{
			ListenIP: "0.0.0.0",
		},
	}
	c.SetNodeInfo(&api.NodeInfo{
		NodeType: "Vmess",
		Port:     10080,
	})
	c.SetTag(c.buildNodeTag())
	if c.GetTag() != "Vmess_0.0.0.0_10080" {
		t.Errorf("unexpected buildNodeTag: %s", c.GetTag())
	}

	u := &api.UserInfo{
		Email: "user@test.com",
		UID:   42,
	}
	userTag := c.buildUserTag(u)
	if userTag != "Vmess_0.0.0.0_10080|user@test.com|42" {
		t.Errorf("unexpected buildUserTag: %s", userTag)
	}
}

func TestControllerSetBuilder(t *testing.T) {
	c := &Controller{}
	c.SetBuilder(nil) // should not panic or set nil
	if c.builder != nil {
		t.Errorf("expected builder to remain nil when passing nil")
	}
}

type dummyCertResolver struct{}

func (d *dummyCertResolver) GetCertFile(certConfig *mylego.CertConfig) (string, string, error) {
	return "/path/to/cert", "/path/to/key", nil
}

type dummySystemStatusProvider struct{}

func (d *dummySystemStatusProvider) GetSystemInfo() (float64, float64, float64, uint64, error) {
	return 10.0, 20.0, 30.0, 100, nil
}

func TestControllerDependencyInjectionOptions(t *testing.T) {
	resolver := &dummyCertResolver{}
	sysStatus := &dummySystemStatusProvider{}
	builder := nodebuilder.New(resolver)

	cfg := &Config{}
	c := New(nil, nil, cfg, "SSPanel",
		WithCertResolver(resolver),
		WithBuilder(builder),
		WithSystemStatusProvider(sysStatus),
	)

	if c.GetCertResolver() != resolver {
		t.Errorf("expected certResolver %v, got %v", resolver, c.GetCertResolver())
	}
	if c.GetBuilder() != builder {
		t.Errorf("expected builder %v, got %v", builder, c.GetBuilder())
	}
	if c.GetSystemStatusProvider() != sysStatus {
		t.Errorf("expected systemStatus %v, got %v", sysStatus, c.GetSystemStatusProvider())
	}
}

func TestControllerSettersAndGetters(t *testing.T) {
	c := &Controller{}
	resolver := &dummyCertResolver{}
	sysStatus := &dummySystemStatusProvider{}

	c.SetCertResolver(resolver)
	if c.GetCertResolver() != resolver {
		t.Errorf("expected certResolver %v, got %v", resolver, c.GetCertResolver())
	}
	if c.GetBuilder() == nil {
		t.Errorf("expected builder to be instantiated with certResolver")
	}

	c.SetSystemStatusProvider(sysStatus)
	if c.GetSystemStatusProvider() != sysStatus {
		t.Errorf("expected sysStatus %v, got %v", sysStatus, c.GetSystemStatusProvider())
	}

	c.SetMonitor(nil)
	if c.GetMonitor() != nil {
		t.Errorf("expected monitor to be nil")
	}
}

func TestControllerCloseNilMonitor(t *testing.T) {
	c := &Controller{}
	if err := c.Close(); err != nil {
		t.Errorf("expected nil error closing controller with nil monitor, got: %v", err)
	}
}
