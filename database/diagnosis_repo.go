package database

import "log-analyzer/model"

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
