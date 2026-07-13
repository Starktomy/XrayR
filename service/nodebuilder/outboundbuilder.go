package nodebuilder

import (
	"encoding/json"
	"fmt"

	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/config"
)

// BuildOutbound builds a xray-core OutboundHandlerConfig from the per-node nodeInfo and config.
func (b *NodeBuilder) BuildOutbound(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (*core.OutboundHandlerConfig, error) {
	outboundDetourConfig := &conf.OutboundDetourConfig{}
	outboundDetourConfig.Protocol = "freedom"
	outboundDetourConfig.Tag = tag

	// SendThrough setting
	if cfg != nil && cfg.SendIP != "" {
		outboundDetourConfig.SendThrough = &cfg.SendIP
	}

	// Freedom Protocol setting
	var domainStrategy = "Asis"
	if cfg != nil && cfg.EnableDNS {
		if cfg.DNSType != "" {
			domainStrategy = cfg.DNSType
		} else {
			domainStrategy = "UseIP"
		}
	}
	proxySetting := &conf.FreedomConfig{
		DomainStrategy: domainStrategy,
	}
	// Used for Shadowsocks-Plugin
	if nodeInfo.NodeType == "dokodemo-door" {
		proxySetting.Redirect = fmt.Sprintf("127.0.0.1:%d", nodeInfo.Port-1)
	}
	data, err := json.Marshal(proxySetting)
	if err != nil {
		return nil, fmt.Errorf("marshal proxy %s config failed: %w", nodeInfo.NodeType, err)
	}
	setting := json.RawMessage(data)
	outboundDetourConfig.Settings = &setting
	return outboundDetourConfig.Build()
}
