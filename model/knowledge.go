package model

import (
	"time"

	"github.com/pgvector/pgvector-go"
)

type KnowledgeBase struct {
	ID        int             `gorm:"primaryKey;autoIncrement" json:"id"`
	Title     string          `gorm:"type:varchar(255);not null" json:"title"`
	Content   string          `gorm:"type:text;not null" json:"content"`
	Category  string          `gorm:"type:varchar(50)" json:"category"`
	Keywords  string          `gorm:"type:text" json:"keywords"`
	Symptoms  string          `gorm:"type:text" json:"symptoms"`
	Embedding pgvector.Vector `gorm:"type:vector(1024)" json:"embedding,omitempty"`
	CreatedAt time.Time       `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time       `gorm:"autoUpdateTime" json:"updated_at"`
}

func (KnowledgeBase) TableName() string {
	return "knowledge_base"
}

type KnowledgeItem struct {
	ID              int     `json:"id"`
	Title           string  `json:"title"`
	Content         string  `json:"content"`
	Category        string  `json:"category"`
	SimilarityScore float64 `json:"similarity_score"`
}
