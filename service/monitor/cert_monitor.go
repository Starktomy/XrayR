package monitor

import (
	"github.com/Starktomy/XrayR/common/mylego"
)

func (m *Monitor) certMonitor() error {
	nodeInfo := m.nodeCtrl.GetNodeInfo()
	if nodeInfo != nil && nodeInfo.EnableTLS && !m.config.EnableREALITY {
		if m.config.CertConfig != nil {
			switch m.config.CertConfig.CertMode {
			case "dns", "http", "tls":
				lego, err := mylego.New(m.config.CertConfig)
				if err != nil {
					m.logger.Print(err)
					return err
				}
				if _, _, _, err = lego.RenewCert(); err != nil {
					m.logger.Print(err)
				}
			}
		}
	}
	return nil
}
