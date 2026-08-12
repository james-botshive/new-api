package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// GetPersonalModels returns distinct model names from the current user's consume logs.
func GetPersonalModels(c *gin.Context) {
	userId := c.GetInt("id")
	models, err := model.GetUserDistinctModels(userId)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": []string{}})
		return
	}
	if models == nil {
		models = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": models})
}

// GetPersonalReconciliation returns the current user's consume logs aggregated by model.
func GetPersonalReconciliation(c *gin.Context) {
	userId := c.GetInt("id")
	startTs, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTs, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")

	result, err := service.GetPersonalReconciliation(userId, startTs, endTs, modelName)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// GetAdminReconciliation returns all users' consume logs aggregated by user+model.
func GetAdminReconciliation(c *gin.Context) {
	startTs, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTs, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	modelName := c.Query("model_name")
	username := c.Query("username")

	result, err := service.GetAdminReconciliation(startTs, endTs, modelName, username)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}
