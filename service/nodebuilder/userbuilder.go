package nodebuilder

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	C "github.com/sagernet/sing/common"
	"github.com/xtls/xray-core/common/errors"
	"github.com/xtls/xray-core/common/protocol"
	"github.com/xtls/xray-core/common/serial"
	"github.com/xtls/xray-core/infra/conf"
	"github.com/xtls/xray-core/proxy/shadowsocks"
	"github.com/xtls/xray-core/proxy/shadowsocks_2022"
	"github.com/xtls/xray-core/proxy/trojan"
	"github.com/xtls/xray-core/proxy/vless"

	"github.com/Starktomy/XrayR/api"
)

var AEADMethod = map[shadowsocks.CipherType]uint8{
	shadowsocks.CipherType_AES_128_GCM:        0,
	shadowsocks.CipherType_AES_256_GCM:        0,
	shadowsocks.CipherType_CHACHA20_POLY1305:  0,
	shadowsocks.CipherType_XCHACHA20_POLY1305: 0,
}

// BuildUser creates Xray users for the specified nodeType and user list.
func (b *NodeBuilder) BuildUser(nodeType string, userInfo *[]api.UserInfo, tag string, vlessFlow string, panelType string) ([]*protocol.User, error) {
	if userInfo == nil {
		return []*protocol.User{}, nil
	}
	switch nodeType {
	case "V2ray", "Vmess":
		return buildVmessUser(userInfo, tag), nil
	case "Vless":
		return buildVlessUser(userInfo, tag, vlessFlow), nil
	case "Trojan":
		return buildTrojanUser(userInfo, tag), nil
	case "Shadowsocks":
		return buildSSUser(userInfo, tag, panelType, nodeType), nil
	case "Shadowsocks-Plugin":
		return buildSSPluginUser(userInfo, tag, panelType), nil
	default:
		return nil, fmt.Errorf("unsupported node type: %s", nodeType)
	}
}

func buildVmessUser(userInfo *[]api.UserInfo, tag string) []*protocol.User {
	users := make([]*protocol.User, len(*userInfo))
	for i, user := range *userInfo {
		vmessAccount := &conf.VMessAccount{
			ID:       user.UUID,
			Security: "auto",
		}
		users[i] = &protocol.User{
			Level:   0,
			Email:   buildUserTag(tag, &user),
			Account: serial.ToTypedMessage(vmessAccount.Build()),
		}
	}
	return users
}

func buildVlessUser(userInfo *[]api.UserInfo, tag string, vlessFlow string) []*protocol.User {
	users := make([]*protocol.User, len(*userInfo))
	for i, user := range *userInfo {
		vlessAccount := &vless.Account{
			Id:   user.UUID,
			Flow: vlessFlow,
		}
		users[i] = &protocol.User{
			Level:   0,
			Email:   buildUserTag(tag, &user),
			Account: serial.ToTypedMessage(vlessAccount),
		}
	}
	return users
}

func buildTrojanUser(userInfo *[]api.UserInfo, tag string) []*protocol.User {
	users := make([]*protocol.User, len(*userInfo))
	for i, user := range *userInfo {
		trojanAccount := &trojan.Account{
			Password: user.UUID,
		}
		users[i] = &protocol.User{
			Level:   0,
			Email:   buildUserTag(tag, &user),
			Account: serial.ToTypedMessage(trojanAccount),
		}
	}
	return users
}

func buildSSUser(userInfo *[]api.UserInfo, tag string, panelType string, defaultMethod string) []*protocol.User {
	users := make([]*protocol.User, 0, len(*userInfo))

	for _, user := range *userInfo {
		method := user.Method
		if method == "" {
			if defaultMethod != "Shadowsocks" && defaultMethod != "" {
				method = defaultMethod
			} else {
				method = "aes-256-gcm"
			}
		}
		if C.Contains(shadowaead_2022.List, strings.ToLower(method)) {
			e := buildUserTag(tag, &user)
			userKey, err := checkShadowsocksPassword(user.Passwd, method, panelType)
			if err != nil {
				errors.LogError(context.Background(), "[UID: %d] %s", user.UID, err)
				continue
			}
			users = append(users, &protocol.User{
				Level: 0,
				Email: e,
				Account: serial.ToTypedMessage(&shadowsocks_2022.Account{
					Key: userKey,
				}),
			})
		} else {
			users = append(users, &protocol.User{
				Level: 0,
				Email: buildUserTag(tag, &user),
				Account: serial.ToTypedMessage(&shadowsocks.Account{
					Password:   user.Passwd,
					CipherType: cipherFromString(method),
				}),
			})
		}
	}
	return users
}

func buildSSPluginUser(userInfo *[]api.UserInfo, tag string, panelType string) []*protocol.User {
	users := make([]*protocol.User, 0, len(*userInfo))

	for _, user := range *userInfo {
		if C.Contains(shadowaead_2022.List, strings.ToLower(user.Method)) {
			e := buildUserTag(tag, &user)
			userKey, err := checkShadowsocksPassword(user.Passwd, user.Method, panelType)
			if err != nil {
				errors.LogError(context.Background(), "[UID: %d] %s", user.UID, err)
				continue
			}
			users = append(users, &protocol.User{
				Level: 0,
				Email: e,
				Account: serial.ToTypedMessage(&shadowsocks_2022.Account{
					Key: userKey,
				}),
			})
		} else {
			cypherMethod := cipherFromString(user.Method)
			if _, ok := AEADMethod[cypherMethod]; ok {
				users = append(users, &protocol.User{
					Level: 0,
					Email: buildUserTag(tag, &user),
					Account: serial.ToTypedMessage(&shadowsocks.Account{
						Password:   user.Passwd,
						CipherType: cypherMethod,
					}),
				})
			}
		}
	}
	return users
}

func cipherFromString(c string) shadowsocks.CipherType {
	switch strings.ToLower(c) {
	case "aes-128-gcm", "aead_aes_128_gcm":
		return shadowsocks.CipherType_AES_128_GCM
	case "aes-256-gcm", "aead_aes_256_gcm":
		return shadowsocks.CipherType_AES_256_GCM
	case "chacha20-poly1305", "aead_chacha20_poly1305", "chacha20-ietf-poly1305":
		return shadowsocks.CipherType_CHACHA20_POLY1305
	case "none", "plain":
		return shadowsocks.CipherType_NONE
	default:
		return shadowsocks.CipherType_UNKNOWN
	}
}

func buildUserTag(tag string, user *api.UserInfo) string {
	return fmt.Sprintf("%s|%s|%d", tag, user.Email, user.UID)
}

func checkShadowsocksPassword(password string, method string, panelType string) (string, error) {
	if strings.Contains(panelType, "V2board") {
		var userKey string
		if len(password) < 16 {
			return "", fmt.Errorf("shadowsocks2022 key's length must be greater than 16")
		}
		if method == "2022-blake3-aes-128-gcm" {
			userKey = password[:16]
		} else {
			if len(password) < 32 {
				return "", fmt.Errorf("shadowsocks2022 key's length must be greater than 32")
			}
			userKey = password[:32]
		}
		return base64.StdEncoding.EncodeToString([]byte(userKey)), nil
	} else {
		return password, nil
	}
}
