package newV2board_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/api/newV2board"
)

func newMockPanelServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	return httptest.NewServer(mux)
}

func CreateClient(url string) api.API {
	apiConfig := &api.Config{
		APIHost:  url,
		Key:      "qwertyuiopasdfghjkl",
		NodeID:   1,
		NodeType: "V2ray",
	}
	return newV2board.New(apiConfig)
}

func TestGetV2rayNodeInfo(t *testing.T) {
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

func TestGetSSNodeInfo(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	apiConfig := &api.Config{
		APIHost:  mock.URL,
		Key:      "qwertyuiopasdfghjkl",
		NodeID:   1,
		NodeType: "Shadowsocks",
	}
	client := newV2board.New(apiConfig)
	nodeInfo, err := client.GetNodeInfo()
	if err != nil {
		t.Log(err)
	} else {
		t.Log(nodeInfo)
	}
}

func TestGetTrojanNodeInfo(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	apiConfig := &api.Config{
		APIHost:  mock.URL,
		Key:      "qwertyuiopasdfghjkl",
		NodeID:   1,
		NodeType: "Trojan",
	}
	client := newV2board.New(apiConfig)
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

