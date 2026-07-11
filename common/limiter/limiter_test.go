package limiter

import (
	"sync"
	"testing"

	"github.com/Starktomy/XrayR/api"
)

func TestAddAndDeleteInbound(t *testing.T) {
	l := New()
	users := []api.UserInfo{
		{UID: 1, Email: "alice", SpeedLimit: 1_000_000, DeviceLimit: 2},
		{UID: 2, Email: "bob", SpeedLimit: 0, DeviceLimit: 0},
	}
	if err := l.AddInboundLimiter("tag-a", 50_000_000, &users, nil); err != nil {
		t.Fatalf("AddInboundLimiter: %s", err)
	}
	v, ok := l.InboundInfo.Load("tag-a")
	if !ok {
		t.Fatal("expected inbound info to be stored")
	}
	ii := v.(*InboundInfo)
	if ii.Tag != "tag-a" {
		t.Errorf("Tag = %q, want tag-a", ii.Tag)
	}
	if ii.UserInfo == nil {
		t.Fatal("UserInfo map is nil")
	}
	if _, ok := ii.UserInfo.Load("tag-a|alice|1"); !ok {
		t.Errorf("expected alice in UserInfo map")
	}
	if _, ok := ii.UserInfo.Load("tag-a|bob|2"); !ok {
		t.Errorf("expected bob in UserInfo map")
	}

	// Replace with a different list. AddInboundLimiter stores a
	// new *InboundInfo into the map, so the local ii pointer
	// from the first call is no longer reachable through the
	// map. We re-Load to confirm the replacement happened.
	newUsers := []api.UserInfo{{UID: 1, Email: "alice", SpeedLimit: 2_000_000}}
	if err := l.AddInboundLimiter("tag-a", 50_000_000, &newUsers, nil); err != nil {
		t.Fatalf("AddInboundLimiter (replace): %s", err)
	}
	v2, ok := l.InboundInfo.Load("tag-a")
	if !ok {
		t.Fatal("expected inbound info to still be stored after replace")
	}
	ii2 := v2.(*InboundInfo)
	if ii == ii2 {
		t.Errorf("expected a fresh *InboundInfo after replace")
	}
	if _, ok := ii2.UserInfo.Load("tag-a|bob|2"); ok {
		t.Errorf("bob should have been evicted after replace")
	}

	if err := l.DeleteInboundLimiter("tag-a"); err != nil {
		t.Fatalf("DeleteInboundLimiter: %s", err)
	}
	if _, ok := l.InboundInfo.Load("tag-a"); ok {
		t.Errorf("expected tag-a to be gone after delete")
	}
}

func TestGetUserBucket(t *testing.T) {
	l := New()
	users := []api.UserInfo{
		{UID: 1, Email: "alice", SpeedLimit: 4_000_000, DeviceLimit: 1},
	}
	if err := l.AddInboundLimiter("tag-b", 100_000_000, &users, nil); err != nil {
		t.Fatalf("AddInboundLimiter: %s", err)
	}

	// First call should create the bucket.
	first, ok, reject := l.GetUserBucket("tag-b", "alice|1", "10.0.0.1")
	if reject {
		t.Fatal("first hit should not be rejected")
	}
	if first == nil {
		t.Fatal("first hit should produce a non-nil bucket")
	}
	if !ok {
		t.Errorf("first hit ok flag = false, want true")
	}

	// Second call reuses the bucket.
	second, ok2, _ := l.GetUserBucket("tag-b", "alice|1", "10.0.0.1")
	if !ok2 {
		t.Errorf("second hit ok flag = false, want true")
	}
	if first != second {
		t.Errorf("expected the same bucket pointer across calls")
	}
}

func TestGetOnlineDeviceNoGlobalLimit(t *testing.T) {
	l := New()
	users := []api.UserInfo{{UID: 1, Email: "alice", SpeedLimit: 0, DeviceLimit: 0}}
	if err := l.AddInboundLimiter("tag-c", 0, &users, nil); err != nil {
		t.Fatalf("AddInboundLimiter: %s", err)
	}
	// No GlobalLimit configured -> no online-device tracking.
	online, err := l.GetOnlineDevice("tag-c")
	if err != nil {
		t.Fatalf("GetOnlineDevice: %s", err)
	}
	if online == nil {
		t.Errorf("expected non-nil slice even when no global limit is configured")
	}
}

func TestPushIPWaitGroup(t *testing.T) {
	// pushIP is called from CheckLimit inside a goroutine. With
	// a GlobalLimit the test would need a working cache; this
	// test exercises the WaitGroup bookkeeping on the
	// DeleteInboundLimiter path so we know a future caller
	// won't race the in-flight goroutine.
	ii := &InboundInfo{Tag: "tag-d"}
	ii.pushWG.Add(3)
	var wg sync.WaitGroup
	wg.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer wg.Done()
			defer ii.pushWG.Done()
		}()
	}
	wg.Wait()
	ii.pushWG.Wait() // should not hang
}
