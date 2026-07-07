package database

import (
	"log-analyzer/model"
)

type DiagnosisRepo struct{}

func NewDiagnosisRepo() *DiagnosisRepo {
	return &DiagnosisRepo{}
}

func (r *DiagnosisRepo) Create(history *model.DiagnosisHistory) error {
	return DB.Create(history).Error
}

func (r *DiagnosisRepo) GetBySessionID(sessionID string) (*model.DiagnosisHistory, error) {
	var history model.DiagnosisHistory
	err := DB.Where("session_id = ?", sessionID).First(&history).Error
	return &history, err
}

// List 分页查询诊断历史
func (r *DiagnosisRepo) List(page, pageSize int) ([]model.DiagnosisHistory, int64, error) {
	var items []model.DiagnosisHistory
	var total int64

	if err := DB.Model(&model.DiagnosisHistory{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	err := DB.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&items).Error
	return items, total, err
}

// GetFeedbackStats 获取反馈统计（从 user_feedback 表）
func (r *DiagnosisRepo) GetFeedbackStats() (correct int64, incorrect int64, err error) {
	if err := DB.Table("user_feedback").Where("feedback = 1").Count(&correct).Error; err != nil {
		return 0, 0, err
	}
	if err := DB.Table("user_feedback").Where("feedback = 0").Count(&incorrect).Error; err != nil {
		return 0, 0, err
	}
	return correct, incorrect, nil
}
