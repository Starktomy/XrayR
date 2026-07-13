package monitor

import (
	"sync"
	"sync/atomic"

	"github.com/xtls/xray-core/features/stats"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/common/limiter"
)

// mockCounter implements stats.Counter interface from xray-core
type mockCounter struct {
	value int64
}

func (c *mockCounter) Value() int64 {
	return atomic.LoadInt64(&c.value)
}

func (c *mockCounter) Set(v int64) int64 {
	atomic.StoreInt64(&c.value, v)
	return v
}

func (c *mockCounter) Add(v int64) int64 {
	return atomic.AddInt64(&c.value, v)
}

// MockAPI implements api.API interface
type MockAPI struct {
	mu sync.Mutex

	NodeInfo         *api.NodeInfo
	GetNodeErr       error
	UserList         *[]api.UserInfo
	GetUserErr       error
	NodeRules        *[]api.DetectRule
	GetRuleErr       error
	ClientInfo       api.ClientInfo
	ReportStatusErr  error
	ReportTrafficErr error
	ReportOnlineErr  error
	ReportIllegalErr error

	ReportedStatus      []*api.NodeStatus
	ReportedTraffic     []*[]api.UserTraffic
	ReportedOnlineUsers []*[]api.OnlineUser
	ReportedIllegal     []*[]api.DetectResult
}

func (m *MockAPI) GetNodeInfo() (*api.NodeInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetNodeErr != nil {
		return nil, m.GetNodeErr
	}
	return m.NodeInfo, nil
}

func (m *MockAPI) GetUserList() (*[]api.UserInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetUserErr != nil {
		return nil, m.GetUserErr
	}
	return m.UserList, nil
}

func (m *MockAPI) ReportNodeStatus(nodeStatus *api.NodeStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ReportStatusErr != nil {
		return m.ReportStatusErr
	}
	m.ReportedStatus = append(m.ReportedStatus, nodeStatus)
	return nil
}

func (m *MockAPI) ReportNodeOnlineUsers(onlineUser *[]api.OnlineUser) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ReportOnlineErr != nil {
		return m.ReportOnlineErr
	}
	if onlineUser != nil {
		m.ReportedOnlineUsers = append(m.ReportedOnlineUsers, onlineUser)
	}
	return nil
}

func (m *MockAPI) ReportUserTraffic(userTraffic *[]api.UserTraffic) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ReportTrafficErr != nil {
		return m.ReportTrafficErr
	}
	if userTraffic != nil {
		m.ReportedTraffic = append(m.ReportedTraffic, userTraffic)
	}
	return nil
}

func (m *MockAPI) Describe() api.ClientInfo {
	return m.ClientInfo
}

func (m *MockAPI) GetNodeRule() (*[]api.DetectRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetRuleErr != nil {
		return nil, m.GetRuleErr
	}
	return m.NodeRules, nil
}

func (m *MockAPI) ReportIllegal(detectResultList *[]api.DetectResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ReportIllegalErr != nil {
		return m.ReportIllegalErr
	}
	if detectResultList != nil {
		m.ReportedIllegal = append(m.ReportedIllegal, detectResultList)
	}
	return nil
}

// MockNodeController implements NodeController interface
type MockNodeController struct {
	mu sync.Mutex

	NodeInfo *api.NodeInfo
	UserList *[]api.UserInfo
	Tag      string

	RebuildNodeCalls    int
	LastRebuiltNodeInfo *api.NodeInfo
	RebuildErr          error

	SyncUsersCalls  int
	LastSyncDeleted []api.UserInfo
	LastSyncAdded   []api.UserInfo
	SyncUsersErr    error

	UpdateRuleCalls         int
	LastUpdatedRules        []api.DetectRule
	AddLimiterCalls         int
	UpdateLimiterCalls      int
	LastUpdatedLimiterUsers []api.UserInfo
	DeleteLimiterCalls      int
}

func (m *MockNodeController) GetNodeInfo() *api.NodeInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.NodeInfo
}

func (m *MockNodeController) SetNodeInfo(ni *api.NodeInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.NodeInfo = ni
}

func (m *MockNodeController) GetUserList() *[]api.UserInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.UserList
}

func (m *MockNodeController) SetUserList(ul *[]api.UserInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UserList = ul
}

func (m *MockNodeController) GetTag() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Tag
}

func (m *MockNodeController) SetTag(tag string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Tag = tag
}

func (m *MockNodeController) RebuildNode(newNodeInfo *api.NodeInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RebuildNodeCalls++
	m.LastRebuiltNodeInfo = newNodeInfo
	if m.RebuildErr != nil {
		return m.RebuildErr
	}
	m.NodeInfo = newNodeInfo
	return nil
}

func (m *MockNodeController) SyncUsers(deleted []api.UserInfo, added []api.UserInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SyncUsersCalls++
	m.LastSyncDeleted = deleted
	m.LastSyncAdded = added
	return m.SyncUsersErr
}

func (m *MockNodeController) UpdateRule(tag string, newRuleList []api.DetectRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpdateRuleCalls++
	m.LastUpdatedRules = newRuleList
	return nil
}

func (m *MockNodeController) AddInboundLimiter(tag string, nodeSpeedLimit uint64, userList *[]api.UserInfo, globalDeviceLimitConfig *limiter.GlobalDeviceLimitConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AddLimiterCalls++
	return nil
}

func (m *MockNodeController) UpdateInboundLimiter(tag string, updatedUserList *[]api.UserInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpdateLimiterCalls++
	if updatedUserList != nil {
		m.LastUpdatedLimiterUsers = append(m.LastUpdatedLimiterUsers, *updatedUserList...)
	}
	return nil
}

func (m *MockNodeController) DeleteInboundLimiter(tag string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteLimiterCalls++
	return nil
}

// MockMetricsProvider implements MetricsProvider interface
type MockMetricsProvider struct {
	mu sync.Mutex

	TrafficData map[string]struct {
		Up   int64
		Down int64
	}
	Counters map[string]*mockCounter

	OnlineUsers   *[]api.OnlineUser
	GetOnlineErr  error
	DetectResults *[]api.DetectResult
	GetDetectErr  error

	ResetCalled bool
}

func (m *MockMetricsProvider) GetTraffic(email string) (up int64, down int64, upCounter stats.Counter, downCounter stats.Counter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.TrafficData != nil {
		if data, ok := m.TrafficData[email]; ok {
			up = data.Up
			down = data.Down
		}
	}
	if m.Counters == nil {
		m.Counters = make(map[string]*mockCounter)
	}
	uCnt, ok := m.Counters[email+">>>up"]
	if !ok {
		uCnt = &mockCounter{value: up}
		m.Counters[email+">>>up"] = uCnt
	}
	dCnt, ok := m.Counters[email+">>>down"]
	if !ok {
		dCnt = &mockCounter{value: down}
		m.Counters[email+">>>down"] = dCnt
	}
	return up, down, uCnt, dCnt
}

func (m *MockMetricsProvider) ResetTraffic(upCounterList *[]stats.Counter, downCounterList *[]stats.Counter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResetCalled = true
	if upCounterList != nil {
		for _, c := range *upCounterList {
			if c != nil {
				c.Set(0)
			}
		}
	}
	if downCounterList != nil {
		for _, c := range *downCounterList {
			if c != nil {
				c.Set(0)
			}
		}
	}
}

func (m *MockMetricsProvider) GetOnlineDevice(tag string) (*[]api.OnlineUser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetOnlineErr != nil {
		return nil, m.GetOnlineErr
	}
	return m.OnlineUsers, nil
}

func (m *MockMetricsProvider) GetDetectResult(tag string) (*[]api.DetectResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.GetDetectErr != nil {
		return nil, m.GetDetectErr
	}
	return m.DetectResults, nil
}

// MockSystemStatusProvider implements SystemStatusProvider interface
type MockSystemStatusProvider struct {
	CPU    float64
	Mem    float64
	Disk   float64
	Uptime uint64
	Err    error
}

func (m *MockSystemStatusProvider) GetSystemInfo() (float64, float64, float64, uint64, error) {
	return m.CPU, m.Mem, m.Disk, m.Uptime, m.Err
}
