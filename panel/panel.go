package panel

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"

	"dario.cat/mergo"
	"github.com/r3labs/diff/v2"
	log "github.com/sirupsen/logrus"
	"github.com/xtls/xray-core/app/dispatcher"
	"github.com/xtls/xray-core/app/proxyman"
	"github.com/xtls/xray-core/app/stats"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/api/bunpanel"
	"github.com/Starktomy/XrayR/api/gov2panel"
	"github.com/Starktomy/XrayR/api/newV2board"
	"github.com/Starktomy/XrayR/api/pmpanel"
	"github.com/Starktomy/XrayR/api/proxypanel"
	"github.com/Starktomy/XrayR/api/sspanel"
	"github.com/Starktomy/XrayR/api/v2raysocks"
	"github.com/Starktomy/XrayR/app/mydispatcher"
	_ "github.com/Starktomy/XrayR/cmd/distro/all"
	"github.com/Starktomy/XrayR/service"
	"github.com/Starktomy/XrayR/service/controller"
)

// Panel Structure
type Panel struct {
	access      sync.Mutex
	panelConfig *Config
	Server      *core.Instance
	Service     []service.Service
	Running     bool
}

// New constructs a Panel with the given configuration but
// does not yet start the underlying xray instance or any
// node controllers. Call Start to bring the panel up.
func New(panelConfig *Config) *Panel {
	p := &Panel{panelConfig: panelConfig}
	return p
}

func (p *Panel) loadCore(panelConfig *Config) (*core.Instance, error) {
	// Log Config
	coreLogConfig := &conf.LogConfig{}
	logConfig := getDefaultLogConfig()
	if panelConfig.LogConfig != nil {
		if _, err := diff.Merge(logConfig, panelConfig.LogConfig, logConfig); err != nil {
			return nil, fmt.Errorf("merge log config: %w", err)
		}
	}
	coreLogConfig.LogLevel = logConfig.Level
	coreLogConfig.AccessLog = logConfig.AccessPath
	coreLogConfig.ErrorLog = logConfig.ErrorPath

	// DNS config
	coreDnsConfig := &conf.DNSConfig{}
	if panelConfig.DnsConfigPath != "" {
		data, err := os.ReadFile(panelConfig.DnsConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read DNS config %s: %w", panelConfig.DnsConfigPath, err)
		}
		if err = json.Unmarshal(data, coreDnsConfig); err != nil {
			return nil, fmt.Errorf("unmarshal DNS config %s: %w", panelConfig.DnsConfigPath, err)
		}
	}

	dnsConfig, err := coreDnsConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build DNS config (see https://xtls.github.io/config/dns.html): %w", err)
	}

	// Routing config
	coreRouterConfig := &conf.RouterConfig{}
	if panelConfig.RouteConfigPath != "" {
		data, err := os.ReadFile(panelConfig.RouteConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read Routing config %s: %w", panelConfig.RouteConfigPath, err)
		}
		if err = json.Unmarshal(data, coreRouterConfig); err != nil {
			return nil, fmt.Errorf("unmarshal Routing config %s: %w", panelConfig.RouteConfigPath, err)
		}
	}
	routeConfig, err := coreRouterConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build Routing config (see https://xtls.github.io/config/routing.html): %w", err)
	}
	// Custom Inbound config
	var coreCustomInboundConfig []conf.InboundDetourConfig
	if panelConfig.InboundConfigPath != "" {
		data, err := os.ReadFile(panelConfig.InboundConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read Custom Inbound config %s: %w", panelConfig.InboundConfigPath, err)
		}
		if err = json.Unmarshal(data, &coreCustomInboundConfig); err != nil {
			return nil, fmt.Errorf("unmarshal Custom Inbound config %s: %w", panelConfig.InboundConfigPath, err)
		}
	}
	var inBoundConfig []*core.InboundHandlerConfig
	for _, config := range coreCustomInboundConfig {
		oc, err := config.Build()
		if err != nil {
			return nil, fmt.Errorf("build Inbound config (see https://xtls.github.io/config/inbound.html): %w", err)
		}
		inBoundConfig = append(inBoundConfig, oc)
	}
	// Custom Outbound config
	var coreCustomOutboundConfig []conf.OutboundDetourConfig
	if panelConfig.OutboundConfigPath != "" {
		data, err := os.ReadFile(panelConfig.OutboundConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read Custom Outbound config %s: %w", panelConfig.OutboundConfigPath, err)
		}
		if err = json.Unmarshal(data, &coreCustomOutboundConfig); err != nil {
			return nil, fmt.Errorf("unmarshal Custom Outbound config %s: %w", panelConfig.OutboundConfigPath, err)
		}
	}
	var outBoundConfig []*core.OutboundHandlerConfig
	for _, config := range coreCustomOutboundConfig {
		oc, err := config.Build()
		if err != nil {
			return nil, fmt.Errorf("build Outbound config (see https://xtls.github.io/config/outbound.html): %w", err)
		}
		outBoundConfig = append(outBoundConfig, oc)
	}
	// Policy config
	levelPolicyConfig, err := parseConnectionConfig(panelConfig.ConnectionConfig)
	if err != nil {
		return nil, err
	}
	corePolicyConfig := &conf.PolicyConfig{}
	corePolicyConfig.Levels = map[uint32]*conf.Policy{0: levelPolicyConfig}
	policyConfig, err := corePolicyConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build Policy config: %w", err)
	}
	// Build Core Config
	config := &core.Config{
		App: []*serial.TypedMessage{
			serial.ToTypedMessage(coreLogConfig.Build()),
			serial.ToTypedMessage(&dispatcher.Config{}),
			serial.ToTypedMessage(&mydispatcher.Config{}),
			serial.ToTypedMessage(&stats.Config{}),
			serial.ToTypedMessage(&proxyman.InboundConfig{}),
			serial.ToTypedMessage(&proxyman.OutboundConfig{}),
			serial.ToTypedMessage(policyConfig),
			serial.ToTypedMessage(dnsConfig),
			serial.ToTypedMessage(routeConfig),
		},
		Inbound:  inBoundConfig,
		Outbound: outBoundConfig,
	}
	server, err := core.New(config)
	if err != nil {
		return nil, fmt.Errorf("create xray instance: %w", err)
	}

	return server, nil
}

// Start the panel
func (p *Panel) Start() error {
	p.access.Lock()
	defer p.access.Unlock()
	log.Print("Start the panel..")
	// Load Core
	server, err := p.loadCore(p.panelConfig)
	if err != nil {
		return err
	}
	if err := server.Start(); err != nil {
		return fmt.Errorf("start xray instance: %w", err)
	}
	p.Server = server

	// Load Nodes config
	for _, nodeConfig := range p.panelConfig.NodesConfig {
		var apiClient api.API
		switch nodeConfig.PanelType {
		case "SSpanel":
			apiClient = sspanel.New(nodeConfig.ApiConfig)
		case "NewV2board", "V2board":
			apiClient = newV2board.New(nodeConfig.ApiConfig)
		case "PMpanel":
			apiClient = pmpanel.New(nodeConfig.ApiConfig)
		case "Proxypanel":
			apiClient = proxypanel.New(nodeConfig.ApiConfig)
		case "V2RaySocks":
			apiClient = v2raysocks.New(nodeConfig.ApiConfig)
		case "GoV2Panel":
			apiClient = gov2panel.New(nodeConfig.ApiConfig)
		case "BunPanel":
			apiClient = bunpanel.New(nodeConfig.ApiConfig)
		default:
			return fmt.Errorf("unsupported panel type: %s", nodeConfig.PanelType)
		}
		var controllerService service.Service
		// Register controller service
		controllerConfig := getDefaultControllerConfig()
		if nodeConfig.ControllerConfig != nil {
			if err := mergo.Merge(controllerConfig, nodeConfig.ControllerConfig, mergo.WithOverride); err != nil {
				return fmt.Errorf("merge controller config: %w", err)
			}
		}
		controllerService = controller.New(server, apiClient, controllerConfig, nodeConfig.PanelType)
		p.Service = append(p.Service, controllerService)

	}

	// Start all the service
	for _, s := range p.Service {
		if err := s.Start(); err != nil {
			return fmt.Errorf("start %s: %w", reflect.TypeOf(s).String(), err)
		}
	}
	p.Running = true
	return nil
}

// Close the panel
func (p *Panel) Close() {
	p.access.Lock()
	defer p.access.Unlock()
	for _, s := range p.Service {
		if err := s.Close(); err != nil {
			log.Errorf("Close service failed: %s", err)
		}
	}
	p.Service = nil
	p.Server.Close()
	p.Running = false
	return
}

func parseConnectionConfig(c *ConnectionConfig) (*conf.Policy, error) {
	connectionConfig := getDefaultConnectionConfig()
	if c != nil {
		if _, err := diff.Merge(connectionConfig, c, connectionConfig); err != nil {
			return nil, fmt.Errorf("merge ConnectionConfig: %w", err)
		}
	}
	policy := &conf.Policy{
		StatsUserUplink:   true,
		StatsUserDownlink: true,
		Handshake:         &connectionConfig.Handshake,
		ConnectionIdle:    &connectionConfig.ConnIdle,
		UplinkOnly:        &connectionConfig.UplinkOnly,
		DownlinkOnly:      &connectionConfig.DownlinkOnly,
		BufferSize:        &connectionConfig.BufferSize,
	}

	return policy, nil
}
