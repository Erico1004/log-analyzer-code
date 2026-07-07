package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"log-analyzer/database"
)

// HistoryHandler 诊断历史处理器
type HistoryHandler struct {
	diagRepo *database.DiagnosisRepo
}

// NewHistoryHandler 创建历史处理器
func NewHistoryHandler() *HistoryHandler {
	return &HistoryHandler{diagRepo: database.NewDiagnosisRepo()}
}

// List 获取诊断历史列表（分页）
func (h *HistoryHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := h.diagRepo.List(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// Stats 获取系统统计数据
func (h *HistoryHandler) Stats(c *gin.Context) {
	correct, incorrect, err := h.diagRepo.GetFeedbackStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"feedback_correct":   correct,
		"feedback_incorrect": incorrect,
	})
}
