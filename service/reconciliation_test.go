package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reconLog builds a consume log with the given billing info. Pass nil other
// to leave the `other` JSON field empty.
func reconLog(t *testing.T, username, modelName string, quota, promptTokens, completionTokens int, other map[string]interface{}) *model.Log {
	t.Helper()
	otherJSON := ""
	if other != nil {
		data, err := common.Marshal(other)
		require.NoError(t, err)
		otherJSON = string(data)
	}
	return &model.Log{
		Username:         username,
		ModelName:        modelName,
		Quota:            quota,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Other:            otherJSON,
	}
}

func findReconItem(t *testing.T, items []ReconItem, username, modelName string) *ReconItem {
	t.Helper()
	for i := range items {
		if items[i].Username == username && items[i].ModelName == modelName {
			return &items[i]
		}
	}
	t.Fatalf("item %q/%q not found in %v", username, modelName, items)
	return nil
}

func TestAggregateLogs_GroupRatioQuotaWeighted(t *testing.T) {
	logs := []*model.Log{
		reconLog(t, "alice", "gpt-4", 300, 10, 20, map[string]interface{}{"group_ratio": 2.0, "cache_tokens": 10}),
		reconLog(t, "alice", "gpt-4", 100, 30, 40, map[string]interface{}{"group_ratio": 1.0}),
	}
	result := aggregateLogs(logs, false)

	require.Len(t, result.Items, 1)
	item := &result.Items[0]
	// (2.0*300 + 1.0*100) / 400
	assert.InDelta(t, 1.75, item.GroupRatio, 1e-9)
	// Existing aggregation keeps working alongside the ratio.
	assert.Equal(t, 2, item.Count)
	assert.Equal(t, 400, item.Quota)
	assert.Equal(t, 10, item.CacheHitTokens)
	// The total row carries the same weighted ratio.
	assert.InDelta(t, 1.75, result.Total.GroupRatio, 1e-9)
}

func TestAggregateLogs_GroupRatioIgnoresLogsWithoutRatio(t *testing.T) {
	logs := []*model.Log{
		reconLog(t, "alice", "gpt-4", 100, 1, 1, map[string]interface{}{"group_ratio": 1.5}),
		reconLog(t, "alice", "gpt-4", 500, 1, 1, nil),
	}
	result := aggregateLogs(logs, false)

	require.Len(t, result.Items, 1)
	// The 500-quota log without ratio info must not dilute the average.
	assert.InDelta(t, 1.5, result.Items[0].GroupRatio, 1e-9)
}

func TestAggregateLogs_GroupRatioZeroQuotaFallsBackToAverage(t *testing.T) {
	logs := []*model.Log{
		reconLog(t, "alice", "gpt-4", 0, 0, 0, map[string]interface{}{"group_ratio": 2.0}),
		reconLog(t, "alice", "gpt-4", 0, 0, 0, map[string]interface{}{"group_ratio": 1.0}),
	}
	result := aggregateLogs(logs, false)

	require.Len(t, result.Items, 1)
	assert.InDelta(t, 1.5, result.Items[0].GroupRatio, 1e-9)
}

func TestAggregateLogs_AdminGroupsByUserAndModel(t *testing.T) {
	logs := []*model.Log{
		reconLog(t, "alice", "gpt-4", 100, 1, 1, map[string]interface{}{"group_ratio": 1.0}),
		reconLog(t, "bob", "gpt-4", 100, 1, 1, map[string]interface{}{"group_ratio": 3.0}),
	}
	admin := aggregateLogs(logs, true)

	require.Len(t, admin.Items, 2)
	alice := findReconItem(t, admin.Items, "alice", "gpt-4")
	bob := findReconItem(t, admin.Items, "bob", "gpt-4")
	assert.InDelta(t, 1.0, alice.GroupRatio, 1e-9)
	assert.InDelta(t, 3.0, bob.GroupRatio, 1e-9)
	// Total ratio is weighted across both rows: (1.0*100 + 3.0*100) / 200
	assert.InDelta(t, 2.0, admin.Total.GroupRatio, 1e-9)

	// The personal view aggregates by model only.
	personal := aggregateLogs(logs, false)
	require.Len(t, personal.Items, 1)
	assert.Equal(t, "", personal.Items[0].Username)
	assert.InDelta(t, 2.0, personal.Items[0].GroupRatio, 1e-9)
}
