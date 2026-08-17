package operation_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetMonitorSetting_ChannelTestEnabledEnvOverridesEnabledConfig(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "false")
	t.Setenv("CHANNEL_TEST_FREQUENCY", "5")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: true,
		AutoTestChannelMinutes: 20,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.False(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(5), setting.AutoTestChannelMinutes)
}

func TestGetMonitorSetting_ChannelTestEnabledEnvCanEnableDisabledConfig(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "true")
	monitorSetting = MonitorSetting{
		AutoTestChannelEnabled: false,
		AutoTestChannelMinutes: 12,
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.True(t, setting.AutoTestChannelEnabled)
	assert.Equal(t, float64(12), setting.AutoTestChannelMinutes)
}

func TestGetMonitorSettingPreservesAutoBanOnlyMode(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("CHANNEL_TEST_ENABLED", "")
	t.Setenv("CHANNEL_TEST_FREQUENCY", "")
	monitorSetting = MonitorSetting{ChannelTestMode: ChannelTestModeAutoBanOnly}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.Equal(t, ChannelTestModeAutoBanOnly, setting.ChannelTestMode)
}

func TestGetMonitorSetting_EmailNotifyEnvOverrides(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("EMAIL_NOTIFY_ENABLED", "true")
	t.Setenv("EMAIL_NOTIFY_RECIPIENTS", "ops@example.com;dev@example.com")
	monitorSetting = MonitorSetting{
		EmailNotifyEnabled: false,
		EmailRecipients:    "",
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.True(t, setting.EmailNotifyEnabled)
	assert.Equal(t, "ops@example.com;dev@example.com", setting.EmailRecipients)
}

func TestGetMonitorSetting_EmailNotifyInvalidEnvIgnored(t *testing.T) {
	orig := monitorSetting
	t.Cleanup(func() { monitorSetting = orig })

	t.Setenv("EMAIL_NOTIFY_ENABLED", "not-a-bool")
	monitorSetting = MonitorSetting{
		EmailNotifyEnabled: true,
		EmailRecipients:    "ops@example.com",
	}

	setting := GetMonitorSetting()

	require.NotNil(t, setting)
	assert.True(t, setting.EmailNotifyEnabled, "invalid env value must be ignored")
	assert.Equal(t, "ops@example.com", setting.EmailRecipients)
}
