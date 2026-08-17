package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// urlRewriteTransport routes every request to the given base URL over plain
// HTTP, so the fixed https://ilinkai.weixin.qq.com URLs used by production
// code land on the httptest stub server instead of the real host.
type urlRewriteTransport struct {
	base *url.URL
}

func (t *urlRewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.URL = &url.URL{
		Scheme:   t.base.Scheme,
		Host:     t.base.Host,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}
	return http.DefaultTransport.RoundTrip(clone)
}

// wechatBotTestStub stands in for the iLink API so tests never hit the real
// ilinkai.weixin.qq.com host.
type wechatBotTestStub struct {
	server         *httptest.Server
	mu             sync.Mutex
	lastQrBody     []byte
	qrResponse     string // JSON body returned by get_bot_qrcode
	statusResponse string // JSON body returned by get_qrcode_status
}

func newWechatBotTestStub(t *testing.T) *wechatBotTestStub {
	t.Helper()
	stub := &wechatBotTestStub{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ilink/bot/get_bot_qrcode":
			body, _ := io.ReadAll(r.Body)
			stub.mu.Lock()
			stub.lastQrBody = body
			stub.mu.Unlock()
			_, _ = w.Write([]byte(stub.qrResponse))
		case "/ilink/bot/get_qrcode_status":
			_, _ = w.Write([]byte(stub.statusResponse))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func (stub *wechatBotTestStub) qrRequestBody(t *testing.T) []byte {
	t.Helper()
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.lastQrBody
}

// setupWeChatBotControllerTest swaps in an in-memory sqlite DB, clears host
// env overrides, and points the iLink client seam at a fresh stub server.
func setupWeChatBotControllerTest(t *testing.T) *wechatBotTestStub {
	t.Helper()
	previousDB := model.DB
	previousType := common.MainDatabaseType()
	previousClient := ilinkHTTPClient
	previousRedisEnabled := common.RedisEnabled
	// The test process never calls InitRedisClient, so RedisEnabled stays
	// true while RDB is nil — force it off so user-cache updates skip Redis.
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}))
	model.DB = db
	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	// common.OptionMap is nil in a bare test process; initialize it so option
	// writes (UpdateOption/UpdateOptionsBulk) work and GetMonitorSetting has
	// the registered defaults to load from.
	model.InitOptionMap()

	// Host env must not leak into GetMonitorSetting.
	for _, key := range []string{
		"WECHAT_BOT_BASE_URL", "WECHAT_BOT_TOKEN", "WECHAT_BOT_APP_ID",
		"WECHAT_BOT_CLIENT_VERSION", "WECHAT_BOT_CHANNEL_VERSION",
		"WECHAT_BOT_AGENT", "WECHAT_BOT_TO_USER_ID",
	} {
		t.Setenv(key, "")
	}

	// Reset QR sessions so tests start from a clean slate.
	qrSessionsMu.Lock()
	qrSessions = map[string]*qrSession{}
	qrSessionsMu.Unlock()

	stub := newWechatBotTestStub(t)
	stubURL, err := url.Parse(stub.server.URL)
	require.NoError(t, err)
	ilinkHTTPClient = &http.Client{
		Timeout:   35 * time.Second,
		Transport: &urlRewriteTransport{base: stubURL},
	}

	t.Cleanup(func() {
		// Reset the shared monitor options back to empty before restoring
		// global state; the next GetMonitorSetting call reloads from the map.
		_ = model.UpdateOption("monitor_setting.wechat_bot_token", "")
		_ = model.UpdateOption("monitor_setting.wechat_bot_base_url", "")
		_ = model.UpdateOption("monitor_setting.wechat_bot_id", "")
		ilinkHTTPClient = previousClient
		model.DB = previousDB
		common.SetMainDatabaseType(previousType)
		common.RedisEnabled = previousRedisEnabled
		qrSessionsMu.Lock()
		qrSessions = map[string]*qrSession{}
		qrSessionsMu.Unlock()
	})
	return stub
}

// seedQRSession installs a QR session directly into the session store.
func seedQRSession(key, qrcode string, registering bool) {
	qrSessionsMu.Lock()
	qrSessions[key] = &qrSession{
		qrcode:      qrcode,
		createdAt:   time.Now(),
		registering: registering,
	}
	qrSessionsMu.Unlock()
}

func sessionExists(key string) bool {
	qrSessionsMu.Lock()
	defer qrSessionsMu.Unlock()
	_, ok := qrSessions[key]
	return ok
}

func newWeChatBotContext(method, path string, role, userId int) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, path, nil)
	if role != 0 {
		c.Set("role", role)
	}
	if userId != 0 {
		c.Set("id", userId)
	}
	return c, recorder
}

type wechatQRResponse struct {
	Success    bool              `json:"success"`
	Message    string            `json:"message"`
	SessionKey string            `json:"session_key"`
	QrcodeURL  string            `json:"qrcode_url"`
	Data       map[string]string `json:"data"`
}

func monitorOption(t *testing.T, key string) string {
	t.Helper()
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	return common.OptionMap[key]
}

func TestGetWeChatQRCodeTokenlessRequiresAdmin(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.qrResponse = `{"qrcode":"q1","qrcode_img_content":"https://example.com/qr"}`

	c, recorder := newWeChatBotContext(http.MethodPost, "/api/user/wechat/bot/qrcode", common.RoleCommonUser, 1)
	GetWeChatQRCode(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
	var resp wechatQRResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
	require.Empty(t, stub.qrRequestBody(t), "non-admin registration must not reach iLink")

	// An admin may start a registration without any configured token.
	c, recorder = newWeChatBotContext(http.MethodPost, "/api/user/wechat/bot/qrcode", common.RoleAdminUser, 1)
	GetWeChatQRCode(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.SessionKey)
	assert.Equal(t, "https://example.com/qr", resp.QrcodeURL)

	var reqBody struct {
		LocalTokenList []string `json:"local_token_list"`
	}
	require.NoError(t, common.Unmarshal(stub.qrRequestBody(t), &reqBody))
	assert.Len(t, reqBody.LocalTokenList, 0, "registration QR must send an empty local_token_list")
}

func TestGetWeChatQRCodeWithTokenAllowsAnyRole(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.qrResponse = `{"qrcode":"q1","qrcode_img_content":"https://example.com/qr"}`
	require.NoError(t, model.UpdateOption("monitor_setting.wechat_bot_token", "tok"))

	c, recorder := newWeChatBotContext(http.MethodPost, "/api/user/wechat/bot/qrcode", common.RoleCommonUser, 1)
	GetWeChatQRCode(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var reqBody struct {
		LocalTokenList []string `json:"local_token_list"`
	}
	require.NoError(t, common.Unmarshal(stub.qrRequestBody(t), &reqBody))
	assert.Equal(t, []string{"tok"}, reqBody.LocalTokenList)
}

func TestGetWeChatQRCodeEmptyQrcodeFromIlink(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.qrResponse = `{"qrcode":""}`

	c, recorder := newWeChatBotContext(http.MethodPost, "/api/user/wechat/bot/qrcode", common.RoleAdminUser, 1)
	GetWeChatQRCode(c)

	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	var resp wechatQRResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.False(t, resp.Success)
}

func TestPollWeChatQRStatusConfirmedSavesCredentials(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.statusResponse = `{"status":"confirmed","bot_token":"newtok","ilink_bot_id":"bid1","baseurl":"https://alt.example","ilink_user_id":"wx-user-1"}`
	require.NoError(t, model.DB.Create(&model.User{Id: 42, Username: "alice"}).Error)
	seedQRSession("sk", "q1", true)

	c, recorder := newWeChatBotContext(http.MethodGet, "/api/user/wechat/bot/qrcode/status?session_key=sk", common.RoleAdminUser, 42)
	PollWeChatQRStatus(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp wechatQRResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, "confirmed", resp.Data["status"])
	assert.Equal(t, "wx-user-1", resp.Data["wechat_user_id"])
	assert.Equal(t, "true", resp.Data["bound"])
	assert.NotContains(t, resp.Data, "error")

	assert.Equal(t, "newtok", monitorOption(t, "monitor_setting.wechat_bot_token"))
	assert.Equal(t, "https://alt.example", monitorOption(t, "monitor_setting.wechat_bot_base_url"))
	assert.Equal(t, "bid1", monitorOption(t, "monitor_setting.wechat_bot_id"))
	assert.False(t, sessionExists("sk"), "session must be removed after confirmation")

	user, err := model.GetUserById(42, false)
	require.NoError(t, err)
	assert.Equal(t, "wx-user-1", user.GetSetting().WeChatUserId)
	assert.Equal(t, "wechat", user.GetSetting().NotifyType)
}

func TestPollWeChatQRStatusBaseURLFallback(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.statusResponse = `{"status":"confirmed","bot_token":"newtok","ilink_bot_id":"bid1","ilink_user_id":"wx-user-1"}`
	require.NoError(t, model.DB.Create(&model.User{Id: 42, Username: "alice"}).Error)
	seedQRSession("sk", "q1", true)

	c, recorder := newWeChatBotContext(http.MethodGet, "/api/user/wechat/bot/qrcode/status?session_key=sk", common.RoleAdminUser, 42)
	PollWeChatQRStatus(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, ilinkFixedBaseURL, monitorOption(t, "monitor_setting.wechat_bot_base_url"))
}

func TestPollWeChatQRStatusNonAdminCannotOverwriteCredentials(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.statusResponse = `{"status":"confirmed","bot_token":"nefarious","ilink_bot_id":"bid-x","baseurl":"https://evil.example","ilink_user_id":"wx-user-2"}`
	require.NoError(t, model.UpdateOption("monitor_setting.wechat_bot_token", "origtok"))
	require.NoError(t, model.DB.Create(&model.User{Id: 7, Username: "bob"}).Error)
	seedQRSession("sk", "q1", false)

	c, recorder := newWeChatBotContext(http.MethodGet, "/api/user/wechat/bot/qrcode/status?session_key=sk", common.RoleCommonUser, 7)
	PollWeChatQRStatus(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp wechatQRResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, "true", resp.Data["bound"], "personal binding stays open to any role")
	assert.Equal(t, "origtok", monitorOption(t, "monitor_setting.wechat_bot_token"), "non-admin must not overwrite global creds")
	assert.Equal(t, "", monitorOption(t, "monitor_setting.wechat_bot_base_url"))
}

func TestPollWeChatQRStatusConfirmedWithoutTokenReportsError(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.statusResponse = `{"status":"confirmed","ilink_user_id":"wx-user-3"}`
	seedQRSession("sk", "q1", true)

	c, recorder := newWeChatBotContext(http.MethodGet, "/api/user/wechat/bot/qrcode/status?session_key=sk", common.RoleAdminUser, 42)
	PollWeChatQRStatus(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp wechatQRResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Data["error"], "registration without a returned token must surface an error")
	assert.NotContains(t, resp.Data, "bound")
	assert.Equal(t, "", monitorOption(t, "monitor_setting.wechat_bot_token"))
	assert.False(t, sessionExists("sk"), "failed registration must drop the session")
}

func TestPollWeChatQRStatusMissingSession(t *testing.T) {
	setupWeChatBotControllerTest(t)

	c, recorder := newWeChatBotContext(http.MethodGet, "/api/user/wechat/bot/qrcode/status?session_key=unknown", common.RoleAdminUser, 1)
	PollWeChatQRStatus(c)

	require.Equal(t, http.StatusNotFound, recorder.Code)
}

func TestPollWeChatQRStatusWaitStatus(t *testing.T) {
	stub := setupWeChatBotControllerTest(t)
	stub.statusResponse = `{"status":"wait"}`
	seedQRSession("sk", "q1", true)

	c, recorder := newWeChatBotContext(http.MethodGet, "/api/user/wechat/bot/qrcode/status?session_key=sk", common.RoleAdminUser, 1)
	PollWeChatQRStatus(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	var resp wechatQRResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &resp))
	assert.Equal(t, "wait", resp.Data["status"])
	assert.True(t, sessionExists("sk"), "session stays alive while waiting for the scan")
}
