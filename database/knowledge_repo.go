package database

import (
	"log-analyzer/model"
)

// KnowledgeRepo 知识库数据操作层
// 知识库表设计
type KnowledgeRepo struct{}

// NewKnowledgeRepo 创建知识库仓库实例
func NewKnowledgeRepo() *KnowledgeRepo {
	return &KnowledgeRepo{}
}

// GetAll 获取所有知识条目
func (r *KnowledgeRepo) GetAll() ([]model.KnowledgeBase, error) {
	var items []model.KnowledgeBase
	err := DB.Order("id DESC").Find(&items).Error
	return items, err
}

// GetByID 根据ID获取知识条目
func (r *KnowledgeRepo) GetByID(id int) (*model.KnowledgeBase, error) {
	var item model.KnowledgeBase
	err := DB.First(&item, id).Error
	return &item, err
}

// GetByCategory 按分类获取知识条目
func (r *KnowledgeRepo) GetByCategory(category string) ([]model.KnowledgeBase, error) {
	var items []model.KnowledgeBase
	err := DB.Where("category = ?", category).Order("id DESC").Find(&items).Error
	return items, err
}

// Create 创建知识条目
func (r *KnowledgeRepo) Create(item *model.KnowledgeBase) error {
	return DB.Create(item).Error
}

// Update 更新知识条目
func (r *KnowledgeRepo) Update(item *model.KnowledgeBase) error {
	return DB.Save(item).Error
}

// Delete 删除知识条目
func (r *KnowledgeRepo) Delete(id int) error {
	return DB.Delete(&model.KnowledgeBase{}, id).Error
}

// List 分页搜索知识库
func (r *KnowledgeRepo) List(keyword string, page, pageSize int) ([]model.KnowledgeBase, int64, error) {
	var items []model.KnowledgeBase
	var total int64

	query := DB.Model(&model.KnowledgeBase{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("title LIKE ? OR content LIKE ? OR keywords LIKE ? OR symptoms LIKE ? OR category LIKE ?",
			like, like, like, like, like)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&items).Error
	return items, total, err
}

// Count 统计知识库条目数
func (r *KnowledgeRepo) Count() (int64, error) {
	var count int64
	err := DB.Model(&model.KnowledgeBase{}).Count(&count).Error
	return count, err
}

// FindWithoutEmbedding 获取所有没有 embedding 的条目
func (r *KnowledgeRepo) FindWithoutEmbedding() ([]model.KnowledgeBase, error) {
	var items []model.KnowledgeBase
	err := DB.Where("embedding IS NULL OR JSON_LENGTH(embedding) = 0").Find(&items).Error
	return items, err
}

// UpdateEmbedding 更新条目的 embedding
func (r *KnowledgeRepo) UpdateEmbedding(id int, embedding []float64) error {
	return DB.Model(&model.KnowledgeBase{}).Where("id = ?", id).Update("embedding", embedding).Error
}
