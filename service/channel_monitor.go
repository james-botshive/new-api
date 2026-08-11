package service

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/bytedance/gopkg/util/gopool"
)

// channelFailureStore tracks consecutive upstream failures per channel.
// Key is "channelId" or "channelId:usingKey" for multi-key channels.
// Values are *channelFailureEntry; each entry has its own mutex so
// concurrent relays on the same channel cannot double-alert.
var channelFailureStore sync.Map

type channelFailureEntry struct {
	mu          sync.Mutex
	count       int
	alerted     bool
	lastAlertAt time.Time
	channelName string
	lastError   string
}

// channelFailKey mirrors the multi-key semantics used by DisableChannel:
// multi-key channels are tracked per (channelId, usingKey) so one bad key
// does not suppress alerts for healthy keys on the same channel.
func channelFailKey(channelId int, usingKey string) string {
	if usingKey == "" {
		return strconv.Itoa(channelId)
	}
	return fmt.Sprintf("%d:%s", channelId, usingKey)
}

// HandleChannelFailure increments the consecutive-failure counter for a channel.
// When the counter crosses the configured threshold and the channel is not
// already alerted (or the cooldown has elapsed), it fires a notification
// to the root user asynchronously.
func HandleChannelFailure(channelError types.ChannelError, lastError string) {
	setting := operation_setting.GetMonitorSetting()
	if !setting.ChannelFailureMonitorEnabled || setting.ChannelFailureThreshold <= 0 {
		return
	}

	key := channelFailKey(channelError.ChannelId, channelError.UsingKey)
	val, _ := channelFailureStore.LoadOrStore(key, &channelFailureEntry{
		channelName: channelError.ChannelName,
	})
	entry := val.(*channelFailureEntry)

	entry.mu.Lock()
	entry.count++
	entry.lastError = lastError
	count := entry.count

	shouldNotify := false
	if count >= setting.ChannelFailureThreshold {
		if !entry.alerted {
			shouldNotify = true
		} else if setting.ChannelFailureCooldownMinutes > 0 &&
			time.Since(entry.lastAlertAt) >= time.Duration(setting.ChannelFailureCooldownMinutes)*time.Minute {
			// Periodic re-alert while channel keeps failing
			shouldNotify = true
		}
	}
	if shouldNotify {
		entry.alerted = true
		entry.lastAlertAt = time.Now()
	}
	entry.mu.Unlock()

	if !shouldNotify {
		return
	}

	// Fire notification asynchronously (same pattern as DisableChannel)
	entryCopy := entry
	gopool.Go(func() {
		notifyType := fmt.Sprintf("%s_%d", dto.NotifyTypeChannelFail, channelError.ChannelId)
		subject := fmt.Sprintf("通道「%s」（#%d）连续失败 %d 次",
			channelError.ChannelName, channelError.ChannelId, entryCopy.count)
		content := fmt.Sprintf("通道「%s」（#%d）连续失败 %d 次（阈值 %d）\n最后一次错误：%s",
			channelError.ChannelName, channelError.ChannelId, entryCopy.count,
			setting.ChannelFailureThreshold, common.LocalLogPreview(entryCopy.lastError))
		NotifyRootUser(notifyType, subject, content)
	})
}

// ResetChannelFailure clears the counter and alerted state for a channel.
// Called on a successful relay request and when a channel is manually re-enabled.
func ResetChannelFailure(channelId int, usingKey string) {
	channelFailureStore.Delete(channelFailKey(channelId, usingKey))
}

// GetChannelFailureState returns the current failure state for a channel,
// or nil if no failures have been recorded. Exported for admin/diagnostics.
func GetChannelFailureState(channelId int, usingKey string) *channelFailureEntry {
	key := channelFailKey(channelId, usingKey)
	if val, ok := channelFailureStore.Load(key); ok {
		entry := val.(*channelFailureEntry)
		entry.mu.Lock()
		defer entry.mu.Unlock()
		// Return a copy to avoid races
		copy := *entry
		return &copy
	}
	return nil
}
