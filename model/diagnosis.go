package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

type DiagnosisResult struct {
	RootCause       string   `json:"root_cause"`
	AnalysisProcess string   `json:"analysis_process"`
	SolutionSteps   []string `json:"solution_steps"`
	Confidence      float64  `json:"confidence"`
}

func (d *DiagnosisResult) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(bytes, d)
}

func (d DiagnosisResult) Value() (driver.Value, error) {
	return json.Marshal(d)
}

type DiagnosisHistory struct {
	ID              int             `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID       string          `gorm:"type:char(36);not null;uniqueIndex" json:"session_id"`
	LogHash         string          `gorm:"type:char(64);not null" json:"log_hash"`
	LogSnippet      string          `gorm:"type:text" json:"log_snippet"`
	RetrievedDocIDs string          `gorm:"type:varchar(500)" json:"retrieved_doc_ids"`
	DiagnosisResult DiagnosisResult `gorm:"type:json;not null" json:"diagnosis_result"`
	ModelUsed       string          `gorm:"type:varchar(50);not null" json:"model_used"`
	PromptStrategy  string          `gorm:"type:varchar(20);not null" json:"prompt_strategy"`
	TotalTokens     int             `json:"total_tokens"`
	LatencyMs       int             `json:"latency_ms"`
	CreatedAt       time.Time       `gorm:"autoCreateTime" json:"created_at"`
}

func (DiagnosisHistory) TableName() string {
	return "diagnosis_history"
}
