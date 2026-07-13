package monitor

import (
	"github.com/xtls/xray-core/features/stats"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/common/limiter"
	"github.com/Starktomy/XrayR/common/serverstatus"
)

// NodeController defines the interface required by monitor to manage node and user state.
type NodeController interface {
	GetNodeInfo() *api.NodeInfo
	SetNodeInfo(ni *api.NodeInfo)
	GetUserList() *[]api.UserInfo
	SetUserList(ul *[]api.UserInfo)
	GetTag() string
	SetTag(tag string)
	RebuildNode(newNodeInfo *api.NodeInfo) error
	SyncUsers(deleted []api.UserInfo, added []api.UserInfo) error
	UpdateRule(tag string, newRuleList []api.DetectRule) error
	AddInboundLimiter(tag string, nodeSpeedLimit uint64, userList *[]api.UserInfo, globalDeviceLimitConfig *limiter.GlobalDeviceLimitConfig) error
	UpdateInboundLimiter(tag string, updatedUserList *[]api.UserInfo) error
	DeleteInboundLimiter(tag string) error
}

// MetricsProvider defines the interface required by monitor to collect user traffic and status.
type MetricsProvider interface {
	GetTraffic(email string) (up int64, down int64, upCounter stats.Counter, downCounter stats.Counter)
	ResetTraffic(upCounterList *[]stats.Counter, downCounterList *[]stats.Counter)
	GetOnlineDevice(tag string) (*[]api.OnlineUser, error)
	GetDetectResult(tag string) (*[]api.DetectResult, error)
}

// SystemStatusProvider abstracts OS system info polling.
type SystemStatusProvider interface {
	GetSystemInfo() (cpu float64, mem float64, disk float64, uptime uint64, err error)
}

// DefaultSystemStatusProvider calls serverstatus.GetSystemInfo directly.
type DefaultSystemStatusProvider struct{}

func (d *DefaultSystemStatusProvider) GetSystemInfo() (float64, float64, float64, uint64, error) {
	return serverstatus.GetSystemInfo()
}
