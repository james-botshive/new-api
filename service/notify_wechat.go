package service

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func randomWechatUin() string {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<32))
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(n.String()))
}

// SendWeChatNotify sends a text notification via the iLink Bot API.
// toUserId is the recipient's WeChat user ID (from the admin's wechat_user_id setting).
func SendWeChatNotify(toUserId string, data dto.Notify) error {
	setting := operation_setting.GetMonitorSetting()

	if toUserId == "" {
		return fmt.Errorf("wechat notify skipped: no target WeChat user ID")
	}
	if setting.WechatBotToken == "" {
		return fmt.Errorf("wechat bot not configured: missing token")
	}

	// Process placeholder substitution
	content := data.Content
	for _, value := range data.Values {
		content = strings.Replace(content, dto.ContentValueParam, fmt.Sprintf("%v", value), 1)
	}

	// Build iLink sendmessage payload
	body := fmt.Sprintf(`{"msg":{"to_user_id":"%s","from_user_id":"","client_id":"newapi-%s","message_type":2,"message_state":2,"item_list":[{"type":1,"text_item":{"text":"%s"}}]},"base_info":{"channel_version":"%s","bot_agent":"%s"}}`,
		toUserId,
		randomHex(6),
		escapeJSON(content),
		setting.WechatBotChannelVersion,
		setting.WechatBotAgent,
	)

	baseURL := strings.TrimSuffix(setting.WechatBotBaseURL, "/")
	req, err := http.NewRequest("POST", baseURL+"/ilink/bot/sendmessage", bytes.NewReader([]byte(body)))
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+setting.WechatBotToken)
	req.Header.Set("AuthorizationType", "ilink_bot_token")
	req.Header.Set("X-WECHAT-UIN", randomWechatUin())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		common.SysLog(fmt.Sprintf("[wechat] send failed: %v", err))
		return fmt.Errorf("wechat send failed: %v", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	common.SysLog(fmt.Sprintf("[wechat] send: status=%d body=%s", resp.StatusCode, string(respBody)))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("wechat send failed: HTTP %d - %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}
