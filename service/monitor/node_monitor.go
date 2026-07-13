package monitor

import (
	"errors"
	"time"

	"github.com/Starktomy/XrayR/api"
)

func (m *Monitor) nodeInfoMonitor() error {
	if time.Since(m.startAt) < time.Duration(m.config.UpdatePeriodic)*time.Second {
		return nil
	}

	var nodeInfoChanged = true
	newNodeInfo, err := m.apiClient.GetNodeInfo()
	if err != nil {
		if errors.Is(err, api.ErrNodeNotModified) || err.Error() == api.NodeNotModified {
			nodeInfoChanged = false
			newNodeInfo = m.nodeCtrl.GetNodeInfo()
		} else {
			m.logger.Print(err)
			return nil
		}
	}
	if newNodeInfo != nil && newNodeInfo.Port == 0 {
		return errors.New("server port must > 0")
	}

	var usersChanged = true
	newUserInfo, err := m.apiClient.GetUserList()
	if err != nil {
		if errors.Is(err, api.ErrUserNotModified) || err.Error() == api.UserNotModified {
			usersChanged = false
			newUserInfo = m.nodeCtrl.GetUserList()
		} else {
			m.logger.Print(err)
			return nil
		}
	}

	if nodeInfoChanged {
		if err := m.nodeCtrl.RebuildNode(newNodeInfo); err != nil {
			m.logger.Print(err)
			return nil
		}
	}

	if !m.config.DisableGetRule {
		if ruleList, err := m.apiClient.GetNodeRule(); err != nil {
			if !errors.Is(err, api.ErrRuleNotModified) && err.Error() != api.RuleNotModified {
				m.logger.Printf("Get rule list filed: %s", err)
			}
		} else if ruleList != nil && len(*ruleList) > 0 {
			if err := m.nodeCtrl.UpdateRule(m.nodeCtrl.GetTag(), *ruleList); err != nil {
				m.logger.Print(err)
			}
		}
	}

	if nodeInfoChanged {
		// Handled by RebuildNode
	} else {
		var deleted, added []api.UserInfo
		if usersChanged {
			deleted, added = compareUserList(m.nodeCtrl.GetUserList(), newUserInfo)
			if len(deleted) > 0 || len(added) > 0 {
				if err := m.nodeCtrl.SyncUsers(deleted, added); err != nil {
					m.logger.Print(err)
				}
			}
		}
		m.logger.Printf("%d user deleted, %d user added", len(deleted), len(added))
	}
	m.nodeCtrl.SetUserList(newUserInfo)
	return nil
}

func compareUserList(old, new *[]api.UserInfo) (deleted, added []api.UserInfo) {
	if old == nil || new == nil {
		if old != nil {
			deleted = make([]api.UserInfo, len(*old))
			copy(deleted, *old)
			sortUserListForDiff(deleted)
			return deleted, nil
		}
		if new != nil {
			added = make([]api.UserInfo, len(*new))
			copy(added, *new)
			sortUserListForDiff(added)
			return nil, added
		}
		return nil, nil
	}

	oldByUID := make(map[int]api.UserInfo, len(*old))
	for _, u := range *old {
		oldByUID[u.UID] = u
	}

	for _, u := range *new {
		prev, ok := oldByUID[u.UID]
		if !ok {
			added = append(added, u)
			continue
		}
		if prev != u {
			deleted = append(deleted, prev)
			added = append(added, u)
		}
		delete(oldByUID, u.UID)
	}

	for _, u := range oldByUID {
		deleted = append(deleted, u)
	}

	sortUserListForDiff(deleted)
	sortUserListForDiff(added)
	return deleted, added
}

func sortUserListForDiff(s []api.UserInfo) {
	if len(s) < 2 {
		return
	}
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1].UID > s[j].UID; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
