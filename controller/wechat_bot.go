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
	qrcode      string
	qrcodeURL   string
	createdAt   time.Time
	registering bool // true when the QR was requested without a configured token
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

// ilinkHTTPClient is a package-level seam so tests can point the iLink calls
// at a stub server instead of the real ilinkai.weixin.qq.com host.
var ilinkHTTPClient = &http.Client{Timeout: 35 * time.Second}

type qrCodeResp struct {
	Qrcode           string `json:"qrcode"`
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
	resp, err := ilinkHTTPClient.Do(req)
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
	resp, err := ilinkHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ---- API handlers ----

// GetWeChatQRCode generates a QR code for WeChat Bot registration or binding.
//
// With a configured token the QR binds the scanning user to the existing bot
// (any authenticated user may bind). Without a token the QR is a registration
// QR — iLink returns a fresh bot_token/baseurl on confirmation, which
// PollWeChatQRStatus persists. Only admins may start a registration, since it
// writes global settings.
func GetWeChatQRCode(c *gin.Context) {
	setting := operation_setting.GetMonitorSetting()

	token := setting.WechatBotToken

	if token == "" && c.GetInt("role") < common.RoleAdminUser {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "仅管理员可注册微信 Bot",
		})
		return
	}

	// An empty local_token_list makes iLink treat the QR as a bot registration
	// (credentials arrive via the status endpoint); a populated list binds the
	// scanner to the existing bot instead.
	localTokenList := []string{}
	if token != "" {
		localTokenList = []string{token}
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"local_token_list": localTokenList,
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
		qrcode:      qrResp.Qrcode,
		qrcodeURL:   qrResp.QrcodeImgContent,
		createdAt:   time.Now(),
		registering: token == "",
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
	registering := sess.registering
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

		// A registration QR must return the bot credentials; without them the
		// registration failed. Surface it and drop the session so polling
		// stops instead of binding the user to a nonexistent bot.
		if registering && statusResp.BotToken == "" {
			result["error"] = "服务器未返回 bot_token，请重新扫码注册"
			qrSessionsMu.Lock()
			delete(qrSessions, sessionKey)
			qrSessionsMu.Unlock()
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"data":    result,
			})
			return
		}

		// Persist the fresh credentials from the QR login. Global settings may
		// only be written by admins — a non-admin binding scan must never
		// overwrite the system bot token.
		if c.GetInt("role") >= common.RoleAdminUser && statusResp.BotToken != "" {
			baseURL := statusResp.BaseURL
			if baseURL == "" {
				baseURL = ilinkFixedBaseURL
			}
			err := model.UpdateOptionsBulk(map[string]string{
				"monitor_setting.wechat_bot_token":    statusResp.BotToken,
				"monitor_setting.wechat_bot_base_url": baseURL,
				"monitor_setting.wechat_bot_id":       statusResp.IlinkBotId,
			})
			if err != nil {
				common.SysLog("wechat qr: save bot credentials failed: " + err.Error())
				result["error"] = "保存微信 Bot 配置失败"
			}
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
