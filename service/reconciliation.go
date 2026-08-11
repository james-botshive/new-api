package service

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// ReconItem holds aggregated usage stats for one model (personal view)
// or one user+model (admin view).
type ReconItem struct {
	Username           string `json:"username,omitempty"`
	ModelName          string `json:"model_name"`
	Count              int    `json:"count"`
	PromptTokens       int    `json:"prompt_tokens"`
	CompletionTokens   int    `json:"completion_tokens"`
	CacheHitTokens     int    `json:"cache_hit_tokens"`
	CacheWrite5mTokens int    `json:"cache_write_5m_tokens"`
	CacheWrite1hTokens int    `json:"cache_write_1h_tokens"`
	CacheWriteTokens   int    `json:"cache_write_tokens"`
	Quota              int    `json:"quota"`
}

// ReconResult is the full reconciliation response.
type ReconResult struct {
	Items []ReconItem `json:"items"`
	Total ReconItem   `json:"total"`
}

// GetPersonalReconciliation returns current user's consume logs aggregated by model.
func GetPersonalReconciliation(userId int, startTs, endTs int64, modelName string) (*ReconResult, error) {
	logs, _, err := model.GetUserLogs(userId, model.LogTypeConsume, startTs, endTs, modelName, "", 0, 10000, "", "", "")
	if err != nil {
		return nil, err
	}
	return aggregateLogs(logs, false), nil
}

// GetAdminReconciliation returns all users' consume logs aggregated by user+model.
func GetAdminReconciliation(startTs, endTs int64, modelName, username string) (*ReconResult, error) {
	logs, _, err := model.GetAllLogs(model.LogTypeConsume, startTs, endTs, modelName, username, "", 0, 10000, 0, "", "", "")
	if err != nil {
		return nil, err
	}
	return aggregateLogs(logs, true), nil
}

func aggregateLogs(logs []*model.Log, admin bool) *ReconResult {
	result := &ReconResult{}
	grouped := map[string]*ReconItem{}

	for _, log := range logs {
		key := log.ModelName
		if admin {
			key = log.Username + "\x00" + log.ModelName
		}

		item, ok := grouped[key]
		if !ok {
			item = &ReconItem{}
			if admin {
				item.Username = log.Username
			}
			item.ModelName = log.ModelName
			grouped[key] = item
		}

		item.Count++
		item.PromptTokens += log.PromptTokens
		item.CompletionTokens += log.CompletionTokens
		item.Quota += log.Quota

		// Parse cache tokens from the `other` JSON field
		otherMap, _ := common.StrToMap(log.Other)
		item.CacheHitTokens += intFromMap(otherMap, "cache_tokens")
		item.CacheWrite5mTokens += intFromMap(otherMap, "cache_creation_tokens_5m")
		item.CacheWrite1hTokens += intFromMap(otherMap, "cache_creation_tokens_1h")
		item.CacheWriteTokens += intFromMap(otherMap, "cache_write_tokens")
	}

	for _, item := range grouped {
		result.Items = append(result.Items, *item)
		result.Total.Count += item.Count
		result.Total.PromptTokens += item.PromptTokens
		result.Total.CompletionTokens += item.CompletionTokens
		result.Total.CacheHitTokens += item.CacheHitTokens
		result.Total.CacheWrite5mTokens += item.CacheWrite5mTokens
		result.Total.CacheWrite1hTokens += item.CacheWrite1hTokens
		result.Total.CacheWriteTokens += item.CacheWriteTokens
		result.Total.Quota += item.Quota
	}

	return result
}

func intFromMap(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}
