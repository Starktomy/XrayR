package mylego_test

import (
	"strings"
	"testing"

	"github.com/Starktomy/XrayR/common/mylego"
)

// TestLegoNew verifies the constructor accepts an empty
// config (defaults to disk paths under cwd) and writes the
// expected LegoCMD fields.
func TestLegoNew(t *testing.T) {
	c, err := mylego.New(&mylego.CertConfig{
		CertDomain: "node1.test.com",
		Email:      "test@gmail.com",
	})
	if err != nil {
		t.Fatalf("New: %s", err)
	}
	if c == nil {
		t.Fatal("New returned nil LegoCMD")
	}
	if c.C.CertDomain != "node1.test.com" {
		t.Errorf("CertDomain = %q, want node1.test.com", c.C.CertDomain)
	}
	if c.C.Email != "test@gmail.com" {
		t.Errorf("Email = %q, want test@gmail.com", c.C.Email)
	}
}

// TestLegoNewNoDomain verifies the constructor doesn't blow
// up when the user hasn't provided a domain. The actual
// challenge still fails at run time, but the call itself
// must succeed.
func TestLegoNewNoDomain(t *testing.T) {
	c, err := mylego.New(&mylego.CertConfig{})
	if err != nil {
		t.Fatalf("New: %s", err)
	}
	if c == nil {
		t.Fatal("New returned nil LegoCMD")
	}
}

// TestLegoHTTPCertMissingFile verifies that HTTPCert
// returns an error (not a panic) when no prior certificate
// exists on disk. The pre-existing test called the real
// Let's Encrypt server, which obviously can't run in CI;
// this version exercises the same error path in isolation.
func TestLegoHTTPCertMissingFile(t *testing.T) {
	c, err := mylego.New(&mylego.CertConfig{
		CertMode:   "http",
		CertDomain: "definitely-not-registered.test.example",
		Email:      "test@gmail.com",
	})
	if err != nil {
		t.Fatalf("New: %s", err)
	}
	_, _, err = c.HTTPCert()
	if err == nil {
		t.Fatal("expected error from HTTPCert on a fresh host with no prior cert, got nil")
	}
	if !strings.Contains(err.Error(), "cert") {
		t.Errorf("error %q should mention cert loading failure", err.Error())
	}
}

// TestLegoRenewCertUnregistered verifies RenewCert returns
// the documented "not registered" error when the local
// account has no registration yet. Pre-existing test drove
// a full renewal against a real cert, which we can't run
// offline.
func TestLegoRenewCertUnregistered(t *testing.T) {
	c, err := mylego.New(&mylego.CertConfig{
		CertMode:   "dns",
		CertDomain: "definitely-not-registered.test.example",
		Email:      "test@gmail.com",
		Provider:   "alidns",
		DNSEnv: map[string]string{
			"ALICLOUD_ACCESS_KEY": "aaa",
			"ALICLOUD_SECRET_KEY": "bbb",
		},
	})
	if err != nil {
		t.Fatalf("New: %s", err)
	}
	_, _, ok, err := c.RenewCert()
	if err == nil {
		t.Fatal("expected error from RenewCert on unregistered host, got nil")
	}
	if ok {
		t.Error("expected ok=false on a failed renewal")
	}
}
