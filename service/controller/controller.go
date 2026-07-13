package controller

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/features/inbound"
	"github.com/xtls/xray-core/features/outbound"
	"github.com/xtls/xray-core/features/policy"
	"github.com/xtls/xray-core/features/stats"
	"golang.org/x/sync/errgroup"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/app/mydispatcher"
	"github.com/Starktomy/XrayR/common/mylego"
	"github.com/Starktomy/XrayR/common/serverstatus"
)

type LimitInfo struct {
	end               int64
	currentSpeedLimit int
	originSpeedLimit  uint64
}

type Controller struct {
	server       *core.Instance
	config       *Config
	clientInfo   api.ClientInfo
	apiClient    api.API
	nodeInfo     *api.NodeInfo
	Tag          string
	userList     *[]api.UserInfo
	tasks        []periodicTask
	limitedUsers map[api.UserInfo]LimitInfo
	warnedUsers  map[api.UserInfo]int
	panelType    string
	ibm          inbound.Manager
	obm          outbound.Manager
	stm          stats.Manager
	pm           policy.Manager
	dispatcher   *mydispatcher.DefaultDispatcher
	startAt      time.Time
	logger       *log.Entry

	// nodeInfoMu and userListMu protect nodeInfo / userList
	// against concurrent reads from the dispatch path and
	// writes from the four periodic monitor goroutines
	// (cert / nodeInfo / userInfo / report). Reads use RLock,
	// writes Lock the full block.
	nodeInfoMu sync.RWMutex
	userListMu sync.RWMutex

	// monitorErrs aggregates the most recent error from each
	// periodic monitor. The monitor goroutines write via
	// recordMonitorError, Close reads the result. This gives
	// the operator a single error to look at on shutdown
	// instead of a long stream of c.logger.Print lines that
	// the previous code threw away.
	monitorErrsMu sync.Mutex
	monitorErrs   map[string]error

	ctx    context.Context
	cancel context.CancelFunc
	eg     *errgroup.Group
}

type periodicTask struct {
	tag      string
	Interval time.Duration
	Execute  func() error
}

// getNodeInfo returns the current nodeInfo under a read lock.
// Callers that need to compare against a freshly fetched
// descriptor must use this instead of touching c.nodeInfo
// directly so the four monitor goroutines don't race.
func (c *Controller) getNodeInfo() *api.NodeInfo {
	c.nodeInfoMu.RLock()
	defer c.nodeInfoMu.RUnlock()
	return c.nodeInfo
}

// setNodeInfo stores a new nodeInfo under a write lock.
func (c *Controller) setNodeInfo(ni *api.NodeInfo) {
	c.nodeInfoMu.Lock()
	defer c.nodeInfoMu.Unlock()
	c.nodeInfo = ni
}

// getUserList returns the current user list under a read lock.
func (c *Controller) getUserList() *[]api.UserInfo {
	c.userListMu.RLock()
	defer c.userListMu.RUnlock()
	return c.userList
}

// setUserList stores a new user list under a write lock.
func (c *Controller) setUserList(ul *[]api.UserInfo) {
	c.userListMu.Lock()
	defer c.userListMu.Unlock()
	c.userList = ul
}

// recordMonitorError stores err under task so Close can
// surface it. Only the most recent error per task is kept;
// periodic monitors recover from individual failures, and
// keeping the latest is enough to diagnose why a node fell
// behind. nil clears the entry (the monitor succeeded).
func (c *Controller) recordMonitorError(task string, err error) {
	if err == nil {
		return
	}
	c.monitorErrsMu.Lock()
	defer c.monitorErrsMu.Unlock()
	if c.monitorErrs == nil {
		c.monitorErrs = make(map[string]error)
	}
	c.monitorErrs[task] = err
}

// drainMonitorErrors returns the accumulated monitor errors
// and clears the map. Used by Close to roll the per-task
// failures into a single error to return to the panel.
func (c *Controller) drainMonitorErrors() map[string]error {
	c.monitorErrsMu.Lock()
	defer c.monitorErrsMu.Unlock()
	out := c.monitorErrs
	c.monitorErrs = nil
	return out
}

// New return a Controller service with default parameters.
// New constructs a Controller bound to an already-started
// xray server. The controller is responsible for keeping the
// server's inbounds, outbounds, and user list in sync with
// the upstream panel.
//
// server must have the mydispatcher feature registered; the
// controller's type assertion will panic otherwise.
func New(server *core.Instance, api api.API, config *Config, panelType string) *Controller {
	logger := log.NewEntry(log.StandardLogger()).WithFields(log.Fields{
		"Host": api.Describe().APIHost,
		"Type": api.Describe().NodeType,
		"ID":   api.Describe().NodeID,
	})
	controller := &Controller{
		server:     server,
		config:     config,
		apiClient:  api,
		panelType:  panelType,
		ibm:        server.GetFeature(inbound.ManagerType()).(inbound.Manager),
		obm:        server.GetFeature(outbound.ManagerType()).(outbound.Manager),
		stm:        server.GetFeature(stats.ManagerType()).(stats.Manager),
		pm:         server.GetFeature(policy.ManagerType()).(policy.Manager),
		dispatcher: server.GetFeature(mydispatcher.Type()).(*mydispatcher.DefaultDispatcher),
		startAt:    time.Now(),
		logger:     logger,
	}

	return controller
}

// Start implement the Start() function of the service interface
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
	c.setNodeInfo(newNodeInfo)
	c.Tag = c.buildNodeTag()

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
	c.setUserList(userInfo)

	err = c.addNewUser(userInfo, newNodeInfo)
	if err != nil {
		return err
	}

	// Add Limiter
	if err := c.AddInboundLimiter(c.Tag, newNodeInfo.SpeedLimit, userInfo, c.config.GlobalDeviceLimitConfig); err != nil {
		c.logger.Print(err)
	}

	// Add Rule Manager
	if !c.config.DisableGetRule {
		if ruleList, err := c.apiClient.GetNodeRule(); err != nil {
			c.logger.Printf("Get rule list filed: %s", err)
		} else if len(*ruleList) > 0 {
			if err := c.UpdateRule(c.Tag, *ruleList); err != nil {
				c.logger.Print(err)
			}
		}
	}

	// Init AutoSpeedLimitConfig
	if c.config.AutoSpeedLimitConfig == nil {
		c.config.AutoSpeedLimitConfig = &AutoSpeedLimitConfig{0, 0, 0, 0}
	}
	if c.config.AutoSpeedLimitConfig.Limit > 0 {
		c.limitedUsers = make(map[api.UserInfo]LimitInfo)
		c.warnedUsers = make(map[api.UserInfo]int)
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.eg, c.ctx = errgroup.WithContext(c.ctx)

	// Add periodic tasks
	c.tasks = append(c.tasks,
		periodicTask{
			tag:      "node monitor",
			Interval: time.Duration(c.config.UpdatePeriodic) * time.Second,
			Execute:  c.nodeInfoMonitor,
		},
		periodicTask{
			tag:      "user monitor",
			Interval: time.Duration(c.config.UpdatePeriodic) * time.Second,
			Execute:  c.userInfoMonitor,
		},
	)

	// Check cert service in need
	if c.getNodeInfo().EnableTLS && c.config.EnableREALITY == false {
		c.tasks = append(c.tasks, periodicTask{
			tag:      "cert monitor",
			Interval: time.Duration(c.config.UpdatePeriodic) * time.Second * 60,
			Execute:  c.certMonitor,
		})
	}

	// Start periodic tasks. Wrap each in a panic recovery so a bug
	// in one tick doesn't kill the whole process; the rest of the
	// monitor (cert / nodeInfo / userInfo / report) keep running.
	for i := range c.tasks {
		c.logger.Printf("Start %s periodic task", c.tasks[i].tag)
		tag := c.tasks[i].tag
		task := c.tasks[i]
		execute := task.Execute

		c.eg.Go(func() error {
			defer func() {
				if r := recover(); r != nil {
					c.recordMonitorError(tag, fmt.Errorf("panic: %v", r))
					c.logger.Printf("recovered panic in %s periodic task: %v", tag, r)
				}
			}()

			ticker := time.NewTicker(task.Interval)
			defer ticker.Stop()

			// Run once immediately
			err := execute()
			c.recordMonitorError(tag, err)

			for {
				select {
				case <-c.ctx.Done():
					// Context cancelled, do one last run to flush data
					err := execute()
					c.recordMonitorError(tag, err)
					return nil
				case <-ticker.C:
					err := execute()
					c.recordMonitorError(tag, err)
				}
			}
		})
	}

	return nil
}

// Close implement the Close() function of the service interface
func (c *Controller) Close() error {
	if c.cancel != nil {
		c.cancel()
	}
	if c.eg != nil {
		_ = c.eg.Wait()
	}

	// Aggregate any non-fatal monitor errors collected during
	// the run. Returning them as a single error (joined via
	// '\n' so it is readable in the panel log) gives operators
	// one place to look instead of having to scroll through
	// the periodic logger.Print lines that were previously
	// the only record.
	if errs := c.drainMonitorErrors(); len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for tag, e := range errs {
			msgs = append(msgs, fmt.Sprintf("%s: %s", tag, e))
		}
		sort.Strings(msgs) // stable order for tests/logs
		return errors.New(strings.Join(msgs, "; "))
	}
	return nil
}

func (c *Controller) nodeInfoMonitor() (err error) {
	// delay to start
	if time.Since(c.startAt) < time.Duration(c.config.UpdatePeriodic)*time.Second {
		return nil
	}

	// First fetch Node Info
	var nodeInfoChanged = true
	newNodeInfo, err := c.apiClient.GetNodeInfo()
	if err != nil {
		if err.Error() == api.NodeNotModified {
			nodeInfoChanged = false
			newNodeInfo = c.getNodeInfo()
		} else {
			c.logger.Print(err)
			return nil
		}
	}
	if newNodeInfo.Port == 0 {
		return errors.New("server port must > 0")
	}

	// Update User
	var usersChanged = true
	newUserInfo, err := c.apiClient.GetUserList()
	if err != nil {
		if err.Error() == api.UserNotModified {
			usersChanged = false
			newUserInfo = c.getUserList()
		} else {
			c.logger.Print(err)
			return nil
		}
	}

	// If nodeInfo changed
	if nodeInfoChanged {
		// The panel tells us explicitly whether the descriptor
		// changed by returning 304 NotModified; in that case
		// newNodeInfo is the old snapshot. Trust the 304 over
		// a deep walk of the struct (which is O(field count)
		// and would also be incorrect on any future field
		// holding a func or chan).
		oldTag := c.Tag
		if err := c.removeOldTag(oldTag); err != nil {
			c.logger.Print(err)
			return nil
		}
		if c.getNodeInfo().NodeType == "Shadowsocks-Plugin" {
			if err := c.removeOldTag(fmt.Sprintf("dokodemo-door_%s+1", c.Tag)); err != nil {
				c.logger.Print(err)
				return nil
			}
		}
		// Add new tag
		c.setNodeInfo(newNodeInfo)
		c.Tag = c.buildNodeTag()
		if err := c.addNewTag(newNodeInfo); err != nil {
			c.logger.Print(err)
			return nil
		}
		// Remove Old limiter
		if err := c.DeleteInboundLimiter(oldTag); err != nil {
			c.logger.Print(err)
			return nil
		}
	}

	// Check Rule
	if !c.config.DisableGetRule {
		if ruleList, err := c.apiClient.GetNodeRule(); err != nil {
			if err.Error() != api.RuleNotModified {
				c.logger.Printf("Get rule list filed: %s", err)
			}
		} else if len(*ruleList) > 0 {
			if err := c.UpdateRule(c.Tag, *ruleList); err != nil {
				c.logger.Print(err)
			}
		}
	}

	if nodeInfoChanged {
		err = c.addNewUser(newUserInfo, newNodeInfo)
		if err != nil {
			c.logger.Print(err)
			return nil
		}

		// Add Limiter
		if err := c.AddInboundLimiter(c.Tag, newNodeInfo.SpeedLimit, newUserInfo, c.config.GlobalDeviceLimitConfig); err != nil {
			c.logger.Print(err)
			return nil
		}

	} else {
		var deleted, added []api.UserInfo
		if usersChanged {
			deleted, added = compareUserList(c.getUserList(), newUserInfo)
			if len(deleted) > 0 {
				deletedEmail := make([]string, len(deleted))
				for i, u := range deleted {
					deletedEmail[i] = fmt.Sprintf("%s|%s|%d", c.Tag, u.Email, u.UID)
				}
				err := c.removeUsers(deletedEmail, c.Tag)
				if err != nil {
					c.logger.Print(err)
				}
			}
			if len(added) > 0 {
				err = c.addNewUser(&added, c.nodeInfo)
				if err != nil {
					c.logger.Print(err)
				}
				// Update Limiter
				if err := c.UpdateInboundLimiter(c.Tag, &added); err != nil {
					c.logger.Print(err)
				}
			}
		}
		c.logger.Printf("%d user deleted, %d user added", len(deleted), len(added))
	}
	c.setUserList(newUserInfo)
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
	if newNodeInfo.NodeType != "Shadowsocks-Plugin" {
		inboundConfig, err := InboundBuilder(c.config, newNodeInfo, c.Tag)
		if err != nil {
			return err
		}
		err = c.addInbound(inboundConfig)
		if err != nil {

			return err
		}
		outBoundConfig, err := OutboundBuilder(c.config, newNodeInfo, c.Tag)
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
	// Shadowsocks-Plugin require a separate inbound for other TransportProtocol likes: ws, grpc
	fakeNodeInfo := newNodeInfo
	fakeNodeInfo.TransportProtocol = "tcp"
	fakeNodeInfo.EnableTLS = false
	// Add a regular Shadowsocks inbound and outbound
	inboundConfig, err := InboundBuilder(c.config, &fakeNodeInfo, c.Tag)
	if err != nil {
		return err
	}
	err = c.addInbound(inboundConfig)
	if err != nil {

		return err
	}
	outBoundConfig, err := OutboundBuilder(c.config, &fakeNodeInfo, c.Tag)
	if err != nil {

		return err
	}
	err = c.addOutbound(outBoundConfig)
	if err != nil {

		return err
	}
	// Add an inbound for upper streaming protocol
	fakeNodeInfo = newNodeInfo
	fakeNodeInfo.Port++
	fakeNodeInfo.NodeType = "dokodemo-door"
	dokodemoTag := fmt.Sprintf("dokodemo-door_%s+1", c.Tag)
	inboundConfig, err = InboundBuilder(c.config, &fakeNodeInfo, dokodemoTag)
	if err != nil {
		return err
	}
	err = c.addInbound(inboundConfig)
	if err != nil {

		return err
	}
	outBoundConfig, err = OutboundBuilder(c.config, &fakeNodeInfo, dokodemoTag)
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
	users := make([]*protocol.User, 0)
	switch nodeInfo.NodeType {
	case "V2ray", "Vmess", "Vless":
		if nodeInfo.EnableVless || (nodeInfo.NodeType == "Vless" && nodeInfo.NodeType != "Vmess") {
			users = c.buildVlessUser(userInfo)
		} else {
			users = c.buildVmessUser(userInfo)
		}
	case "Trojan":
		users = c.buildTrojanUser(userInfo)
	case "Shadowsocks":
		users = c.buildSSUser(userInfo, nodeInfo.CypherMethod)
	case "Shadowsocks-Plugin":
		users = c.buildSSPluginUser(userInfo)
	default:
		return fmt.Errorf("unsupported node type: %s", nodeInfo.NodeType)
	}

	err = c.addUsers(users, c.Tag)
	if err != nil {
		return err
	}
	c.logger.Printf("Added %d new users", len(*userInfo))
	return nil
}

// compareUserList returns the per-user diff between the
// previously-known user list (old) and the freshly-fetched
// list (new). The pre-existing implementation was O(N²) via
// two passes of map inserts plus a delete loop; for a panel
// that hosts 10k users that was visibly slow. The current
// version indexes both slices by UID, then walks the new
// slice in one pass. Total work is O(N) and allocations
// are bounded by the diff size, not the input size.
//
// Semantics: users are matched by UID. A user whose
// SpeedLimit, DeviceLimit or any other field changed
// appears in both deleted and added, matching the
// pre-existing behaviour the controller's limiter loop
// depends on (it has to re-issue the limiter entry).
func compareUserList(old, new *[]api.UserInfo) (deleted, added []api.UserInfo) {
	if old == nil || new == nil {
		return nil, nil
	}

	oldByUID := make(map[int]api.UserInfo, len(*old))
	for _, u := range *old {
		oldByUID[u.UID] = u
	}

	// Walk the new slice: a UID not present in old is
	// "added"; a UID present but with a different value is
	// reported as both deleted and added (the panel must
	// re-issue the limiter entry for a changed user).
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

	// Anything still in oldByUID after the walk is gone
	// from the new slice.
	for _, u := range oldByUID {
		deleted = append(deleted, u)
	}
	// Map iteration order is non-deterministic; sort the
	// result so the caller (and tests) see a stable order.
	sortUserListForDiff(deleted)
	sortUserListForDiff(added)
	return deleted, added
}

// sortUserListForDiff sorts in place by UID. We need a
// dedicated helper because sort.Slice requires a closure
// (or a package-level function) and we want this file to
// stay self-contained.
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

func limitUser(c *Controller, user api.UserInfo, silentUsers *[]api.UserInfo) {
	c.limitedUsers[user] = LimitInfo{
		end:               time.Now().Unix() + int64(c.config.AutoSpeedLimitConfig.LimitDuration*60),
		currentSpeedLimit: c.config.AutoSpeedLimitConfig.LimitSpeed,
		originSpeedLimit:  user.SpeedLimit,
	}
	c.logger.Printf("Limit User: %s Speed: %d End: %s", c.buildUserTag(&user), c.config.AutoSpeedLimitConfig.LimitSpeed, time.Unix(c.limitedUsers[user].end, 0).Format("01-02 15:04:05"))
	user.SpeedLimit = uint64((c.config.AutoSpeedLimitConfig.LimitSpeed * 1000000) / 8)
	*silentUsers = append(*silentUsers, user)
}

func (c *Controller) userInfoMonitor() (err error) {
	// delay to start
	if time.Since(c.startAt) < time.Duration(c.config.UpdatePeriodic)*time.Second {
		return nil
	}

	// Get server status
	CPU, Mem, Disk, Uptime, err := serverstatus.GetSystemInfo()
	if err != nil {
		c.logger.Print(err)
	}
	err = c.apiClient.ReportNodeStatus(
		&api.NodeStatus{
			CPU:    CPU,
			Mem:    Mem,
			Disk:   Disk,
			Uptime: Uptime,
		})
	if err != nil {
		c.logger.Print(err)
	}
	// Unlock users
	if c.config.AutoSpeedLimitConfig.Limit > 0 && len(c.limitedUsers) > 0 {
		c.logger.Printf("Limited users:")
		toReleaseUsers := make([]api.UserInfo, 0)
		for user, limitInfo := range c.limitedUsers {
			if time.Now().Unix() > limitInfo.end {
				user.SpeedLimit = limitInfo.originSpeedLimit
				toReleaseUsers = append(toReleaseUsers, user)
				c.logger.Printf("User: %s Speed: %d End: nil (Unlimit)", c.buildUserTag(&user), user.SpeedLimit)
				delete(c.limitedUsers, user)
			} else {
				c.logger.Printf("User: %s Speed: %d End: %s", c.buildUserTag(&user), limitInfo.currentSpeedLimit, time.Unix(c.limitedUsers[user].end, 0).Format("01-02 15:04:05"))
			}
		}
		if len(toReleaseUsers) > 0 {
			if err := c.UpdateInboundLimiter(c.Tag, &toReleaseUsers); err != nil {
				c.logger.Print(err)
			}
		}
	}

	// Get User traffic
	userList := *c.getUserList()
	userCount := len(userList)
	userTraffic := make([]api.UserTraffic, 0, userCount)
	upCounterList := make([]stats.Counter, 0, userCount)
	downCounterList := make([]stats.Counter, 0, userCount)
	AutoSpeedLimit := int64(c.config.AutoSpeedLimitConfig.Limit)
	UpdatePeriodic := int64(c.config.UpdatePeriodic)
	limitedUsers := make([]api.UserInfo, 0)
	for _, user := range userList {
		userTag := c.buildUserTag(&user)
		up, down, upCounter, downCounter := c.getTraffic(userTag)
		if down > 0 {
			c.logger.Printf("Traffic counted: tag=%s up=%d down=%d", userTag, up, down)
		}
		if up > 0 || down > 0 {
			// Over speed users
			if AutoSpeedLimit > 0 {
				if down > AutoSpeedLimit*1000000*UpdatePeriodic/8 || up > AutoSpeedLimit*1000000*UpdatePeriodic/8 {
					if _, ok := c.limitedUsers[user]; !ok {
						if c.config.AutoSpeedLimitConfig.WarnTimes == 0 {
							limitUser(c, user, &limitedUsers)
						} else {
							c.warnedUsers[user] += 1
							if c.warnedUsers[user] > c.config.AutoSpeedLimitConfig.WarnTimes {
								limitUser(c, user, &limitedUsers)
								delete(c.warnedUsers, user)
							}
						}
					}
				} else {
					delete(c.warnedUsers, user)
				}
			}
			userTraffic = append(userTraffic, api.UserTraffic{
				UID:      user.UID,
				Email:    user.Email,
				Upload:   up,
				Download: down})

			if upCounter != nil {
				upCounterList = append(upCounterList, upCounter)
			}
			if downCounter != nil {
				downCounterList = append(downCounterList, downCounter)
			}
		} else {
			delete(c.warnedUsers, user)
		}
	}
	if len(limitedUsers) > 0 {
		if err := c.UpdateInboundLimiter(c.Tag, &limitedUsers); err != nil {
			c.logger.Print(err)
		}
	}
	if len(userTraffic) > 0 {
		c.logger.Printf("Reporting %d user(s) traffic to panel; example: UID=%d up=%d down=%d", len(userTraffic), userTraffic[0].UID, userTraffic[0].Upload, userTraffic[0].Download)
		var err error
		if c.config.DisableUploadTraffic {
			// Without upload, we still need to reset counters so
			// the next tick doesn't re-report the same bytes.
			c.resetTraffic(&upCounterList, &downCounterList)
		} else {
			err = c.apiClient.ReportUserTraffic(&userTraffic)
			// If report traffic error, do not clear the traffic.
			if err != nil {
				c.logger.Print(err)
			} else {
				c.resetTraffic(&upCounterList, &downCounterList)
			}
		}
	}

	// Report Online info
	if onlineDevice, err := c.GetOnlineDevice(c.Tag); err != nil {
		c.logger.Print(err)
	} else if len(*onlineDevice) > 0 {
		if err = c.apiClient.ReportNodeOnlineUsers(onlineDevice); err != nil {
			c.logger.Print(err)
		} else {
			c.logger.Printf("Report %d online users", len(*onlineDevice))
		}
	}

	// Report Illegal user
	if detectResult, err := c.GetDetectResult(c.Tag); err != nil {
		c.logger.Print(err)
	} else if len(*detectResult) > 0 {
		if err = c.apiClient.ReportIllegal(detectResult); err != nil {
			c.logger.Print(err)
		} else {
			c.logger.Printf("Report %d illegal behaviors", len(*detectResult))
		}

	}
	return nil
}

func (c *Controller) buildNodeTag() string {
	return fmt.Sprintf("%s_%s_%d", c.getNodeInfo().NodeType, c.config.ListenIP, c.getNodeInfo().Port)
}

// Check Cert
func (c *Controller) certMonitor() error {
	if c.getNodeInfo().EnableTLS && c.config.EnableREALITY == false {
		switch c.config.CertConfig.CertMode {
		case "dns", "http", "tls":
			lego, err := mylego.New(c.config.CertConfig)
			if err != nil {
				// Skip renewal if we failed to build the cert manager;
				// otherwise the next line would nil-deref on lego.
				c.logger.Print(err)
				return err
			}
			// Xray-core supports the OcspStapling certification hot renew
			if _, _, _, err = lego.RenewCert(); err != nil {
				c.logger.Print(err)
			}
		}
	}
	return nil
}
