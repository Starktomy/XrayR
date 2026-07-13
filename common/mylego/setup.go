package mylego

import (
	"fmt"
	"os"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/challenge/tlsalpn01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/providers/dns"
	"github.com/go-acme/lego/v4/registration"
	"golang.org/x/crypto/acme"
)

const filePerm os.FileMode = 0o600

// setup loads (or creates) the ACME account and returns a
// configured lego client. It returns an error rather than
// panicking so the cert monitor in controller can surface the
// failure instead of crashing XrayR.
func setup(accountsStorage *AccountsStorage) (*Account, *lego.Client, error) {
	keyType := certcrypto.EC256
	privateKey, err := accountsStorage.GetPrivateKey(keyType)
	if err != nil {
		return nil, nil, err
	}

	var account *Account
	exists, err := accountsStorage.ExistsAccountFilePath()
	if err != nil {
		return nil, nil, err
	}
	if exists {
		account, err = accountsStorage.LoadAccount(privateKey)
		if err != nil {
			return nil, nil, err
		}
	} else {
		account = &Account{Email: accountsStorage.GetUserID(), key: privateKey}
	}

	client, err := newClient(account, keyType)
	if err != nil {
		return nil, nil, err
	}

	return account, client, nil
}

func newClient(acc registration.User, keyType certcrypto.KeyType) (*lego.Client, error) {
	config := lego.NewConfig(acc)
	config.CADirURL = acme.LetsEncryptURL

	config.Certificate = lego.CertificateConfig{
		KeyType: keyType,
		Timeout: 30 * time.Second,
	}
	config.UserAgent = "lego-cli/dev"

	client, err := lego.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create lego client: %w", err)
	}

	return client, nil
}

func createNonExistingFolder(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return os.MkdirAll(path, 0o700)
	} else if err != nil {
		return err
	}
	return nil
}

func setupChallenges(l *LegoCMD, client *lego.Client) error {
	switch l.C.CertMode {
	case "http":
		if err := client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", "")); err != nil {
			return fmt.Errorf("set HTTP-01 challenge provider: %w", err)
		}
	case "tls":
		if err := client.Challenge.SetTLSALPN01Provider(tlsalpn01.NewProviderServer("", "")); err != nil {
			return fmt.Errorf("set TLS-ALPN-01 challenge provider: %w", err)
		}
	case "dns":
		return setupDNS(l.C.Provider, client)
	default:
		return fmt.Errorf("no challenge selected. Specify at least one: http, tls, dns")
	}
	return nil
}

func setupDNS(p string, client *lego.Client) error {
	provider, err := dns.NewDNSChallengeProviderByName(p)
	if err != nil {
		return fmt.Errorf("create DNS provider %q: %w", p, err)
	}

	if err := client.Challenge.SetDNS01Provider(
		provider,
		dns01.CondOption(true, dns01.AddDNSTimeout(10*time.Second)),
	); err != nil {
		return fmt.Errorf("set DNS-01 challenge provider: %w", err)
	}
	return nil
}
