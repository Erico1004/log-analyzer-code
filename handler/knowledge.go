package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"log-analyzer/database"
	"log-analyzer/model"
)

// KnowledgeHandler 知识库管理处理器
type KnowledgeHandler struct {
	repo *database.KnowledgeRepo
}

// NewKnowledgeHandler 创建知识库处理器
func NewKnowledgeHandler() *KnowledgeHandler {
	return &KnowledgeHandler{repo: database.NewKnowledgeRepo()}
}

// List 获取知识库列表（分页 + 搜索）
func (h *KnowledgeHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	keyword := c.Query("search")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	items, total, err := h.repo.List(keyword, page, pageSize)
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

// Get 获取单条知识
func (h *KnowledgeHandler) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	item, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "条目不存在"})
		return
	}

	c.JSON(http.StatusOK, item)
}

// Create 创建知识条目
func (h *KnowledgeHandler) Create(c *gin.Context) {
	var item model.KnowledgeBase
	if err := c.ShouldBindJSON(&item); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	if item.Title == "" || item.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题和内容不能为空"})
		return
	}

	if err := h.repo.Create(&item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": item.ID, "message": "创建成功"})
}

// Update 更新知识条目
func (h *KnowledgeHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	existing, err := h.repo.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "条目不存在"})
		return
	}

	var input model.KnowledgeBase
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数错误: " + err.Error()})
		return
	}

	existing.Title = input.Title
	existing.Content = input.Content
	existing.Category = input.Category
	existing.Keywords = input.Keywords
	existing.Symptoms = input.Symptoms

	if err := h.repo.Update(existing); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "更新成功"})
}

// Delete 删除知识条目
func (h *KnowledgeHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的ID"})
		return
	}

	if err := h.repo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
