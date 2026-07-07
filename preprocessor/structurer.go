package preprocessor

import (
	"log-analyzer/model"
	"strings"
)

type LogStructurer struct{}

func NewLogStructurer() *LogStructurer {
	return &LogStructurer{}
}

func (s *LogStructurer) Structure(text string) *model.LogContext {
	ctx := &model.LogContext{
		ProcessedText: text,
		TokenCount:    len(text) / 3,
		KeyErrors:     s.extractKeyErrors(text),
	}
	return ctx
}

func (s *LogStructurer) extractKeyErrors(text string) []string {
	patterns := []string{
		"OutOfMemoryError", "Java heap space", "GC overhead",
		"NullPointerException", "ClassCastException",
		"Connection refused", "Connection timeout", "SocketTimeoutException",
		"SQLException", "Deadlock", "Lock wait timeout",
		"Disk full", "No space left",
		"CPU", "memory leak",
		"502", "503", "504",
		"Permission denied", "Access denied",
	}

	textUpper := strings.ToUpper(text)
	found := make(map[string]bool)
	var result []string

	for _, pattern := range patterns {
		if strings.Contains(textUpper, strings.ToUpper(pattern)) {
			if !found[pattern] {
				found[pattern] = true
				result = append(result, pattern)
			}
		}
	}

	if len(result) > 10 {
		result = result[:10]
	}
	return result
}
