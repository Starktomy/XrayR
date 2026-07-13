package controller

import (
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/features/stats"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/app/mydispatcher"
	"github.com/Starktomy/XrayR/service/monitor"
	"github.com/Starktomy/XrayR/service/nodebuilder"
)

var (
	_ monitor.NodeController  = (*Controller)(nil)
	_ monitor.MetricsProvider = (*Controller)(nil)
)

// Option defines a functional option for configuring a Controller.
type Option func(*Controller)

// WithBuilder sets a custom nodebuilder.Builder for the Controller.
func WithBuilder(b nodebuilder.Builder) Option {
	return func(c *Controller) {
		c.SetBuilder(b)
	}
}

// WithCertResolver sets a custom nodebuilder.CertResolver for the Controller and rebuilds the nodebuilder.Builder.
func WithCertResolver(resolver nodebuilder.CertResolver) Option {
	return func(c *Controller) {
		c.SetCertResolver(resolver)
	}
}

// WithSystemStatusProvider sets a custom monitor.SystemStatusProvider for the Controller.
func WithSystemStatusProvider(sysStatus monitor.SystemStatusProvider) Option {
	return func(c *Controller) {
		c.SetSystemStatusProvider(sysStatus)
	}
}

// WithMonitor sets a custom monitor.Monitor instance for the Controller.
func WithMonitor(m *monitor.Monitor) Option {
	return func(c *Controller) {
		c.SetMonitor(m)
	}
}

type Controller struct {
	server       *core.Instance
	config       *Config
	clientInfo   api.ClientInfo
	apiClient    api.API
	nodeInfo     *api.NodeInfo
	Tag          string
	userList     *[]api.UserInfo
	panelType    string
	builder      nodebuilder.Builder
	certResolver nodebuilder.CertResolver
	sysStatus    monitor.SystemStatusProvider
	ibm          inbound.Manager
	obm          outbound.Manager
	stm          stats.Manager
	pm           policy.Manager
	dispatcher   *mydispatcher.DefaultDispatcher
	startAt      time.Time
	logger       *log.Entry

	nodeInfoMu sync.RWMutex
	userListMu sync.RWMutex
	tagMu      sync.RWMutex

	monitor *monitor.Monitor
}

func (c *Controller) GetNodeInfo() *api.NodeInfo {
	c.nodeInfoMu.RLock()
	defer c.nodeInfoMu.RUnlock()
	return c.nodeInfo
}

func (c *Controller) SetNodeInfo(ni *api.NodeInfo) {
	c.nodeInfoMu.Lock()
	defer c.nodeInfoMu.Unlock()
	c.nodeInfo = ni
}

func (c *Controller) GetUserList() *[]api.UserInfo {
	c.userListMu.RLock()
	defer c.userListMu.RUnlock()
	return c.userList
}

func (c *Controller) SetUserList(ul *[]api.UserInfo) {
	c.userListMu.Lock()
	defer c.userListMu.Unlock()
	c.userList = ul
}

func (c *Controller) GetTag() string {
	c.tagMu.RLock()
	defer c.tagMu.RUnlock()
	return c.Tag
}

func (c *Controller) SetTag(tag string) {
	c.tagMu.Lock()
	defer c.tagMu.Unlock()
	c.Tag = tag
}

func (c *Controller) SetBuilder(b nodebuilder.Builder) {
	if b != nil {
		c.builder = b
	}
}

func (c *Controller) SetCertResolver(resolver nodebuilder.CertResolver) {
	c.certResolver = resolver
	c.builder = nodebuilder.New(resolver)
}

func (c *Controller) SetSystemStatusProvider(sysStatus monitor.SystemStatusProvider) {
	c.sysStatus = sysStatus
}

func (c *Controller) SetMonitor(m *monitor.Monitor) {
	c.monitor = m
}

func (c *Controller) GetBuilder() nodebuilder.Builder {
	return c.builder
}

func (c *Controller) GetCertResolver() nodebuilder.CertResolver {
	return c.certResolver
}

func (c *Controller) GetSystemStatusProvider() monitor.SystemStatusProvider {
	return c.sysStatus
}

func (c *Controller) GetMonitor() *monitor.Monitor {
	return c.monitor
}

// New returns a Controller service with default parameters.
// New constructs a Controller bound to an xray server.
// Optional functional options (opts) allow injecting custom node builders, cert resolvers,
// system status providers, or monitor instances.
func New(server *core.Instance, api api.API, config *Config, panelType string, opts ...Option) *Controller {
	var logger *log.Entry
	if api != nil {
		desc := api.Describe()
		logger = log.NewEntry(log.StandardLogger()).WithFields(log.Fields{
			"Host": desc.APIHost,
			"Type": desc.NodeType,
			"ID":   desc.NodeID,
		})
	} else {
		logger = log.NewEntry(log.StandardLogger())
	}
	controller := &Controller{
		server:    server,
		config:    config,
		apiClient: api,
		panelType: panelType,
		builder:   nodebuilder.New(nil),
		startAt:   time.Now(),
		logger:    logger,
	}

	if server != nil {
		if rawIbm := server.GetFeature(inbound.ManagerType()); rawIbm != nil {
			controller.ibm, _ = rawIbm.(inbound.Manager)
		}
		if rawObm := server.GetFeature(outbound.ManagerType()); rawObm != nil {
			controller.obm, _ = rawObm.(outbound.Manager)
		}
		if rawStm := server.GetFeature(stats.ManagerType()); rawStm != nil {
			controller.stm, _ = rawStm.(stats.Manager)
		}
		if rawPm := server.GetFeature(policy.ManagerType()); rawPm != nil {
			controller.pm, _ = rawPm.(policy.Manager)
		}
		if rawDisp := server.GetFeature(mydispatcher.Type()); rawDisp != nil {
			controller.dispatcher, _ = rawDisp.(*mydispatcher.DefaultDispatcher)
		}
	}

	for _, opt := range opts {
		if opt != nil {
			opt(controller)
		}
	}

	return controller
}

// Start implements the Start() function of the service interface
func (c *Controller) Start() error {
	c.clientInfo = c.apiClient.Describe()
	// First fetch Node Info
	newNodeInfo, err := c.apiClient.GetNodeInfo()
	if err != nil {
		return err
	}
	if newNodeInfo.Port == 0 {
		return errors.New("server port must > 0")
	}
	c.SetNodeInfo(newNodeInfo)
	c.SetTag(c.buildNodeTag())

	// Add new tag
	err = c.addNewTag(newNodeInfo)
	if err != nil {
		return fmt.Errorf("add new tag: %w", err)
	}
	// Update user
	userInfo, err := c.apiClient.GetUserList()
	if err != nil {
		return err
	}

	// sync controller userList
	c.SetUserList(userInfo)

	err = c.addNewUser(userInfo, newNodeInfo)
	if err != nil {
		return err
	}

	// Add Limiter
	if err := c.AddInboundLimiter(c.GetTag(), newNodeInfo.SpeedLimit, userInfo, c.config.GlobalDeviceLimitConfig); err != nil {
		c.logger.Print(err)
	}

	// Add Rule Manager
	if !c.config.DisableGetRule {
		if ruleList, err := c.apiClient.GetNodeRule(); err != nil {
			c.logger.Printf("Get rule list filed: %s", err)
		} else if len(*ruleList) > 0 {
			if err := c.UpdateRule(c.GetTag(), *ruleList); err != nil {
				c.logger.Print(err)
			}
		}
	}

	if c.monitor == nil {
		c.monitor = monitor.New(c.config, c.apiClient, c, c, c.sysStatus, c.panelType)
	}
	return c.monitor.Start()
}

// Close implements the Close() function of the service interface
func (c *Controller) Close() error {
	if c.monitor != nil {
		return c.monitor.Close()
	}
	return nil
}

func (c *Controller) RebuildNode(newNodeInfo *api.NodeInfo) error {
	oldTag := c.GetTag()
	if oldTag != "" {
		if err := c.removeOldTag(oldTag); err != nil {
			c.logger.Print(err)
		}
		currNode := c.GetNodeInfo()
		if currNode != nil && currNode.NodeType == "Shadowsocks-Plugin" {
			if err := c.removeOldTag(fmt.Sprintf("dokodemo-door_%s+1", oldTag)); err != nil {
				c.logger.Print(err)
			}
		}
		if err := c.DeleteInboundLimiter(oldTag); err != nil {
			c.logger.Print(err)
		}
	}

	c.SetNodeInfo(newNodeInfo)
	c.SetTag(c.buildNodeTag())

	if err := c.addNewTag(newNodeInfo); err != nil {
		return fmt.Errorf("add new tag: %w", err)
	}

	userList := c.GetUserList()
	if userList != nil && len(*userList) > 0 {
		if err := c.addNewUser(userList, newNodeInfo); err != nil {
			return fmt.Errorf("add new user: %w", err)
		}
		if err := c.AddInboundLimiter(c.GetTag(), newNodeInfo.SpeedLimit, userList, c.config.GlobalDeviceLimitConfig); err != nil {
			c.logger.Print(err)
		}
	}
	return nil
}

func (c *Controller) SyncUsers(deleted []api.UserInfo, added []api.UserInfo) error {
	tag := c.GetTag()
	if len(deleted) > 0 {
		deletedEmail := make([]string, len(deleted))
		for i, u := range deleted {
			deletedEmail[i] = fmt.Sprintf("%s|%s|%d", tag, u.Email, u.UID)
		}
		err := c.removeUsers(deletedEmail, tag)
		if err != nil {
			c.logger.Print(err)
		}
	}
	if len(added) > 0 {
		nodeInfo := c.GetNodeInfo()
		err := c.addNewUser(&added, nodeInfo)
		if err != nil {
			c.logger.Print(err)
		}
		if err := c.UpdateInboundLimiter(tag, &added); err != nil {
			c.logger.Print(err)
		}
	}
	c.logger.Printf("%d user deleted, %d user added", len(deleted), len(added))
	return nil
}

func (c *Controller) removeOldTag(oldTag string) (err error) {
	err = c.removeInbound(oldTag)
	if err != nil {
		return err
	}
	err = c.removeOutbound(oldTag)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) addNewTag(newNodeInfo *api.NodeInfo) (err error) {
	tag := c.GetTag()
	if newNodeInfo.NodeType != "Shadowsocks-Plugin" {
		inboundConfig, err := c.builder.BuildInbound(c.config, newNodeInfo, tag)
		if err != nil {
			return err
		}
		err = c.addInbound(inboundConfig)
		if err != nil {
			return err
		}
		outBoundConfig, err := c.builder.BuildOutbound(c.config, newNodeInfo, tag)
		if err != nil {
			return err
		}
		err = c.addOutbound(outBoundConfig)
		if err != nil {
			return err
		}
	} else {
		return c.addInboundForSSPlugin(*newNodeInfo)
	}
	return nil
}

func (c *Controller) addInboundForSSPlugin(newNodeInfo api.NodeInfo) (err error) {
	tag := c.GetTag()
	fakeNodeInfo := newNodeInfo
	fakeNodeInfo.TransportProtocol = "tcp"
	fakeNodeInfo.EnableTLS = false
	inboundConfig, err := c.builder.BuildInbound(c.config, &fakeNodeInfo, tag)
	if err != nil {
		return err
	}
	err = c.addInbound(inboundConfig)
	if err != nil {
		return err
	}
	outBoundConfig, err := c.builder.BuildOutbound(c.config, &fakeNodeInfo, tag)
	if err != nil {
		return err
	}
	err = c.addOutbound(outBoundConfig)
	if err != nil {
		return err
	}
	inboundConfig, outBoundConfig, err = c.builder.BuildSSPluginDetour(c.config, &newNodeInfo, tag)
	if err != nil {
		return err
	}
	err = c.addInbound(inboundConfig)
	if err != nil {
		return err
	}
	err = c.addOutbound(outBoundConfig)
	if err != nil {
		return err
	}
	return nil
}

func (c *Controller) addNewUser(userInfo *[]api.UserInfo, nodeInfo *api.NodeInfo) (err error) {
	nodeType := nodeInfo.NodeType
	if (nodeInfo.EnableVless || nodeType == "Vless") && nodeType != "Vmess" {
		nodeType = "Vless"
	}
	tag := c.GetTag()
	users, err := c.builder.BuildUser(nodeType, userInfo, tag, nodeInfo.VlessFlow, c.panelType)
	if err != nil {
		return err
	}

	err = c.addUsers(users, tag)
	if err != nil {
		return err
	}
	c.logger.Printf("Added %d new users", len(*userInfo))
	return nil
}

func (c *Controller) buildUserTag(user *api.UserInfo) string {
	return fmt.Sprintf("%s|%s|%d", c.GetTag(), user.Email, user.UID)
}

func compareUserList(old, new *[]api.UserInfo) (deleted, added []api.UserInfo) {
	if old == nil || new == nil {
		return nil, nil
	}

	oldByUID := make(map[int]api.UserInfo, len(*old))
	for _, u := range *old {
		oldByUID[u.UID] = u
	}

	for _, u := range *new {
		prev, ok := oldByUID[u.UID]
		if !ok {
			added = append(added, u)
			continue
		}
		if prev != u {
			deleted = append(deleted, prev)
			added = append(added, u)
		}
		delete(oldByUID, u.UID)
	}

	for _, u := range oldByUID {
		deleted = append(deleted, u)
	}
	sortUserListForDiff(deleted)
	sortUserListForDiff(added)
	return deleted, added
}

func sortUserListForDiff(s []api.UserInfo) {
	if len(s) < 2 {
		return
	}
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].UID > s[j].UID; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

func (c *Controller) buildNodeTag() string {
	ni := c.GetNodeInfo()
	return fmt.Sprintf("%s_%s_%d", ni.NodeType, c.config.ListenIP, ni.Port)
}
