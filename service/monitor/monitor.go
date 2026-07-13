package monitor

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/config"
)

type LimitInfo struct {
	end               int64
	currentSpeedLimit int
	originSpeedLimit  uint64
}

type Monitor struct {
	config       *config.Config
	apiClient    api.API
	nodeCtrl     NodeController
	metrics      MetricsProvider
	sysStatus    SystemStatusProvider
	panelType    string
	limitedUsers map[api.UserInfo]LimitInfo
	warnedUsers  map[api.UserInfo]int
	startAt      time.Time
	tasks        []periodicTask

	monitorErrsMu sync.Mutex
	monitorErrs   map[string]error

	ctx    context.Context
	cancel context.CancelFunc
	eg     *errgroup.Group
	logger *log.Entry
}

type periodicTask struct {
	tag      string
	Interval time.Duration
	Execute  func() error
}

func New(cfg *config.Config, apiClient api.API, nodeCtrl NodeController, metrics MetricsProvider, sysStatus SystemStatusProvider, panelType string) *Monitor {
	if sysStatus == nil {
		sysStatus = &DefaultSystemStatusProvider{}
	}

	var logger *log.Entry
	if apiClient != nil {
		desc := apiClient.Describe()
		logger = log.NewEntry(log.StandardLogger()).WithFields(log.Fields{
			"Host": desc.APIHost,
			"Type": desc.NodeType,
			"ID":   desc.NodeID,
		})
	} else {
		logger = log.NewEntry(log.StandardLogger())
	}

	return &Monitor{
		config:    cfg,
		apiClient: apiClient,
		nodeCtrl:  nodeCtrl,
		metrics:   metrics,
		sysStatus: sysStatus,
		panelType: panelType,
		startAt:   time.Now(),
		logger:    logger,
	}
}

func (m *Monitor) recordMonitorError(task string, err error) {
	if err == nil {
		return
	}
	m.monitorErrsMu.Lock()
	defer m.monitorErrsMu.Unlock()
	if m.monitorErrs == nil {
		m.monitorErrs = make(map[string]error)
	}
	m.monitorErrs[task] = err
}

func (m *Monitor) drainMonitorErrors() map[string]error {
	m.monitorErrsMu.Lock()
	defer m.monitorErrsMu.Unlock()
	out := m.monitorErrs
	m.monitorErrs = nil
	return out
}

func (m *Monitor) Start() error {
	if m.config.AutoSpeedLimitConfig == nil {
		m.config.AutoSpeedLimitConfig = &config.AutoSpeedLimitConfig{Limit: 0, WarnTimes: 0, LimitSpeed: 0, LimitDuration: 0}
	}
	if m.config.AutoSpeedLimitConfig.Limit > 0 {
		m.limitedUsers = make(map[api.UserInfo]LimitInfo)
		m.warnedUsers = make(map[api.UserInfo]int)
	}

	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.eg, m.ctx = errgroup.WithContext(m.ctx)

	m.tasks = append(m.tasks,
		periodicTask{
			tag:      "node monitor",
			Interval: time.Duration(m.config.UpdatePeriodic) * time.Second,
			Execute:  m.nodeInfoMonitor,
		},
		periodicTask{
			tag:      "user monitor",
			Interval: time.Duration(m.config.UpdatePeriodic) * time.Second,
			Execute:  m.userInfoMonitor,
		},
	)

	nodeInfo := m.nodeCtrl.GetNodeInfo()
	if nodeInfo != nil && nodeInfo.EnableTLS && !m.config.EnableREALITY {
		m.tasks = append(m.tasks, periodicTask{
			tag:      "cert monitor",
			Interval: time.Duration(m.config.UpdatePeriodic) * time.Second * 60,
			Execute:  m.certMonitor,
		})
	}

	for i := range m.tasks {
		m.logger.Printf("Start %s periodic task", m.tasks[i].tag)
		tag := m.tasks[i].tag
		task := m.tasks[i]
		execute := task.Execute

		m.eg.Go(func() error {
			defer func() {
				if r := recover(); r != nil {
					m.recordMonitorError(tag, fmt.Errorf("panic: %v", r))
					m.logger.Printf("recovered panic in %s periodic task: %v", tag, r)
				}
			}()

			ticker := time.NewTicker(task.Interval)
			defer ticker.Stop()

			// Run once immediately
			err := execute()
			m.recordMonitorError(tag, err)

			for {
				select {
				case <-m.ctx.Done():
					// Context cancelled, do one last run to flush data
					err := execute()
					m.recordMonitorError(tag, err)
					return nil
				case <-ticker.C:
					err := execute()
					m.recordMonitorError(tag, err)
				}
			}
		})
	}

	return nil
}

func (m *Monitor) Close() error {
	if m.cancel != nil {
		m.cancel()
	}
	if m.eg != nil {
		_ = m.eg.Wait()
	}

	if errs := m.drainMonitorErrors(); len(errs) > 0 {
		msgs := make([]string, 0, len(errs))
		for tag, e := range errs {
			msgs = append(msgs, fmt.Sprintf("%s: %s", tag, e))
		}
		sort.Strings(msgs)
		return errors.New(strings.Join(msgs, "; "))
	}
	return nil
}
