package proxypanel_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/api/proxypanel"
)

func newMockPanelServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":[]}`))
	})
	return httptest.NewServer(mux)
}

func CreateClient(url string) api.API {
	apiConfig := &api.Config{
		APIHost:  url,
		Key:      "naBDpLvREiwY9qPr",
		NodeID:   1,
		NodeType: "V2ray",
	}
	return proxypanel.New(apiConfig)
}

func TestGetV2rayNodeinfo(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	apiConfig := &api.Config{
		APIHost:  mock.URL,
		Key:      "naBDpLvREiwY9qPr",
		NodeID:   1,
		NodeType: "V2ray",
	}
	client := proxypanel.New(apiConfig)

	nodeInfo, err := client.GetNodeInfo()
	if err != nil {
		t.Log(err)
	} else {
		t.Log(nodeInfo)
	}
}

func TestGetSSNodeinfo(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	apiConfig := &api.Config{
		APIHost:  mock.URL,
		Key:      "8VtrYVGFHL0Q9azc",
		NodeID:   3,
		NodeType: "Shadowsocks",
	}
	client := proxypanel.New(apiConfig)
	nodeInfo, err := client.GetNodeInfo()
	if err != nil {
		t.Log(err)
	} else {
		t.Log(nodeInfo)
	}
}

func TestGetTrojanNodeinfo(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	apiConfig := &api.Config{
		APIHost:  mock.URL,
		Key:      "kgnO2O66FmvP8rDV",
		NodeID:   2,
		NodeType: "Trojan",
	}
	client := proxypanel.New(apiConfig)
	nodeInfo, err := client.GetNodeInfo()
	if err != nil {
		t.Log(err)
	} else {
		t.Log(nodeInfo)
	}
}

func TestGetSSinfo(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	nodeInfo, err := client.GetNodeInfo()
	if err != nil {
		t.Log(err)
	} else {
		t.Log(nodeInfo)
	}
}

func TestGetUserList(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	userList, err := client.GetUserList()
	if err != nil {
		t.Log(err)
	} else {
		t.Log(userList)
	}
}

func TestReportNodeStatus(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	nodeStatus := &api.NodeStatus{
		CPU: 1, Mem: 1, Disk: 1, Uptime: 256,
	}
	err := client.ReportNodeStatus(nodeStatus)
	if err != nil {
		t.Log(err)
	}
}

func TestReportReportNodeOnlineUsers(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	userList, err := client.GetUserList()
	if err != nil || userList == nil {
		return
	}

	onlineUserList := make([]api.OnlineUser, len(*userList))
	for i, userInfo := range *userList {
		onlineUserList[i] = api.OnlineUser{
			UID: userInfo.UID,
			IP:  fmt.Sprintf("1.1.1.%d", i),
		}
	}
	err = client.ReportNodeOnlineUsers(&onlineUserList)
	if err != nil {
		t.Log(err)
	}
}

func TestReportReportUserTraffic(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	userList, err := client.GetUserList()
	if err != nil || userList == nil {
		return
	}
	generalUserTraffic := make([]api.UserTraffic, len(*userList))
	for i, userInfo := range *userList {
		generalUserTraffic[i] = api.UserTraffic{
			UID:      userInfo.UID,
			Upload:   114514,
			Download: 114514,
		}
	}
	err = client.ReportUserTraffic(&generalUserTraffic)
	if err != nil {
		t.Log(err)
	}
}

func TestGetNodeRule(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	ruleList, err := client.GetNodeRule()
	if err != nil {
		t.Log(err)
	} else {
		t.Log(ruleList)
	}
}

func TestReportIllegal(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	detectResult := []api.DetectResult{
		{UID: 1, RuleID: 1},
		{UID: 1, RuleID: 2},
	}
	err := client.ReportIllegal(&detectResult)
	if err != nil {
		t.Log(err)
	}
}

