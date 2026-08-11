package controller

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// ---- iLink QR login (same protocol as openclaw weixin plugin) ----

const (
	ilinkFixedBaseURL = "https://ilinkai.weixin.qq.com"
	ilinkBotType      = "3"
	qrPollTimeoutSec  = 30
	qrSessionTTL      = 5 * time.Minute
)

type qrSession struct {
	qrcode    string
	qrcodeURL string
	createdAt time.Time
}

var (
	qrSessions   = map[string]*qrSession{}
	qrSessionsMu sync.Mutex
)

func init() {
	go func() {
		for {
			time.Sleep(time.Minute)
			qrSessionsMu.Lock()
			for k, s := range qrSessions {
				if time.Since(s.createdAt) > qrSessionTTL {
					delete(qrSessions, k)
				}
			}
			qrSessionsMu.Unlock()
		}
	}()
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// ---- iLink API helpers ----

type qrCodeResp struct {
	Qrcode          string `json:"qrcode"`
	QrcodeImgContent string `json:"qrcode_img_content"`
}

type qrStatusResp struct {
	Status      string `json:"status"`
	BotToken    string `json:"bot_token,omitempty"`
	IlinkBotId  string `json:"ilink_bot_id,omitempty"`
	BaseURL     string `json:"baseurl,omitempty"`
	IlinkUserId string `json:"ilink_user_id,omitempty"`
}

func ilinkPost(endpoint string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", ilinkFixedBaseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func ilinkGet(endpoint string) ([]byte, error) {
	req, err := http.NewRequest("GET", ilinkFixedBaseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 35 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ---- API handlers ----

// GetWeChatQRCode generates a login QR code for the admin to scan.
// The returned QR code URL embeds the system's bot token so that after scanning,
// the server can resolve the admin's WeChat user ID.
func GetWeChatQRCode(c *gin.Context) {
	setting := operation_setting.GetMonitorSetting()

	token := setting.WechatBotToken
	baseURL := setting.WechatBotBaseURL

	if token == "" || baseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "系统微信 Bot 尚未配置，请先在系统设置中配置 WeChat Bot Token 和 Base URL",
		})
		return
	}

	// Call iLink get_bot_qrcode with our bot token in local_token_list
	reqBody, _ := json.Marshal(map[string]interface{}{
		"local_token_list": []string{token},
	})
	respBody, err := ilinkPost("/ilink/bot/get_bot_qrcode?bot_type="+ilinkBotType, reqBody)
	if err != nil {
		common.SysLog("wechat qr: get_bot_qrcode failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "生成二维码失败: " + err.Error(),
		})
		return
	}

	var qrResp qrCodeResp
	if err := json.Unmarshal(respBody, &qrResp); err != nil {
		common.SysLog("wechat qr: parse qrcode response failed: " + err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "解析二维码响应失败",
		})
		return
	}

	if qrResp.Qrcode == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取二维码失败，服务端返回空",
		})
		return
	}

	sessionKey := randomHex(16)
	qrSessionsMu.Lock()
	qrSessions[sessionKey] = &qrSession{
		qrcode:    qrResp.Qrcode,
		qrcodeURL: qrResp.QrcodeImgContent,
		createdAt: time.Now(),
	}
	qrSessionsMu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_key": sessionKey,
		"qrcode_url": qrResp.QrcodeImgContent,
	})
}

// PollWeChatQRStatus polls the QR code scan status.
// Returns status and the WeChat user ID on success.
func PollWeChatQRStatus(c *gin.Context) {
	sessionKey := c.Query("session_key")
	if sessionKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少 session_key",
		})
		return
	}

	qrSessionsMu.Lock()
	sess := qrSessions[sessionKey]
	if sess == nil {
		qrSessionsMu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "会话不存在或已过期",
		})
		return
	}
	qrcode := sess.qrcode
	qrSessionsMu.Unlock()

	respBody, err := ilinkGet("/ilink/bot/get_qrcode_status?qrcode=" + qrcode)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    map[string]string{"status": "wait"},
		})
		return
	}

	var statusResp qrStatusResp
	if err := json.Unmarshal(respBody, &statusResp); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    map[string]string{"status": "wait"},
		})
		return
	}

	result := map[string]string{"status": statusResp.Status}
	if statusResp.Status == "confirmed" && statusResp.IlinkUserId != "" {
		result["wechat_user_id"] = statusResp.IlinkUserId

		// Save the fresh bot_token from the QR login
		if statusResp.BotToken != "" {
			model.UpdateOption("monitor_setting.wechat_bot_token", statusResp.BotToken)
		}

		// Auto-bind: save to current user's settings
		userId := c.GetInt("id")
		if userId > 0 {
			user, err := model.GetUserById(userId, false)
			if err == nil {
				setting := user.GetSetting()
				setting.WeChatUserId = statusResp.IlinkUserId
				if setting.NotifyType == "" || setting.NotifyType == "email" {
					setting.NotifyType = "wechat"
				}
				_ = model.UpdateUserSetting(userId, setting)
				result["bound"] = "true"
			}
		}

		// Clean up session
		qrSessionsMu.Lock()
		delete(qrSessions, sessionKey)
		qrSessionsMu.Unlock()
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

// ---- legacy API (for the previous context-tokens approach, kept for reference) ----

// WeChatBotUser represents a WeChat user who has interacted with the iLink bot.
type WeChatBotUser struct {
	WeChatUserId string `json:"wechat_user_id"`
}

// GetWeChatBotUsers returns available WeChat user IDs for binding.
// Users are discovered via the QR login flow (GetWeChatQRCode / PollWeChatQRStatus).
func GetWeChatBotUsers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    []WeChatBotUser{},
	})
}

// BindWeChatUser binds a WeChat user ID to the current user's notification settings.
type BindWeChatRequest struct {
	WeChatUserId string `json:"wechat_user_id"`
}

func BindWeChatUser(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "未登录",
		})
		return
	}

	var req BindWeChatRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil || req.WeChatUserId == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请提供微信用户ID",
		})
		return
	}

	user, err := model.GetUserById(userId, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "获取用户信息失败",
		})
		return
	}

	setting := user.GetSetting()
	setting.WeChatUserId = req.WeChatUserId
	if setting.NotifyType == "" || setting.NotifyType == "email" {
		setting.NotifyType = "wechat"
	}

	if err := model.UpdateUserSetting(userId, setting); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "保存设置失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "微信用户绑定成功",
	})
}
