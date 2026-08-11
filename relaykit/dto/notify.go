package dto

type Notify struct {
	Type    string        `json:"type"`
	Title   string        `json:"title"`
	Content string        `json:"content"`
	Values  []interface{} `json:"values"`
}

const ContentValueParam = "{{value}}"

const (
	NotifyTypeQuotaExceed     = "quota_exceed"
	NotifyTypeChannelUpdate   = "channel_update"
	NotifyTypeChannelTest     = "channel_test"
	NotifyTypeChannelFail     = "channel_fail"     // 渠道连续失败告警
	NotifyTypeResourceMonitor = "resource_monitor" // 资源监控告警
)

func NewNotify(t string, title string, content string, values []interface{}) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}
