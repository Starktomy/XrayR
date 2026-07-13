package monitor

import (
	"fmt"
	"time"

	"github.com/xtls/xray-core/features/stats"

	"github.com/Starktomy/XrayR/api"
)

func buildUserTag(tag string, user *api.UserInfo) string {
	return fmt.Sprintf("%s|%s|%d", tag, user.Email, user.UID)
}

func (m *Monitor) limitUser(user api.UserInfo, silentUsers *[]api.UserInfo) {
	m.limitedUsers[user] = LimitInfo{
		end:               time.Now().Unix() + int64(m.config.AutoSpeedLimitConfig.LimitDuration*60),
		currentSpeedLimit: m.config.AutoSpeedLimitConfig.LimitSpeed,
		originSpeedLimit:  user.SpeedLimit,
	}
	tag := buildUserTag(m.nodeCtrl.GetTag(), &user)
	m.logger.Printf("Limit User: %s Speed: %d End: %s", tag, m.config.AutoSpeedLimitConfig.LimitSpeed, time.Unix(m.limitedUsers[user].end, 0).Format("01-02 15:04:05"))
	user.SpeedLimit = uint64((m.config.AutoSpeedLimitConfig.LimitSpeed * 1000000) / 8)
	*silentUsers = append(*silentUsers, user)
}

func (m *Monitor) userInfoMonitor() error {
	if time.Since(m.startAt) < time.Duration(m.config.UpdatePeriodic)*time.Second {
		return nil
	}

	CPU, Mem, Disk, Uptime, err := m.sysStatus.GetSystemInfo()
	if err != nil {
		m.logger.Print(err)
	}
	err = m.apiClient.ReportNodeStatus(
		&api.NodeStatus{
			CPU:    CPU,
			Mem:    Mem,
			Disk:   Disk,
			Uptime: Uptime,
		})
	if err != nil {
		m.logger.Print(err)
	}

	if m.config.AutoSpeedLimitConfig != nil && m.config.AutoSpeedLimitConfig.Limit > 0 && len(m.limitedUsers) > 0 {
		m.logger.Printf("Limited users:")
		toReleaseUsers := make([]api.UserInfo, 0)
		for user, limitInfo := range m.limitedUsers {
			if time.Now().Unix() > limitInfo.end {
				user.SpeedLimit = limitInfo.originSpeedLimit
				toReleaseUsers = append(toReleaseUsers, user)
				tag := buildUserTag(m.nodeCtrl.GetTag(), &user)
				m.logger.Printf("User: %s Speed: %d End: nil (Unlimit)", tag, user.SpeedLimit)
				delete(m.limitedUsers, user)
			} else {
				tag := buildUserTag(m.nodeCtrl.GetTag(), &user)
				m.logger.Printf("User: %s Speed: %d End: %s", tag, limitInfo.currentSpeedLimit, time.Unix(m.limitedUsers[user].end, 0).Format("01-02 15:04:05"))
			}
		}
		if len(toReleaseUsers) > 0 {
			if err := m.nodeCtrl.UpdateInboundLimiter(m.nodeCtrl.GetTag(), &toReleaseUsers); err != nil {
				m.logger.Print(err)
			}
		}
	}

	rawUserList := m.nodeCtrl.GetUserList()
	var userList []api.UserInfo
	if rawUserList != nil {
		userList = *rawUserList
	}
	userCount := len(userList)
	userTraffic := make([]api.UserTraffic, 0, userCount)
	upCounterList := make([]stats.Counter, 0, userCount)
	downCounterList := make([]stats.Counter, 0, userCount)
	AutoSpeedLimit := int64(0)
	if m.config.AutoSpeedLimitConfig != nil {
		AutoSpeedLimit = int64(m.config.AutoSpeedLimitConfig.Limit)
	}
	UpdatePeriodic := int64(m.config.UpdatePeriodic)
	limitedUsers := make([]api.UserInfo, 0)

	for _, user := range userList {
		userTag := buildUserTag(m.nodeCtrl.GetTag(), &user)
		up, down, upCounter, downCounter := m.metrics.GetTraffic(userTag)
		if down > 0 {
			m.logger.Printf("Traffic counted: tag=%s up=%d down=%d", userTag, up, down)
		}
		if up > 0 || down > 0 {
			if AutoSpeedLimit > 0 {
				threshold := AutoSpeedLimit * 1000000 * UpdatePeriodic / 8
				if down > threshold || up > threshold {
					if _, ok := m.limitedUsers[user]; !ok {
						if m.config.AutoSpeedLimitConfig.WarnTimes == 0 {
							m.limitUser(user, &limitedUsers)
						} else {
							m.warnedUsers[user] += 1
							if m.warnedUsers[user] > m.config.AutoSpeedLimitConfig.WarnTimes {
								m.limitUser(user, &limitedUsers)
								delete(m.warnedUsers, user)
							}
						}
					}
				} else {
					delete(m.warnedUsers, user)
				}
			}
			userTraffic = append(userTraffic, api.UserTraffic{
				UID:      user.UID,
				Email:    user.Email,
				Upload:   up,
				Download: down,
			})

			if upCounter != nil {
				upCounterList = append(upCounterList, upCounter)
			}
			if downCounter != nil {
				downCounterList = append(downCounterList, downCounter)
			}
		} else {
			delete(m.warnedUsers, user)
		}
	}

	if len(limitedUsers) > 0 {
		if err := m.nodeCtrl.UpdateInboundLimiter(m.nodeCtrl.GetTag(), &limitedUsers); err != nil {
			m.logger.Print(err)
		}
	}

	if len(userTraffic) > 0 {
		m.logger.Printf("Reporting %d user(s) traffic to panel; example: UID=%d up=%d down=%d", len(userTraffic), userTraffic[0].UID, userTraffic[0].Upload, userTraffic[0].Download)
		var err error
		if m.config.DisableUploadTraffic {
			m.metrics.ResetTraffic(&upCounterList, &downCounterList)
		} else {
			err = m.apiClient.ReportUserTraffic(&userTraffic)
			if err != nil {
				m.logger.Print(err)
			} else {
				m.metrics.ResetTraffic(&upCounterList, &downCounterList)
			}
		}
	}

	if onlineDevice, err := m.metrics.GetOnlineDevice(m.nodeCtrl.GetTag()); err != nil {
		m.logger.Print(err)
	} else if onlineDevice != nil && len(*onlineDevice) > 0 {
		if err = m.apiClient.ReportNodeOnlineUsers(onlineDevice); err != nil {
			m.logger.Print(err)
		} else {
			m.logger.Printf("Report %d online users", len(*onlineDevice))
		}
	}

	if detectResult, err := m.metrics.GetDetectResult(m.nodeCtrl.GetTag()); err != nil {
		m.logger.Print(err)
	} else if detectResult != nil && len(*detectResult) > 0 {
		if err = m.apiClient.ReportIllegal(detectResult); err != nil {
			m.logger.Print(err)
		} else {
			m.logger.Printf("Report %d illegal behaviors", len(*detectResult))
		}
	}

	return nil
}
