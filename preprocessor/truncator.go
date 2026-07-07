package preprocessor

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

type LogLine struct {
	Content string
	Weight  float64
	Index   int
}

type LogTruncator struct {
	levelWeights map[string]float64
	maxTokens    int
}

func NewLogTruncator(maxTokens int) *LogTruncator {
	return &LogTruncator{
		maxTokens: maxTokens,
		levelWeights: map[string]float64{
			"FATAL": 1.0, "ERROR": 1.0, "EXCEPTION": 0.9,
			"WARN": 0.6, "WARNING": 0.6,
			"INFO": 0.2, "DEBUG": 0.05, "TRACE": 0.05,
		},
	}
}

func (t *LogTruncator) calculateWeight(line string) float64 {
	upperLine := strings.ToUpper(line)

	for level, weight := range t.levelWeights {
		if strings.Contains(upperLine, level) {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") ||
				strings.HasPrefix(line, "at ") || strings.HasPrefix(line, "Caused by") {
				if weight < 0.8 {
					return 0.8
				}
			}
			return weight
		}
	}
	return 0.1
}

func (t *LogTruncator) Truncate(rawLogs string) (string, string) {
	lines := strings.Split(rawLogs, "\n")
	if len(lines) == 0 {
		return "", ""
	}

	charLimit := t.maxTokens * 3

	weightedLines := make([]LogLine, len(lines))
	for i, line := range lines {
		weightedLines[i] = LogLine{
			Content: line,
			Weight:  t.calculateWeight(line),
			Index:   i,
		}
	}

	sortedLines := make([]LogLine, len(weightedLines))
	copy(sortedLines, weightedLines)
	sort.Slice(sortedLines, func(i, j int) bool {
		return sortedLines[i].Weight > sortedLines[j].Weight
	})

	selectedSet := make(map[int]bool)
	currentLen := 0

	for i := 0; i < 3 && i < len(lines); i++ {
		selectedSet[i] = true
		currentLen += len(lines[i]) + 1
	}
	for i := len(lines) - 3; i < len(lines) && i >= 0; i++ {
		if !selectedSet[i] {
			selectedSet[i] = true
			currentLen += len(lines[i]) + 1
		}
	}

	for _, line := range sortedLines {
		if currentLen+len(line.Content) > charLimit {
			continue
		}
		if !selectedSet[line.Index] {
			selectedSet[line.Index] = true
			currentLen += len(line.Content) + 1
		}
	}

	var result []string
	for i, line := range lines {
		if selectedSet[i] {
			result = append(result, line)
		}
	}

	hash := sha256.Sum256([]byte(rawLogs))
	hashStr := hex.EncodeToString(hash[:])

	return strings.Join(result, "\n"), hashStr
}
