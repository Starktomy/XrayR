package nodebuilder

import (
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/core"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/common/mylego"
	"github.com/Starktomy/XrayR/config"
)

// Builder defines the interface for building inbound configs, outbound configs, and users.
type Builder interface {
	BuildInbound(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (*core.InboundHandlerConfig, error)
	BuildOutbound(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (*core.OutboundHandlerConfig, error)
	BuildUser(nodeType string, userInfo *[]api.UserInfo, tag string, vlessFlow string, panelType string) ([]*protocol.User, error)
	BuildSSPluginDetour(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (inbound *core.InboundHandlerConfig, outbound *core.OutboundHandlerConfig, err error)
}

// CertResolver defines the interface for resolving TLS certificate and key file paths.
type CertResolver interface {
	GetCertFile(certConfig *mylego.CertConfig) (certFile string, keyFile string, err error)
}

// NodeBuilder implements Builder interface.
type NodeBuilder struct {
	certResolver CertResolver
}

var _ Builder = (*NodeBuilder)(nil)

// New creates a new NodeBuilder instance with the given CertResolver.
// If resolver is nil, DefaultCertResolver is used.
func New(resolver CertResolver) *NodeBuilder {
	if resolver == nil {
		resolver = NewDefaultCertResolver()
	}
	return &NodeBuilder{
		certResolver: resolver,
	}
}

var defaultBuilder = New(nil)

// Package-level helpers forwarding to defaultBuilder instance.
func BuildInbound(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (*core.InboundHandlerConfig, error) {
	return defaultBuilder.BuildInbound(cfg, nodeInfo, tag)
}

func BuildOutbound(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (*core.OutboundHandlerConfig, error) {
	return defaultBuilder.BuildOutbound(cfg, nodeInfo, tag)
}

func BuildUser(nodeType string, userInfo *[]api.UserInfo, tag string, vlessFlow string, panelType string) ([]*protocol.User, error) {
	return defaultBuilder.BuildUser(nodeType, userInfo, tag, vlessFlow, panelType)
}

func BuildSSPluginDetour(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (*core.InboundHandlerConfig, *core.OutboundHandlerConfig, error) {
	return defaultBuilder.BuildSSPluginDetour(cfg, nodeInfo, tag)
}
