package nodebuilder

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	C "github.com/sagernet/sing/common"
	"github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/config"
)

// BuildInbound builds a xray-core InboundHandlerConfig from the per-node nodeInfo and config.
func (b *NodeBuilder) BuildInbound(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (*core.InboundHandlerConfig, error) {
	inboundDetourConfig := &conf.InboundDetourConfig{}
	// Build Listen IP address
	if nodeInfo.NodeType == "Shadowsocks-Plugin" {
		// Shadowsocks listen in 127.0.0.1 for safety
		inboundDetourConfig.ListenOn = &conf.Address{Address: net.ParseAddress("127.0.0.1")}
	} else if cfg != nil && cfg.ListenIP != "" {
		ipAddress := net.ParseAddress(cfg.ListenIP)
		inboundDetourConfig.ListenOn = &conf.Address{Address: ipAddress}
	}

	// Build Port
	portList := &conf.PortList{
		Range: []conf.PortRange{{From: nodeInfo.Port, To: nodeInfo.Port}},
	}
	inboundDetourConfig.PortList = portList
	// Build Tag
	inboundDetourConfig.Tag = tag
	// SniffingConfig
	sniffingConfig := &conf.SniffingConfig{
		Enabled:      true,
		DestOverride: &conf.StringList{"http", "tls", "quic", "fakedns"},
	}
	if cfg != nil && cfg.DisableSniffing {
		sniffingConfig.Enabled = false
	}
	inboundDetourConfig.SniffingConfig = sniffingConfig

	var (
		protocol      string
		streamSetting *conf.StreamConfig
		setting       json.RawMessage
	)

	var proxySetting any
	// Build Protocol and Protocol setting
	switch nodeInfo.NodeType {
	case "V2ray", "Vmess", "Vless":
		if nodeInfo.EnableVless || (nodeInfo.NodeType == "Vless" && nodeInfo.NodeType != "Vmess") {
			protocol = "vless"
			// Enable fallback
			if cfg != nil && cfg.EnableFallback {
				fallbackConfigs, err := buildVlessFallbacks(cfg.FallBackConfigs)
				if err == nil {
					proxySetting = &conf.VLessInboundConfig{
						Decryption: "none",
						Fallbacks:  fallbackConfigs,
					}
				} else {
					return nil, err
				}
			} else {
				proxySetting = &conf.VLessInboundConfig{
					Decryption: "none",
				}
			}
		} else {
			protocol = "vmess"
			proxySetting = &conf.VMessInboundConfig{}
		}
	case "Trojan":
		protocol = "trojan"
		// Enable fallback
		if cfg != nil && cfg.EnableFallback {
			fallbackConfigs, err := buildTrojanFallbacks(cfg.FallBackConfigs)
			if err == nil {
				proxySetting = &conf.TrojanServerConfig{
					Fallbacks: fallbackConfigs,
				}
			} else {
				return nil, err
			}
		} else {
			proxySetting = &conf.TrojanServerConfig{}
		}
	case "Shadowsocks", "Shadowsocks-Plugin":
		protocol = "shadowsocks"
		cipher := strings.ToLower(nodeInfo.CypherMethod)

		proxySetting = &conf.ShadowsocksServerConfig{
			Cipher:   cipher,
			Password: nodeInfo.ServerKey, // shadowsocks2022 shareKey
		}

		ssProxySetting, _ := proxySetting.(*conf.ShadowsocksServerConfig)
		// shadowsocks must have a random password
		// shadowsocks2022's password == user PSK, thus should a length of string >= 32 and base64 encoder
		buf := make([]byte, 32)
		rand.Read(buf)
		randPasswd := hex.EncodeToString(buf)
		if C.Contains(shadowaead_2022.List, cipher) {
			ssProxySetting.Users = append(ssProxySetting.Users, &conf.ShadowsocksUserConfig{
				Password: base64.StdEncoding.EncodeToString(buf),
			})
		} else {
			ssProxySetting.Password = randPasswd
		}

		ssProxySetting.NetworkList = &conf.NetworkList{"tcp", "udp"}
		ssProxySetting.IVCheck = true
		if cfg != nil && cfg.DisableIVCheck {
			ssProxySetting.IVCheck = false
		}

	case "dokodemo-door":
		protocol = "dokodemo-door"
		proxySetting = struct {
			Host        string   `json:"address"`
			NetworkList []string `json:"network"`
		}{
			Host:        "v1.mux.cool",
			NetworkList: []string{"tcp", "udp"},
		}
	default:
		return nil, fmt.Errorf("unsupported node type: %s, Only support: V2ray, Trojan, Shadowsocks, and Shadowsocks-Plugin", nodeInfo.NodeType)
	}

	setting, err := json.Marshal(proxySetting)
	if err != nil {
		return nil, fmt.Errorf("marshal proxy %s config failed: %w", nodeInfo.NodeType, err)
	}
	inboundDetourConfig.Protocol = protocol
	inboundDetourConfig.Settings = &setting

	// Build streamSettings
	streamSetting = new(conf.StreamConfig)
	transportProtocol := conf.TransportProtocol(nodeInfo.TransportProtocol)
	networkType, err := transportProtocol.Build()
	if err != nil {
		return nil, fmt.Errorf("convert TransportProtocol failed: %w", err)
	}

	switch networkType {
	case "tcp":
		enableProxyProtocol := false
		if cfg != nil {
			enableProxyProtocol = cfg.EnableProxyProtocol
		}
		tcpSetting := &conf.TCPConfig{
			AcceptProxyProtocol: enableProxyProtocol,
			HeaderConfig:        nodeInfo.Header,
		}
		streamSetting.TCPSettings = tcpSetting
	case "websocket":
		enableProxyProtocol := false
		if cfg != nil {
			enableProxyProtocol = cfg.EnableProxyProtocol
		}
		headers := make(map[string]string)
		headers["Host"] = nodeInfo.Host
		wsSettings := &conf.WebSocketConfig{
			AcceptProxyProtocol: enableProxyProtocol,
			Host:                nodeInfo.Host,
			Path:                nodeInfo.Path,
			Headers:             headers,
		}
		streamSetting.WSSettings = wsSettings
	case "grpc":
		grpcSettings := &conf.GRPCConfig{
			ServiceName: nodeInfo.ServiceName,
			Authority:   nodeInfo.Authority,
		}
		streamSetting.GRPCSettings = grpcSettings
	case "httpupgrade":
		httpupgradeSettings := &conf.HttpUpgradeConfig{
			Headers:             nodeInfo.Headers,
			Path:                nodeInfo.Path,
			Host:                nodeInfo.Host,
			AcceptProxyProtocol: nodeInfo.AcceptProxyProtocol,
		}
		streamSetting.HTTPUPGRADESettings = httpupgradeSettings
	case "splithttp", "xhttp":
		splithttpSetting := &conf.SplitHTTPConfig{
			Path: nodeInfo.Path,
			Host: nodeInfo.Host,
		}
		streamSetting.SplitHTTPSettings = splithttpSetting
	}
	streamSetting.Network = &transportProtocol

	// Build TLS and REALITY settings
	var isREALITY bool
	if cfg != nil && cfg.DisableLocalREALITYConfig {
		if nodeInfo.REALITYConfig != nil && nodeInfo.EnableREALITY {
			isREALITY = true
			streamSetting.Security = "reality"

			r := nodeInfo.REALITYConfig
			realityConf := &conf.REALITYConfig{
				Dest:         []byte(`"` + r.Dest + `"`),
				Xver:         r.ProxyProtocolVer,
				ServerNames:  r.ServerNames,
				PrivateKey:   r.PrivateKey,
				MinClientVer: r.MinClientVer,
				MaxClientVer: r.MaxClientVer,
				MaxTimeDiff:  r.MaxTimeDiff,
				ShortIds:     r.ShortIds,
			}
			if cfg.REALITYConfigs != nil {
				realityConf.Show = cfg.REALITYConfigs.Show
			}
			streamSetting.REALITYSettings = realityConf
		}
	} else if cfg != nil && cfg.EnableREALITY && cfg.REALITYConfigs != nil {
		isREALITY = true
		streamSetting.Security = "reality"

		streamSetting.REALITYSettings = &conf.REALITYConfig{
			Show:         cfg.REALITYConfigs.Show,
			Dest:         []byte(`"` + cfg.REALITYConfigs.Dest + `"`),
			Xver:         cfg.REALITYConfigs.ProxyProtocolVer,
			ServerNames:  cfg.REALITYConfigs.ServerNames,
			PrivateKey:   cfg.REALITYConfigs.PrivateKey,
			MinClientVer: cfg.REALITYConfigs.MinClientVer,
			MaxClientVer: cfg.REALITYConfigs.MaxClientVer,
			MaxTimeDiff:  cfg.REALITYConfigs.MaxTimeDiff,
			ShortIds:     cfg.REALITYConfigs.ShortIds,
		}
	}

	if !isREALITY && nodeInfo.EnableTLS && cfg != nil && cfg.CertConfig != nil && cfg.CertConfig.CertMode != "none" {
		streamSetting.Security = "tls"
		certFile, keyFile, err := b.certResolver.GetCertFile(cfg.CertConfig)
		if err != nil {
			return nil, err
		}
		tlsSettings := &conf.TLSConfig{
			RejectUnknownSNI: cfg.CertConfig.RejectUnknownSni,
		}
		tlsSettings.Certs = append(tlsSettings.Certs, &conf.TLSCertConfig{CertFile: certFile, KeyFile: keyFile, OcspStapling: 3600})
		streamSetting.TLSSettings = tlsSettings
	}

	// Support ProxyProtocol for any transport protocol
	if networkType != "tcp" && networkType != "ws" && cfg != nil && cfg.EnableProxyProtocol {
		sockoptConfig := &conf.SocketConfig{
			AcceptProxyProtocol: cfg.EnableProxyProtocol,
		}
		streamSetting.SocketSettings = sockoptConfig
	}
	inboundDetourConfig.StreamSetting = streamSetting

	return inboundDetourConfig.Build()
}

// BuildSSPluginDetour builds dokodemo-door inbound and outbound for SS-Plugin detour protocol.
func (b *NodeBuilder) BuildSSPluginDetour(cfg *config.Config, nodeInfo *api.NodeInfo, tag string) (inbound *core.InboundHandlerConfig, outbound *core.OutboundHandlerConfig, err error) {
	fakeNodeInfo := *nodeInfo
	fakeNodeInfo.Port++
	fakeNodeInfo.NodeType = "dokodemo-door"
	dokodemoTag := fmt.Sprintf("dokodemo-door_%s+1", tag)

	inboundConfig, err := b.BuildInbound(cfg, &fakeNodeInfo, dokodemoTag)
	if err != nil {
		return nil, nil, err
	}
	outboundConfig, err := b.BuildOutbound(cfg, &fakeNodeInfo, dokodemoTag)
	if err != nil {
		return nil, nil, err
	}
	return inboundConfig, outboundConfig, nil
}

func buildVlessFallbacks(fallbackConfigs []*config.FallBackConfig) ([]*conf.VLessInboundFallback, error) {
	if fallbackConfigs == nil {
		return nil, fmt.Errorf("you must provide FallBackConfigs")
	}

	vlessFallBacks := make([]*conf.VLessInboundFallback, len(fallbackConfigs))
	for i, c := range fallbackConfigs {
		if c.Dest == "" {
			return nil, fmt.Errorf("dest is required for fallback failed")
		}

		dest, err := json.Marshal(c.Dest)
		if err != nil {
			return nil, fmt.Errorf("marshal dest %s config failed: %w", dest, err)
		}
		vlessFallBacks[i] = &conf.VLessInboundFallback{
			Name: c.SNI,
			Alpn: c.Alpn,
			Path: c.Path,
			Dest: dest,
			Xver: c.ProxyProtocolVer,
		}
	}
	return vlessFallBacks, nil
}

func buildTrojanFallbacks(fallbackConfigs []*config.FallBackConfig) ([]*conf.TrojanInboundFallback, error) {
	if fallbackConfigs == nil {
		return nil, fmt.Errorf("you must provide FallBackConfigs")
	}

	trojanFallBacks := make([]*conf.TrojanInboundFallback, len(fallbackConfigs))
	for i, c := range fallbackConfigs {
		if c.Dest == "" {
			return nil, fmt.Errorf("dest is required for fallback failed")
		}

		dest, err := json.Marshal(c.Dest)
		if err != nil {
			return nil, fmt.Errorf("marshal dest %s config failed: %w", dest, err)
		}
		trojanFallBacks[i] = &conf.TrojanInboundFallback{
			Name: c.SNI,
			Alpn: c.Alpn,
			Path: c.Path,
			Dest: dest,
			Xver: c.ProxyProtocolVer,
		}
	}
	return trojanFallBacks, nil
}
