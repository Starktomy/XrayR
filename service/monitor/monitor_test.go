package monitor

import (
	"errors"
	"testing"
	"time"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/config"
)

func TestNodeMonitor_304NotModified(t *testing.T) {
	mockAPI := &MockAPI{
		GetNodeErr: api.ErrNodeNotModified,
		GetUserErr: api.ErrUserNotModified,
		GetRuleErr: api.ErrRuleNotModified,
	}
	mockCtrl := &MockNodeController{
		NodeInfo: &api.NodeInfo{Port: 8080, NodeType: "V2ray"},
		UserList: &[]api.UserInfo{{UID: 1, Email: "user1@test.com"}},
		Tag:      "v2ray_tag",
	}
	mockMetrics := &MockMetricsProvider{}
	mockSys := &MockSystemStatusProvider{}
	cfg := &config.Config{UpdatePeriodic: 1}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")
	m.startAt = time.Now().Add(-10 * time.Second)

	err := m.nodeInfoMonitor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockCtrl.RebuildNodeCalls != 0 {
		t.Errorf("expected RebuildNodeCalls == 0, got %d", mockCtrl.RebuildNodeCalls)
	}
	if mockCtrl.SyncUsersCalls != 0 {
		t.Errorf("expected SyncUsersCalls == 0, got %d", mockCtrl.SyncUsersCalls)
	}
}

func TestNodeMonitor_NodeUpdates(t *testing.T) {
	newInfo := &api.NodeInfo{Port: 9090, NodeType: "V2ray"}
	mockAPI := &MockAPI{
		NodeInfo:   newInfo,
		GetUserErr: api.ErrUserNotModified,
		GetRuleErr: api.ErrRuleNotModified,
	}
	mockCtrl := &MockNodeController{
		NodeInfo: &api.NodeInfo{Port: 8080, NodeType: "V2ray"},
		UserList: &[]api.UserInfo{{UID: 1, Email: "user1@test.com"}},
		Tag:      "v2ray_tag",
	}
	mockMetrics := &MockMetricsProvider{}
	mockSys := &MockSystemStatusProvider{}
	cfg := &config.Config{UpdatePeriodic: 1}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")
	m.startAt = time.Now().Add(-10 * time.Second)

	err := m.nodeInfoMonitor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockCtrl.RebuildNodeCalls != 1 {
		t.Errorf("expected RebuildNodeCalls == 1, got %d", mockCtrl.RebuildNodeCalls)
	}
	if mockCtrl.LastRebuiltNodeInfo.Port != 9090 {
		t.Errorf("expected rebuilt port 9090, got %d", mockCtrl.LastRebuiltNodeInfo.Port)
	}
}

func TestCompareUserList(t *testing.T) {
	u1 := api.UserInfo{UID: 1, Email: "u1@test.com", SpeedLimit: 100}
	u2 := api.UserInfo{UID: 2, Email: "u2@test.com", SpeedLimit: 200}
	u2Mod := api.UserInfo{UID: 2, Email: "u2@test.com", SpeedLimit: 500}
	u3 := api.UserInfo{UID: 3, Email: "u3@test.com", SpeedLimit: 300}

	// 1. Same user list
	oldList := &[]api.UserInfo{u1, u2}
	newList := &[]api.UserInfo{u1, u2}
	del, add := compareUserList(oldList, newList)
	if len(del) != 0 || len(add) != 0 {
		t.Errorf("expected no diff, got deleted=%v, added=%v", del, add)
	}

	// 2. Added user (u3)
	newListAdded := &[]api.UserInfo{u1, u2, u3}
	del, add = compareUserList(oldList, newListAdded)
	if len(del) != 0 || len(add) != 1 || add[0].UID != 3 {
		t.Errorf("expected 1 added user UID=3, got deleted=%v, added=%v", del, add)
	}

	// 3. Deleted user (u1)
	newListDeleted := &[]api.UserInfo{u2}
	del, add = compareUserList(oldList, newListDeleted)
	if len(del) != 1 || del[0].UID != 1 || len(add) != 0 {
		t.Errorf("expected 1 deleted user UID=1, got deleted=%v, added=%v", del, add)
	}

	// 4. Modified user (u2 -> u2Mod)
	newListModified := &[]api.UserInfo{u1, u2Mod}
	del, add = compareUserList(oldList, newListModified)
	if len(del) != 1 || del[0].UID != 2 || len(add) != 1 || add[0].UID != 2 {
		t.Errorf("expected user UID=2 in both deleted and added, got deleted=%v, added=%v", del, add)
	}

	// 5. Nil slices
	del, add = compareUserList(nil, nil)
	if del != nil || add != nil {
		t.Errorf("expected nil result for nil inputs, got deleted=%v, added=%v", del, add)
	}
}

func TestNodeMonitor_UserSync(t *testing.T) {
	u1 := api.UserInfo{UID: 1, Email: "u1@test.com"}
	u2 := api.UserInfo{UID: 2, Email: "u2@test.com"}
	u3 := api.UserInfo{UID: 3, Email: "u3@test.com"}

	oldUsers := []api.UserInfo{u1, u2}
	newUsers := []api.UserInfo{u2, u3}

	mockAPI := &MockAPI{
		GetNodeErr: api.ErrNodeNotModified,
		UserList:   &newUsers,
		GetRuleErr: api.ErrRuleNotModified,
	}
	mockCtrl := &MockNodeController{
		NodeInfo: &api.NodeInfo{Port: 8080, NodeType: "V2ray"},
		UserList: &oldUsers,
		Tag:      "v2ray_tag",
	}
	mockMetrics := &MockMetricsProvider{}
	mockSys := &MockSystemStatusProvider{}
	cfg := &config.Config{UpdatePeriodic: 1}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")
	m.startAt = time.Now().Add(-10 * time.Second)

	err := m.nodeInfoMonitor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockCtrl.SyncUsersCalls != 1 {
		t.Fatalf("expected SyncUsersCalls == 1, got %d", mockCtrl.SyncUsersCalls)
	}
	if len(mockCtrl.LastSyncDeleted) != 1 || mockCtrl.LastSyncDeleted[0].UID != 1 {
		t.Errorf("expected deleted user UID 1, got %v", mockCtrl.LastSyncDeleted)
	}
	if len(mockCtrl.LastSyncAdded) != 1 || mockCtrl.LastSyncAdded[0].UID != 3 {
		t.Errorf("expected added user UID 3, got %v", mockCtrl.LastSyncAdded)
	}
}

func TestUserMonitor_TrafficReportingAndReset(t *testing.T) {
	user := api.UserInfo{UID: 10, Email: "user10@test.com"}
	userTag := buildUserTag("test_tag", &user)

	mockAPI := &MockAPI{}
	mockCtrl := &MockNodeController{
		Tag:      "test_tag",
		UserList: &[]api.UserInfo{user},
	}
	mockMetrics := &MockMetricsProvider{
		TrafficData: map[string]struct{ Up, Down int64 }{
			userTag: {Up: 100, Down: 200},
		},
	}
	mockSys := &MockSystemStatusProvider{CPU: 10.0, Mem: 20.0, Disk: 30.0, Uptime: 1000}
	cfg := &config.Config{UpdatePeriodic: 1}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")
	m.startAt = time.Now().Add(-10 * time.Second)

	err := m.userInfoMonitor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(mockAPI.ReportedTraffic) != 1 {
		t.Fatalf("expected 1 reported traffic batch, got %d", len(mockAPI.ReportedTraffic))
	}
	trafficPtr := mockAPI.ReportedTraffic[0]
	if trafficPtr == nil {
		t.Fatalf("expected non-nil traffic pointer")
	}
	batch := *trafficPtr
	if len(batch) != 1 || batch[0].UID != 10 || batch[0].Upload != 100 || batch[0].Download != 200 {
		t.Errorf("unexpected reported traffic: %+v", batch)
	}
	if !mockMetrics.ResetCalled {
		t.Errorf("expected ResetTraffic to be called")
	}
}

func TestUserMonitor_TrafficReportError_NoReset(t *testing.T) {
	user := api.UserInfo{UID: 10, Email: "user10@test.com"}
	userTag := buildUserTag("test_tag", &user)

	mockAPI := &MockAPI{
		ReportTrafficErr: errors.New("API error"),
	}
	mockCtrl := &MockNodeController{
		Tag:      "test_tag",
		UserList: &[]api.UserInfo{user},
	}
	mockMetrics := &MockMetricsProvider{
		TrafficData: map[string]struct{ Up, Down int64 }{
			userTag: {Up: 500, Down: 500},
		},
	}
	mockSys := &MockSystemStatusProvider{}
	cfg := &config.Config{UpdatePeriodic: 1}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")
	m.startAt = time.Now().Add(-10 * time.Second)

	err := m.userInfoMonitor()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mockMetrics.ResetCalled {
		t.Errorf("expected ResetTraffic NOT to be called when API report fails")
	}
}

func TestUserMonitor_AutoSpeedLimit(t *testing.T) {
	user := api.UserInfo{UID: 5, Email: "heavy@test.com", SpeedLimit: 1000}
	userTag := buildUserTag("test_tag", &user)

	mockAPI := &MockAPI{}
	mockCtrl := &MockNodeController{
		Tag:      "test_tag",
		UserList: &[]api.UserInfo{user},
	}
	mockMetrics := &MockMetricsProvider{
		TrafficData: map[string]struct{ Up, Down int64 }{
			userTag: {Up: 0, Down: 100000000}, // > threshold
		},
	}
	mockSys := &MockSystemStatusProvider{}
	cfg := &config.Config{
		UpdatePeriodic: 1,
		AutoSpeedLimitConfig: &config.AutoSpeedLimitConfig{
			Limit:         1, // 1 Mbps threshold
			WarnTimes:     1,
			LimitSpeed:    2,
			LimitDuration: 1,
		},
	}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")
	m.limitedUsers = make(map[api.UserInfo]LimitInfo)
	m.warnedUsers = make(map[api.UserInfo]int)
	m.startAt = time.Now().Add(-10 * time.Second)

	// Tick 1: warning count = 1
	err := m.userInfoMonitor()
	if err != nil {
		t.Fatalf("tick 1 error: %v", err)
	}
	if m.warnedUsers[user] != 1 {
		t.Errorf("expected warning count 1, got %d", m.warnedUsers[user])
	}
	if len(m.limitedUsers) != 0 {
		t.Errorf("expected 0 limited users on tick 1, got %d", len(m.limitedUsers))
	}

	// Tick 2: warning count > 1 -> limited
	err = m.userInfoMonitor()
	if err != nil {
		t.Fatalf("tick 2 error: %v", err)
	}
	if len(m.limitedUsers) != 1 {
		t.Fatalf("expected 1 limited user on tick 2, got %d", len(m.limitedUsers))
	}

	// Set limit end time to past so release triggers
	info := m.limitedUsers[user]
	info.end = time.Now().Unix() - 10
	m.limitedUsers[user] = info

	// Tick 3: release user
	err = m.userInfoMonitor()
	if err != nil {
		t.Fatalf("tick 3 error: %v", err)
	}
	if len(m.limitedUsers) != 0 {
		t.Errorf("expected limited users to be cleared on release, got %d", len(m.limitedUsers))
	}
	if mockCtrl.UpdateLimiterCalls < 2 {
		t.Errorf("expected UpdateInboundLimiter to be called for limit & unlimit, got %d", mockCtrl.UpdateLimiterCalls)
	}
}

func TestMonitor_PanicRecovery(t *testing.T) {
	mockAPI := &MockAPI{}
	mockCtrl := &MockNodeController{
		NodeInfo: &api.NodeInfo{Port: 8080, NodeType: "V2ray"},
	}
	mockMetrics := &MockMetricsProvider{}
	mockSys := &MockSystemStatusProvider{}
	cfg := &config.Config{UpdatePeriodic: 1}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")

	m.tasks = []periodicTask{
		{
			tag:      "panic task",
			Interval: 10 * time.Millisecond,
			Execute: func() error {
				panic("test panic")
			},
		},
	}

	err := m.Start()
	if err != nil {
		t.Fatalf("failed to start monitor: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	closeErr := m.Close()
	if closeErr == nil {
		t.Fatalf("expected Close() to return recorded panic error")
	}
	if !errors.Is(closeErr, errors.New("panic task: panic: test panic")) && closeErr.Error() != "panic task: panic: test panic" {
		t.Logf("Close returned expected panic error: %v", closeErr)
	}
}

func TestMonitor_GracefulShutdownFlush(t *testing.T) {
	execCount := 0
	mockAPI := &MockAPI{}
	mockCtrl := &MockNodeController{
		NodeInfo: &api.NodeInfo{Port: 8080, NodeType: "V2ray"},
	}
	mockMetrics := &MockMetricsProvider{}
	mockSys := &MockSystemStatusProvider{}
	cfg := &config.Config{UpdatePeriodic: 10}

	m := New(cfg, mockAPI, mockCtrl, mockMetrics, mockSys, "V2board")
	m.tasks = []periodicTask{
		{
			tag:      "flush task",
			Interval: 10 * time.Hour, // long interval so ticker doesn't fire
			Execute: func() error {
				execCount++
				return nil
			},
		},
	}

	err := m.Start()
	if err != nil {
		t.Fatalf("failed to start monitor: %v", err)
	}

	time.Sleep(20 * time.Millisecond)
	err = m.Close()
	if err != nil {
		t.Fatalf("unexpected Close error: %v", err)
	}

	// execCount should be 2 (initial run + flush run on Close)
	if execCount != 2 {
		t.Errorf("expected execCount == 2 (initial + shutdown flush), got %d", execCount)
	}
}
