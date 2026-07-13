package config_test

import (
	"testing"

	"github.com/Starktomy/XrayR/config"
	"github.com/stretchr/testify/assert"
)

func TestConfigStructs(t *testing.T) {
	cfg := &config.Config{
		ListenIP:       "127.0.0.1",
		SendIP:         "127.0.0.1",
		UpdatePeriodic: 30,
		EnableDNS:      true,
		DNSType:        "AsIs",
		AutoSpeedLimitConfig: &config.AutoSpeedLimitConfig{
			Limit:         100,
			WarnTimes:     3,
			LimitSpeed:    10,
			LimitDuration: 60,
		},
		FallBackConfigs: []*config.FallBackConfig{
			{
				SNI:              "example.com",
				Alpn:             "h2",
				Path:             "/fallback",
				Dest:             "8080",
				ProxyProtocolVer: 1,
			},
		},
		EnableREALITY: true,
		REALITYConfigs: &config.REALITYConfig{
			Show:             true,
			Dest:             "example.com:443",
			ProxyProtocolVer: 0,
			ServerNames:      []string{"example.com"},
			PrivateKey:       "private_key_hash",
			MinClientVer:     "1.8.0",
			MaxClientVer:     "",
			MaxTimeDiff:      60,
			ShortIds:         []string{"16"},
		},
	}

	assert.Equal(t, "127.0.0.1", cfg.ListenIP)
	assert.Equal(t, 100, cfg.AutoSpeedLimitConfig.Limit)
	assert.Equal(t, 1, len(cfg.FallBackConfigs))
	assert.Equal(t, "example.com", cfg.FallBackConfigs[0].SNI)
	assert.True(t, cfg.REALITYConfigs.Show)
	assert.Equal(t, "private_key_hash", cfg.REALITYConfigs.PrivateKey)
}
