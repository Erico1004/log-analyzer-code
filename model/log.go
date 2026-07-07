package model

import "time"

type RawLogInput struct {
	Content    string `json:"content"`
	SourceType string `json:"source_type"`
	Encoding   string `json:"encoding"`
}

type LogContext struct {
	ProcessedText  string   `json:"processed_text"`
	OriginalHash   string   `json:"original_hash"`
	TokenCount     int      `json:"token_count"`
	KeyErrors      []string `json:"key_errors"`
	TimestampRange struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	} `json:"timestamp_range"`
}
