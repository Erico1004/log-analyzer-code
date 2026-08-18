package database

import (
	"log-analyzer/model"

	"github.com/pgvector/pgvector-go"
)

type KnowledgeRepo struct{}

func NewKnowledgeRepo() *KnowledgeRepo {
	return &KnowledgeRepo{}
}

func (r *KnowledgeRepo) GetAll() ([]model.KnowledgeBase, error) {
	var items []model.KnowledgeBase
	err := DB.Order("id DESC").Find(&items).Error
	return items, err
}

func (r *KnowledgeRepo) GetByID(id int) (*model.KnowledgeBase, error) {
	var item model.KnowledgeBase
	err := DB.First(&item, id).Error
	return &item, err
}

func (r *KnowledgeRepo) GetByCategory(category string) ([]model.KnowledgeBase, error) {
	var items []model.KnowledgeBase
	err := DB.Where("category = ?", category).Order("id DESC").Find(&items).Error
	return items, err
}

func (r *KnowledgeRepo) Create(item *model.KnowledgeBase) error {
	return DB.Create(item).Error
}

func (r *KnowledgeRepo) Update(item *model.KnowledgeBase) error {
	return DB.Save(item).Error
}

func (r *KnowledgeRepo) Delete(id int) error {
	return DB.Delete(&model.KnowledgeBase{}, id).Error
}

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

func (r *KnowledgeRepo) Count() (int64, error) {
	var count int64
	err := DB.Model(&model.KnowledgeBase{}).Count(&count).Error
	return count, err
}

func (r *KnowledgeRepo) FindWithoutEmbedding() ([]model.KnowledgeBase, error) {
	var items []model.KnowledgeBase
	err := DB.Where("embedding IS NULL").Find(&items).Error
	return items, err
}

func (r *KnowledgeRepo) UpdateEmbedding(id int, embedding []float32) error {
	return DB.Model(&model.KnowledgeBase{}).Where("id = ?", id).
		Update("embedding", pgvector.NewVector(embedding)).Error
}
