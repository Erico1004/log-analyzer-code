package handler

import (
	"log-analyzer/database"
	"log-analyzer/model"
	"net/http"

	"github.com/gin-gonic/gin"
)

type FeedbackHandler struct {
	feedbackRepo *database.FeedbackRepo
}

func NewFeedbackHandler() *FeedbackHandler {
	return &FeedbackHandler{
		feedbackRepo: database.NewFeedbackRepo(),
	}
}

type FeedbackRequest struct {
	SessionID   string `json:"session_id" binding:"required"`
	Feedback    int8   `json:"feedback" binding:"required"`
	UserComment string `json:"user_comment"`
}

func (h *FeedbackHandler) Handle(c *gin.Context) {
	var req FeedbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	feedback := &model.UserFeedback{
		SessionID:   req.SessionID,
		Feedback:    req.Feedback,
		UserComment: req.UserComment,
	}

	if err := h.feedbackRepo.Create(feedback); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "反馈已记录"})
}
