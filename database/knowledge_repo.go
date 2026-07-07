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

// Count 统计知识库条目数
func (r *KnowledgeRepo) Count() (int64, error) {
	var count int64
	err := DB.Model(&model.KnowledgeBase{}).Count(&count).Error
	return count, err
}
