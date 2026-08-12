package service

import (
	"strconv"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelFailKey(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		assert.Equal(t, "42:default:gpt-4", channelFailKey(42, "default", "gpt-4", ""))
	})
	t.Run("multi key", func(t *testing.T) {
		assert.Equal(t, "42:default:gpt-4:sk-abc123", channelFailKey(42, "default", "gpt-4", "sk-abc123"))
	})
}

func TestHandleChannelFailure_Disabled(t *testing.T) {
	// Save and restore the global setting
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = false
	orig.ChannelFailureThreshold = 5

	chErr := types.NewChannelError(1, 1, "test-channel", false, "", true)
	HandleChannelFailure(*chErr, "default", "test-model", "test error")

	state := GetChannelFailureState(1, "default", "test-model", "")
	assert.Nil(t, state, "disabled monitor should not track failures")
}

func TestHandleChannelFailure_ThresholdZero(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 0

	chErr := types.NewChannelError(2, 1, "test-channel", false, "", true)
	HandleChannelFailure(*chErr, "default", "test-model", "test error")

	state := GetChannelFailureState(2, "default", "test-model", "")
	assert.Nil(t, state, "threshold of 0 should not track failures")
}

func TestHandleChannelFailure_CountsCorrectly(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 5
	orig.ChannelFailureCooldownMinutes = 0

	channelId := 100
	defer ResetChannelFailure(channelId, "default", "test-model", "")

	chErr := types.NewChannelError(channelId, 1, "counting-channel", false, "", true)

	// First 3 failures: threshold not reached
	for i := 0; i < 3; i++ {
		HandleChannelFailure(*chErr, "default", "test-model", "error "+strconv.Itoa(i+1))
	}

	state := GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.Equal(t, 3, state.count)
	assert.False(t, state.alerted, "should not alert below threshold")
}

func TestHandleChannelFailure_ThresholdCrossing(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 3
	orig.ChannelFailureCooldownMinutes = 0

	channelId := 200
	defer ResetChannelFailure(channelId, "default", "test-model", "")

	chErr := types.NewChannelError(channelId, 1, "threshold-channel", false, "", true)

	// First 2 failures
	HandleChannelFailure(*chErr, "default", "test-model", "error 1")
	HandleChannelFailure(*chErr, "default", "test-model", "error 2")

	state := GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.Equal(t, 2, state.count)
	assert.False(t, state.alerted)

	// 3rd failure crosses threshold — should alert
	HandleChannelFailure(*chErr, "default", "test-model", "error 3")

	state = GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.Equal(t, 3, state.count)
	assert.True(t, state.alerted, "should alert at threshold")
}

func TestHandleChannelFailure_AlertDedup(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 2
	orig.ChannelFailureCooldownMinutes = 0

	channelId := 300
	defer ResetChannelFailure(channelId, "default", "test-model", "")

	chErr := types.NewChannelError(channelId, 1, "dedup-channel", false, "", true)

	// Cross threshold
	HandleChannelFailure(*chErr, "default", "test-model", "error 1")
	HandleChannelFailure(*chErr, "default", "test-model", "error 2")

	state := GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.True(t, state.alerted)

	// More failures — should stay alerted but count climbs
	HandleChannelFailure(*chErr, "default", "test-model", "error 3")
	HandleChannelFailure(*chErr, "default", "test-model", "error 4")

	state = GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.Equal(t, 4, state.count)
	assert.True(t, state.alerted, "should remain alerted with no cooldown")
}

func TestHandleChannelFailure_CooldownReAlert(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 2
	orig.ChannelFailureCooldownMinutes = 0 // immediate re-alert

	channelId := 400
	defer ResetChannelFailure(channelId, "default", "test-model", "")

	chErr := types.NewChannelError(channelId, 1, "cooldown-channel", false, "", true)

	// First alert
	HandleChannelFailure(*chErr, "default", "test-model", "error 1")
	HandleChannelFailure(*chErr, "default", "test-model", "error 2")

	state := GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.True(t, state.alerted)

	// With cooldown 0, next threshold hit should re-alert
	HandleChannelFailure(*chErr, "default", "test-model", "error 3")
	HandleChannelFailure(*chErr, "default", "test-model", "error 4")

	state = GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.Equal(t, 4, state.count)
}

func TestResetChannelFailure(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 2
	orig.ChannelFailureCooldownMinutes = 0

	channelId := 500
	chErr := types.NewChannelError(channelId, 1, "reset-channel", false, "", true)

	// Cross threshold
	HandleChannelFailure(*chErr, "default", "test-model", "error 1")
	HandleChannelFailure(*chErr, "default", "test-model", "error 2")

	state := GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.True(t, state.alerted)

	// Reset
	ResetChannelFailure(channelId, "default", "test-model", "")

	state = GetChannelFailureState(channelId, "default", "test-model", "")
	assert.Nil(t, state, "reset should remove the entry")

	// After reset, new failures start from scratch and should re-alert
	HandleChannelFailure(*chErr, "default", "test-model", "new error 1")
	HandleChannelFailure(*chErr, "default", "test-model", "new error 2")

	state = GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.Equal(t, 2, state.count)
	assert.True(t, state.alerted, "should re-alert after reset")
}

func TestHandleChannelFailure_MultiKeyIsolation(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 2
	orig.ChannelFailureCooldownMinutes = 0

	channelId := 600
	defer ResetChannelFailure(channelId, "default", "test-model", "key-a")
	defer ResetChannelFailure(channelId, "default", "test-model", "key-b")

	chErrA := types.NewChannelError(channelId, 1, "multi-key", false, "key-a", true)
	chErrB := types.NewChannelError(channelId, 1, "multi-key", false, "key-b", true)

	// Key A crosses threshold
	HandleChannelFailure(*chErrA, "default", "test-model", "error a1")
	HandleChannelFailure(*chErrA, "default", "test-model", "error a2")

	stateA := GetChannelFailureState(channelId, "default", "test-model", "key-a")
	require.NotNil(t, stateA)
	assert.True(t, stateA.alerted)

	// Key B should be independent
	stateB := GetChannelFailureState(channelId, "default", "test-model", "key-b")
	assert.Nil(t, stateB, "key-b should be independent")

	// Key B starts failing
	HandleChannelFailure(*chErrB, "default", "test-model", "error b1")
	stateB = GetChannelFailureState(channelId, "default", "test-model", "key-b")
	require.NotNil(t, stateB)
	assert.Equal(t, 1, stateB.count)
	assert.False(t, stateB.alerted)
}

func TestHandleChannelFailure_ConcurrentIncrements(t *testing.T) {
	orig := operation_setting.GetMonitorSetting()
	defer func() { *orig = operation_setting.MonitorSetting{} }()

	orig.ChannelFailureMonitorEnabled = true
	orig.ChannelFailureThreshold = 100 // high threshold so we only test counting
	orig.ChannelFailureCooldownMinutes = 0

	channelId := 700
	defer ResetChannelFailure(channelId, "default", "test-model", "")

	chErr := types.NewChannelError(channelId, 1, "concurrent-channel", false, "", true)

	const goroutines = 50
	const incrementsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < incrementsPerGoroutine; j++ {
				HandleChannelFailure(*chErr, "default", "test-model", "concurrent error")
			}
		}()
	}
	wg.Wait()

	state := GetChannelFailureState(channelId, "default", "test-model", "")
	require.NotNil(t, state)
	assert.Equal(t, goroutines*incrementsPerGoroutine, state.count,
		"concurrent increments should not lose counts")
}

func TestGetChannelFailureState_Unknown(t *testing.T) {
	state := GetChannelFailureState(99999, "default", "test-model", "")
	assert.Nil(t, state, "unknown channel should return nil")
}
