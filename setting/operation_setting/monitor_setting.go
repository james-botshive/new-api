package operation_setting

import (
	"os"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes float64 `json:"auto_test_channel_minutes"`
	ChannelTestMode        string  `json:"channel_test_mode"`

	// Resource monitor — per-channel consecutive failure alert
	ChannelFailureMonitorEnabled bool   `json:"channel_failure_monitor_enabled"`
	ChannelFailureThreshold      int    `json:"channel_failure_threshold"`
	ChannelFailureCooldownMinutes int    `json:"channel_failure_cooldown_minutes"`

	// WeChat iLink Bot notification config
	WechatBotBaseURL        string `json:"wechat_bot_base_url"`
	WechatBotToken          string `json:"wechat_bot_token"`
	WechatBotAppId          string `json:"wechat_bot_app_id"`
	WechatBotClientVersion  string `json:"wechat_bot_client_version"`
	WechatBotChannelVersion string `json:"wechat_bot_channel_version"`
	WechatBotAgent          string `json:"wechat_bot_agent"`
	WechatBotToUserId       string `json:"wechat_bot_to_user_id"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:         false,
	AutoTestChannelMinutes:         10,
	ChannelTestMode:                ChannelTestModeScheduledAll,
	ChannelFailureMonitorEnabled:   false,
	ChannelFailureThreshold:        5,
	ChannelFailureCooldownMinutes:  10,
	WechatBotBaseURL:               "",
	WechatBotToken:                 "",
	WechatBotAppId:                 "",
	WechatBotClientVersion:         "",
	WechatBotChannelVersion:        "1.0.0",
	WechatBotAgent:                 "NewAPI/1.0",
	WechatBotToUserId:              "",
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	// Sync from common.OptionMap so admin UI changes take effect immediately.
	// The LoadFromDB call handles the prefix stripping (monitor_setting.xxx → xxx)
	// and sets struct fields via reflection.
	common.OptionMapRWMutex.RLock()
	_ = config.GlobalConfig.LoadFromDB(common.OptionMap)
	common.OptionMapRWMutex.RUnlock()

	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	switch monitorSetting.ChannelTestMode {
	case ChannelTestModeAutoBanOnly, ChannelTestModePassiveRecovery:
	default:
		monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
	}

	// Resource monitor env overrides
	if v := os.Getenv("CHANNEL_FAILURE_MONITOR_ENABLED"); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err == nil {
			monitorSetting.ChannelFailureMonitorEnabled = parsed
		}
	}
	if v := os.Getenv("CHANNEL_FAILURE_THRESHOLD"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil && parsed > 0 {
			monitorSetting.ChannelFailureThreshold = parsed
		}
	}
	if v := os.Getenv("CHANNEL_FAILURE_COOLDOWN_MINUTES"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err == nil && parsed > 0 {
			monitorSetting.ChannelFailureCooldownMinutes = parsed
		}
	}

	// WeChat bot env overrides
	if v := os.Getenv("WECHAT_BOT_BASE_URL"); v != "" {
		monitorSetting.WechatBotBaseURL = v
	}
	if v := os.Getenv("WECHAT_BOT_TOKEN"); v != "" {
		monitorSetting.WechatBotToken = v
	}
	if v := os.Getenv("WECHAT_BOT_APP_ID"); v != "" {
		monitorSetting.WechatBotAppId = v
	}
	if v := os.Getenv("WECHAT_BOT_CLIENT_VERSION"); v != "" {
		monitorSetting.WechatBotClientVersion = v
	}
	if v := os.Getenv("WECHAT_BOT_CHANNEL_VERSION"); v != "" {
		monitorSetting.WechatBotChannelVersion = v
	}
	if v := os.Getenv("WECHAT_BOT_AGENT"); v != "" {
		monitorSetting.WechatBotAgent = v
	}
	if v := os.Getenv("WECHAT_BOT_TO_USER_ID"); v != "" {
		monitorSetting.WechatBotToUserId = v
	}

	return &monitorSetting
}
