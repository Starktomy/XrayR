package mylego

import (
	"fmt"

	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
)

const rootPathWarningMessage = `!!!! HEADS UP !!!!

Your account credentials have been saved in your Let's Encrypt
configuration directory at "%s".

You should make a secure backup of this folder now. This
configuration directory will also contain certificates and
private keys obtained from Let's Encrypt so making regular
backups of this folder is ideal.
`

func (l *LegoCMD) Run() error {
	accountsStorage, err := NewAccountsStorage(l)
	if err != nil {
		return err
	}

	account, client, err := setup(accountsStorage)
	if err != nil {
		return err
	}
	if err := setupChallenges(l, client); err != nil {
		return err
	}

	if account.Registration == nil {
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			return fmt.Errorf("complete ACME registration: %w", err)
		}

		account.Registration = reg
		if err = accountsStorage.Save(account); err != nil {
			return fmt.Errorf("save account: %w", err)
		}

		fmt.Printf(rootPathWarningMessage, accountsStorage.GetRootPath())
	}

	certsStorage := NewCertificatesStorage(l.path)
	certsStorage.CreateRootFolder()

	cert, err := obtainCertificate([]string{l.C.CertDomain}, client)
	if err != nil {
		// Make sure to return a non-zero exit code if ObtainSANCertificate returned at least one error.
		// Due to us not returning partial certificate we can just exit here instead of at the end.
		return fmt.Errorf("obtain certificate: %w", err)
	}

	certsStorage.SaveResource(cert)

	return nil
}

func obtainCertificate(domains []string, client *lego.Client) (*certificate.Resource, error) {
	if len(domains) > 0 {
		// obtain a certificate, generating a new private key
		request := certificate.ObtainRequest{
			Domains: domains,
			Bundle:  true,
		}
		return client.Certificate.Obtain(request)
	}
	return nil, fmt.Errorf("not a valid domain")
}
