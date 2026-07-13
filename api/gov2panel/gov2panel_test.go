package gov2panel_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Starktomy/XrayR/api"
	"github.com/Starktomy/XrayR/api/gov2panel"
	"github.com/gogf/gf/v2/encoding/gjson"
	"github.com/gogf/gf/v2/util/gconv"
)

func newMockPanelServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":[]}`))
	})
	return httptest.NewServer(mux)
}

func CreateClient(url string) api.API {
	apiConfig := &api.Config{
		APIHost:  url,
		Key:      "123456",
		NodeID:   90,
		NodeType: "V2ray",
	}
	return gov2panel.New(apiConfig)
}

func TestGetNodeInfo(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	nodeInfo, err := client.GetNodeInfo()
	if err != nil {
		t.Log(err)
	} else {
		nodeInfoJson := gjson.New(nodeInfo)
		t.Log(nodeInfoJson.String())
	}
}

func TestGetUserList(t *testing.T) {
	mock := newMockPanelServer(t)
	defer mock.Close()

	client := CreateClient(mock.URL)
	userList, err := client.GetUserList()
	if err != nil {
		t.Log(err)
	} else if userList != nil {
		t.Log(len(*userList))
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
	t.Log(userList)
	generalUserTraffic := make([]api.UserTraffic, len(*userList))
	for i, userInfo := range *userList {
		generalUserTraffic[i] = api.UserTraffic{
			UID:      userInfo.UID,
			Upload:   1073741824,
			Download: 1073741824,
		}
	}

	t.Log(gconv.String(generalUserTraffic))
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

