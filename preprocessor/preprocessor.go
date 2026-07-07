package preprocessor

import (
	"log-analyzer/model"
)

const MaxTokens = 6000

type LogPreprocessor struct {
	desensitizer *LogDesensitizer
	truncator    *LogTruncator
	structurer   *LogStructurer
}

func NewLogPreprocessor() *LogPreprocessor {
	return &LogPreprocessor{
		desensitizer: NewLogDesensitizer(),
		truncator:    NewLogTruncator(MaxTokens),
		structurer:   NewLogStructurer(),
	}
}

func (p *LogPreprocessor) Process(input *model.RawLogInput) *model.LogContext {
	masked := p.desensitizer.Mask(input.Content)
	truncated, hash := p.truncator.Truncate(masked)
	ctx := p.structurer.Structure(truncated)
	ctx.OriginalHash = hash
	return ctx
}
