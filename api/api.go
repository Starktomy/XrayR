// Package api contains all the api used by XrayR
// To implement an api , one needs to implement the interface below.

package api

import (
	"fmt"
	"strings"
)

// API is the interface for different panel's api.
type API interface {
	GetNodeInfo() (nodeInfo *NodeInfo, err error)
	GetUserList() (userList *[]UserInfo, err error)
	ReportNodeStatus(nodeStatus *NodeStatus) (err error)
	ReportNodeOnlineUsers(onlineUser *[]OnlineUser) (err error)
	ReportUserTraffic(userTraffic *[]UserTraffic) (err error)
	Describe() ClientInfo
	GetNodeRule() (ruleList *[]DetectRule, err error)
	ReportIllegal(detectResultList *[]DetectResult) (err error)
}

// Creator is a factory function that creates an API instance
type Creator func(config *Config) API

var registry = make(map[string]Creator)

// RegisterPanel registers a panel adapter factory. The panelType is case-insensitive.
func RegisterPanel(panelType string, creator Creator) {
	registry[strings.ToLower(panelType)] = creator
}

// CreatePanel creates an API instance for the given panelType.
func CreatePanel(panelType string, config *Config) (API, error) {
	creator, ok := registry[strings.ToLower(panelType)]
	if !ok {
		return nil, fmt.Errorf("unsupported panel type: %s", panelType)
	}
	return creator(config), nil
}
