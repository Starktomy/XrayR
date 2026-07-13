package nodebuilder_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/common/mylego"
	"github.com/Starktomy/XrayR/config"
	. "github.com/Starktomy/XrayR/service/nodebuilder"
)

// MockCertResolver satisfies nodebuilder.CertResolver for offline unit testing without network/ACME calls.
type MockCertResolver struct {
	CertFile string
	KeyFile  string
	Err      error
}

func (m *MockCertResolver) GetCertFile(certConfig *mylego.CertConfig) (string, string, error) {
	if m.Err != nil {
		return "", "", m.Err
	}
	if m.CertFile != "" && m.KeyFile != "" {
		return m.CertFile, m.KeyFile, nil
	}
	return "/tmp/mock_cert.crt", "/tmp/mock_cert.key", nil
}

func createTempTLSCert(t *testing.T) (string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate private key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Co"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}
	certPem := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("failed to marshal private key: %v", err)
	}
	keyPem := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privBytes})

	tmpDir := t.TempDir()
	certPath := filepath.Join(tmpDir, "server.crt")
	keyPath := filepath.Join(tmpDir, "server.key")

	if err := os.WriteFile(certPath, certPem, 0644); err != nil {
		t.Fatalf("failed to write cert file: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPem, 0600); err != nil {
		t.Fatalf("failed to write key file: %v", err)
	}
	return certPath, keyPath
}

func TestBuildInbound_VMess(t *testing.T) {
	transports := []string{"tcp", "ws", "grpc", "httpupgrade", "splithttp", "xhttp"}
	builder := New(&MockCertResolver{})

	for _, tr := range transports {
		t.Run("Transport_"+tr, func(t *testing.T) {
			nodeInfo := &api.NodeInfo{
				NodeType:          "Vmess",
				NodeID:            1,
				Port:              10080,
				TransportProtocol: tr,
				Host:              "example.com",
				Path:              "/vmess",
				ServiceName:       "vmess-grpc",
				Authority:         "grpc.example.com",
			}
			cfg := &config.Config{
				ListenIP: "0.0.0.0",
			}
			inbound, err := builder.BuildInbound(cfg, nodeInfo, "vmess_tag")
			if err != nil {
				t.Fatalf("BuildInbound VMess (%s) failed: %v", tr, err)
			}
			if inbound.Tag != "vmess_tag" {
				t.Errorf("expected tag vmess_tag, got %s", inbound.Tag)
			}
			if inbound.ReceiverSettings == nil {
				t.Errorf("expected non-nil ReceiverSettings")
			}
		})
	}
}

func TestBuildInbound_VLESS(t *testing.T) {
	builder := New(&MockCertResolver{})

	t.Run("VLESS_Basic", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{
			NodeType:          "Vless",
			NodeID:            2,
			Port:              10081,
			EnableVless:       true,
			TransportProtocol: "tcp",
		}
		cfg := &config.Config{}
		inbound, err := builder.BuildInbound(cfg, nodeInfo, "vless_tag")
		if err != nil {
			t.Fatalf("BuildInbound VLESS failed: %v", err)
		}
		if inbound.Tag != "vless_tag" {
			t.Errorf("expected tag vless_tag, got %s", inbound.Tag)
		}
	})

	t.Run("VLESS_WithFallback", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{
			NodeType:          "Vless",
			NodeID:            3,
			Port:              443,
			EnableVless:       true,
			TransportProtocol: "tcp",
		}
		cfg := &config.Config{
			EnableFallback: true,
			FallBackConfigs: []*config.FallBackConfig{
				{SNI: "example.com", Dest: "8080", Path: "/fallback"},
			},
		}
		inbound, err := builder.BuildInbound(cfg, nodeInfo, "vless_fallback_tag")
		if err != nil {
			t.Fatalf("BuildInbound VLESS with fallback failed: %v", err)
		}
		if inbound == nil {
			t.Fatal("expected non-nil inbound")
		}
	})
}

func TestBuildInbound_Trojan(t *testing.T) {
	builder := New(&MockCertResolver{})

	t.Run("Trojan_Basic", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{
			NodeType:          "Trojan",
			NodeID:            4,
			Port:              4430,
			TransportProtocol: "tcp",
		}
		cfg := &config.Config{}
		inbound, err := builder.BuildInbound(cfg, nodeInfo, "trojan_tag")
		if err != nil {
			t.Fatalf("BuildInbound Trojan failed: %v", err)
		}
		if inbound.Tag != "trojan_tag" {
			t.Errorf("expected tag trojan_tag, got %s", inbound.Tag)
		}
	})

	t.Run("Trojan_WithFallback", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{
			NodeType:          "Trojan",
			NodeID:            5,
			Port:              4431,
			TransportProtocol: "tcp",
		}
		cfg := &config.Config{
			EnableFallback: true,
			FallBackConfigs: []*config.FallBackConfig{
				{SNI: "trojan.com", Dest: "8081"},
			},
		}
		inbound, err := builder.BuildInbound(cfg, nodeInfo, "trojan_fallback_tag")
		if err != nil {
			t.Fatalf("BuildInbound Trojan with fallback failed: %v", err)
		}
		if inbound == nil {
			t.Fatal("expected non-nil inbound")
		}
	})
}

func TestBuildInbound_Shadowsocks(t *testing.T) {
	builder := New(&MockCertResolver{})

	ciphers := []string{"aes-256-gcm", "aes-128-gcm", "chacha20-poly1305"}
	for _, cipher := range ciphers {
		t.Run("Cipher_"+cipher, func(t *testing.T) {
			nodeInfo := &api.NodeInfo{
				NodeType:          "Shadowsocks",
				NodeID:            6,
				Port:              8388,
				CypherMethod:      cipher,
				TransportProtocol: "tcp",
			}
			cfg := &config.Config{}
			inbound, err := builder.BuildInbound(cfg, nodeInfo, "ss_tag")
			if err != nil {
				t.Fatalf("BuildInbound SS (%s) failed: %v", cipher, err)
			}
			if inbound == nil {
				t.Fatal("expected non-nil inbound")
			}
		})
	}
}

func TestBuildInbound_Shadowsocks2022(t *testing.T) {
	builder := New(&MockCertResolver{})

	ss2022Ciphers := []string{"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm"}
	for _, cipher := range ss2022Ciphers {
		t.Run("SS2022_"+cipher, func(t *testing.T) {
			nodeInfo := &api.NodeInfo{
				NodeType:          "Shadowsocks",
				NodeID:            7,
				Port:              8389,
				CypherMethod:      cipher,
				ServerKey:         "dGVzdF9zaGFyZWRfa2V5XzMyX2J5dGVzX2xvbmc=",
				TransportProtocol: "tcp",
			}
			cfg := &config.Config{}
			inbound, err := builder.BuildInbound(cfg, nodeInfo, "ss2022_tag")
			if err != nil {
				t.Fatalf("BuildInbound SS2022 (%s) failed: %v", cipher, err)
			}
			if inbound == nil {
				t.Fatal("expected non-nil inbound")
			}
		})
	}
}

func TestBuildInbound_ShadowsocksPlugin(t *testing.T) {
	builder := New(&MockCertResolver{})

	nodeInfo := &api.NodeInfo{
		NodeType:          "Shadowsocks-Plugin",
		NodeID:            8,
		Port:              8400,
		CypherMethod:      "aes-256-gcm",
		TransportProtocol: "ws",
	}
	cfg := &config.Config{}
	inbound, err := builder.BuildInbound(cfg, nodeInfo, "ssp_tag")
	if err != nil {
		t.Fatalf("BuildInbound SS-Plugin failed: %v", err)
	}
	if inbound == nil {
		t.Fatal("expected non-nil inbound")
	}
}

func TestBuildInbound_REALITY(t *testing.T) {
	builder := New(&MockCertResolver{})
	// 32-byte raw base64url encoded key for X25519
	validPrivateKey := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	t.Run("LocalREALITYConfig", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{
			NodeType:          "Vless",
			NodeID:            9,
			Port:              443,
			EnableVless:       true,
			TransportProtocol: "tcp",
		}
		cfg := &config.Config{
			EnableREALITY: true,
			REALITYConfigs: &config.REALITYConfig{
				Show:         false,
				Dest:         "1.1.1.1:443",
				ServerNames:  []string{"cloudflare.com"},
				PrivateKey:   validPrivateKey,
				ShortIds:     []string{"0123456789abcdef"},
				MinClientVer: "",
				MaxClientVer: "",
				MaxTimeDiff:  0,
			},
		}
		inbound, err := builder.BuildInbound(cfg, nodeInfo, "reality_local_tag")
		if err != nil {
			t.Fatalf("BuildInbound REALITY (local) failed: %v", err)
		}
		if inbound == nil {
			t.Fatal("expected non-nil inbound")
		}
	})

	t.Run("RemoteREALITYConfig", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{
			NodeType:          "Vless",
			NodeID:            10,
			Port:              443,
			EnableVless:       true,
			TransportProtocol: "tcp",
			EnableREALITY:     true,
			REALITYConfig: &api.REALITYConfig{
				Dest:        "example.com:443",
				ServerNames: []string{"example.com"},
				PrivateKey:  validPrivateKey,
				ShortIds:    []string{"1234"},
			},
		}
		cfg := &config.Config{
			DisableLocalREALITYConfig: true,
		}
		inbound, err := builder.BuildInbound(cfg, nodeInfo, "reality_remote_tag")
		if err != nil {
			t.Fatalf("BuildInbound REALITY (remote) failed: %v", err)
		}
		if inbound == nil {
			t.Fatal("expected non-nil inbound")
		}
	})
}

func TestBuildInbound_TLS_MockResolver(t *testing.T) {
	certFile, keyFile := createTempTLSCert(t)
	mockRes := &MockCertResolver{
		CertFile: certFile,
		KeyFile:  keyFile,
	}
	builder := New(mockRes)

	nodeInfo := &api.NodeInfo{
		NodeType:          "Vmess",
		NodeID:            11,
		Port:              443,
		EnableTLS:         true,
		TransportProtocol: "tcp",
	}
	cfg := &config.Config{
		CertConfig: &mylego.CertConfig{
			CertMode:   "dns",
			CertDomain: "test.org",
			Provider:   "cloudflare",
		},
	}

	inbound, err := builder.BuildInbound(cfg, nodeInfo, "tls_tag")
	if err != nil {
		t.Fatalf("BuildInbound with TLS mock resolver failed: %v", err)
	}
	if inbound == nil {
		t.Fatal("expected non-nil inbound")
	}
}

func TestBuildOutbound(t *testing.T) {
	builder := New(&MockCertResolver{})

	t.Run("BasicFreedom", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{NodeType: "Vmess"}
		cfg := &config.Config{SendIP: "127.0.0.1"}
		outbound, err := builder.BuildOutbound(cfg, nodeInfo, "out_tag")
		if err != nil {
			t.Fatalf("BuildOutbound basic failed: %v", err)
		}
		if outbound.Tag != "out_tag" {
			t.Errorf("expected tag out_tag, got %s", outbound.Tag)
		}
	})

	t.Run("DokodemoDoorRedirect", func(t *testing.T) {
		nodeInfo := &api.NodeInfo{
			NodeType: "dokodemo-door",
			Port:     8401,
		}
		cfg := &config.Config{}
		outbound, err := builder.BuildOutbound(cfg, nodeInfo, "dokodemo_out_tag")
		if err != nil {
			t.Fatalf("BuildOutbound dokodemo-door failed: %v", err)
		}
		if outbound == nil {
			t.Fatal("expected non-nil outbound")
		}
	})
}

func TestBuildSSPluginDetour(t *testing.T) {
	builder := New(&MockCertResolver{})

	nodeInfo := &api.NodeInfo{
		NodeType:          "Shadowsocks-Plugin",
		NodeID:            12,
		Port:              8400,
		CypherMethod:      "aes-256-gcm",
		TransportProtocol: "ws",
	}
	cfg := &config.Config{}

	inbound, outbound, err := builder.BuildSSPluginDetour(cfg, nodeInfo, "ss_plugin_tag")
	if err != nil {
		t.Fatalf("BuildSSPluginDetour failed: %v", err)
	}
	if inbound.Tag != "dokodemo-door_ss_plugin_tag+1" {
		t.Errorf("expected inbound tag dokodemo-door_ss_plugin_tag+1, got %s", inbound.Tag)
	}
	if outbound.Tag != "dokodemo-door_ss_plugin_tag+1" {
		t.Errorf("expected outbound tag dokodemo-door_ss_plugin_tag+1, got %s", outbound.Tag)
	}
}

func TestBuildUser(t *testing.T) {
	builder := New(&MockCertResolver{})

	users := []api.UserInfo{
		{UID: 1, Email: "user1@test.com", UUID: "00000000-0000-0000-0000-000000000001", Passwd: "password123", Method: "aes-256-gcm"},
		{UID: 2, Email: "user2@test.com", UUID: "00000000-0000-0000-0000-000000000002", Passwd: "password123456789012345678901234567890", Method: "2022-blake3-aes-128-gcm"},
	}

	t.Run("VMessUsers", func(t *testing.T) {
		builtUsers, err := builder.BuildUser("Vmess", &users, "test_tag", "", "")
		if err != nil {
			t.Fatalf("BuildUser VMess failed: %v", err)
		}
		if len(builtUsers) != 2 {
			t.Errorf("expected 2 users, got %d", len(builtUsers))
		}
		if builtUsers[0].Email != "test_tag|user1@test.com|1" {
			t.Errorf("unexpected user email: %s", builtUsers[0].Email)
		}
	})

	t.Run("VLESSUsers", func(t *testing.T) {
		builtUsers, err := builder.BuildUser("Vless", &users, "vless_tag", "xtls-rprx-vision", "")
		if err != nil {
			t.Fatalf("BuildUser VLESS failed: %v", err)
		}
		if len(builtUsers) != 2 {
			t.Errorf("expected 2 users, got %d", len(builtUsers))
		}
	})

	t.Run("TrojanUsers", func(t *testing.T) {
		builtUsers, err := builder.BuildUser("Trojan", &users, "trojan_tag", "", "")
		if err != nil {
			t.Fatalf("BuildUser Trojan failed: %v", err)
		}
		if len(builtUsers) != 2 {
			t.Errorf("expected 2 users, got %d", len(builtUsers))
		}
	})

	t.Run("SSUsers_DefaultPanel", func(t *testing.T) {
		builtUsers, err := builder.BuildUser("Shadowsocks", &users, "ss_tag", "", "")
		if err != nil {
			t.Fatalf("BuildUser SS failed: %v", err)
		}
		if len(builtUsers) != 2 {
			t.Errorf("expected 2 users, got %d", len(builtUsers))
		}
	})

	t.Run("SSUsers_V2boardPanel", func(t *testing.T) {
		builtUsers, err := builder.BuildUser("Shadowsocks", &users, "ss_v2board_tag", "", "V2board")
		if err != nil {
			t.Fatalf("BuildUser SS V2board failed: %v", err)
		}
		if len(builtUsers) != 2 {
			t.Errorf("expected 2 users, got %d", len(builtUsers))
		}
	})

	t.Run("SSPluginUsers", func(t *testing.T) {
		builtUsers, err := builder.BuildUser("Shadowsocks-Plugin", &users, "ssp_tag", "", "")
		if err != nil {
			t.Fatalf("BuildUser SS-Plugin failed: %v", err)
		}
		if len(builtUsers) != 2 {
			t.Errorf("expected 2 users, got %d", len(builtUsers))
		}
	})

	t.Run("UnsupportedNodeType", func(t *testing.T) {
		_, err := builder.BuildUser("UnknownType", &users, "tag", "", "")
		if err == nil {
			t.Errorf("expected error for unknown node type, got nil")
		}
	})
}

func TestPackageLevelFunctions(t *testing.T) {
	nodeInfo := &api.NodeInfo{
		NodeType:          "Vmess",
		NodeID:            100,
		Port:              10080,
		TransportProtocol: "tcp",
	}
	cfg := &config.Config{}

	inbound, err := BuildInbound(cfg, nodeInfo, "pkg_tag")
	if err != nil {
		t.Fatalf("Package-level BuildInbound failed: %v", err)
	}
	if inbound == nil {
		t.Fatal("expected non-nil inbound")
	}

	outbound, err := BuildOutbound(cfg, nodeInfo, "pkg_tag")
	if err != nil {
		t.Fatalf("Package-level BuildOutbound failed: %v", err)
	}
	if outbound == nil {
		t.Fatal("expected non-nil outbound")
	}

	users := []api.UserInfo{{UID: 1, Email: "a@b.com", UUID: "00000000-0000-0000-0000-000000000000"}}
	builtUsers, err := BuildUser("Vmess", &users, "pkg_tag", "", "")
	if err != nil {
		t.Fatalf("Package-level BuildUser failed: %v", err)
	}
	if len(builtUsers) != 1 {
		t.Errorf("expected 1 user, got %d", len(builtUsers))
	}

	ssNodeInfo := &api.NodeInfo{
		NodeType:          "Shadowsocks-Plugin",
		NodeID:            101,
		Port:              10081,
		TransportProtocol: "tcp",
	}
	inboundDetour, outboundDetour, err := BuildSSPluginDetour(cfg, ssNodeInfo, "pkg_tag")
	if err != nil {
		t.Fatalf("Package-level BuildSSPluginDetour failed: %v", err)
	}
	if inboundDetour == nil || outboundDetour == nil {
		t.Fatal("expected non-nil inbound and outbound detours")
	}
}
