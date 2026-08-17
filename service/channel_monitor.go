package service

import (
	"fmt"
	"html"
	"strings"
	"sync"
	"time"
	"unicode"

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

// channelFailKey builds a unique key for channel→group→model granularity.
// Format: channelId:group:modelName[:usingKey]
func channelFailKey(channelId int, group, modelName, usingKey string) string {
	base := fmt.Sprintf("%d:%s:%s", channelId, group, modelName)
	if usingKey != "" {
		base += ":" + usingKey
	}
	return base
}

// normalizeEmailRecipients parses a recipient list separated by commas,
// semicolons, or whitespace, deduplicates entries, and joins them with ";"
// — the separator common.SendEmail expects.
func normalizeEmailRecipients(raw string) string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})
	seen := make(map[string]struct{}, len(parts))
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		result = append(result, p)
	}
	return strings.Join(result, ";")
}

// HandleChannelFailure increments the consecutive-failure counter for a
// specific channel+group+model combination. When the counter crosses the
// configured threshold, it fires a WeChat notification to the root user.
func HandleChannelFailure(channelError types.ChannelError, group, modelName, lastError string) {
	setting := operation_setting.GetMonitorSetting()
	if !setting.ChannelFailureMonitorEnabled || setting.ChannelFailureThreshold <= 0 {
		return
	}
	if modelName == "" {
		modelName = "unknown"
	}
	if group == "" {
		group = "default"
	}

	key := channelFailKey(channelError.ChannelId, group, modelName, channelError.UsingKey)
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

	// Fire notification asynchronously
	entryCopy := entry
	gopool.Go(func() {
		notifyType := fmt.Sprintf("%s_%d_%s_%s", dto.NotifyTypeChannelFail, channelError.ChannelId, group, modelName)
		subject := fmt.Sprintf("渠道 %s(#%d) 分组 %s 模型 %s 连续失败 %d 次",
			channelError.ChannelName, channelError.ChannelId, group, modelName, entryCopy.count)
		content := fmt.Sprintf("渠道：%s(#%d)\n分组：%s\n模型：%s\n连续失败：%d 次（阈值 %d）\n最后错误：%s",
			channelError.ChannelName, channelError.ChannelId,
			group, modelName,
			entryCopy.count, setting.ChannelFailureThreshold,
			common.LocalLogPreview(entryCopy.lastError))
		NotifyRootUser(notifyType, subject, content)

		// Email channel — independent of WeChat; both can fire on the same alert.
		emailSetting := operation_setting.GetMonitorSetting()
		if emailSetting.EmailNotifyEnabled {
			recipients := normalizeEmailRecipients(emailSetting.EmailRecipients)
			if recipients != "" {
				body := "<pre>" + html.EscapeString(content) + "</pre>"
				if err := common.SendEmail(subject, recipients, body); err != nil {
					common.SysLog(fmt.Sprintf("failed to send channel-failure email: %s", err.Error()))
				}
			}
		}
	})
}

// ResetChannelFailure clears the counter and alerted state for a channel+group+model.
func ResetChannelFailure(channelId int, group, modelName, usingKey string) {
	channelFailureStore.Delete(channelFailKey(channelId, group, modelName, usingKey))
}

// GetChannelFailureState returns the current failure state, or nil if none.
func GetChannelFailureState(channelId int, group, modelName, usingKey string) *channelFailureEntry {
	key := channelFailKey(channelId, group, modelName, usingKey)
	if val, ok := channelFailureStore.Load(key); ok {
		entry := val.(*channelFailureEntry)
		entry.mu.Lock()
		defer entry.mu.Unlock()
		copy := *entry
		return &copy
	}
	return nil
}
