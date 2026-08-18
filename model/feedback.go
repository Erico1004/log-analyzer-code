package model

import "time"

type UserFeedback struct {
	ID          int       `gorm:"primaryKey;autoIncrement" json:"id"`
	SessionID   string    `gorm:"type:char(36);not null;index" json:"session_id"`
	Feedback    int8      `gorm:"type:smallint;not null" json:"feedback"`
	UserComment string    `gorm:"type:text" json:"user_comment"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (UserFeedback) TableName() string {
	return "user_feedback"
}
