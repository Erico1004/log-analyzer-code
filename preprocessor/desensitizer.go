package preprocessor

import "regexp"

var (
	ipv4Regex      = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	emailRegex     = regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`)
	secretKeyRegex = regexp.MustCompile(`[A-Za-z0-9]{32,}`)
	tokenRegex     = regexp.MustCompile(`(?i)(token|secret|key|password|passwd)[=:]\s*[^\s,;]+`)
)

type LogDesensitizer struct{}

func NewLogDesensitizer() *LogDesensitizer {
	return &LogDesensitizer{}
}

func (d *LogDesensitizer) Mask(text string) string {
	text = ipv4Regex.ReplaceAllString(text, "[IP_REDACTED]")
	text = emailRegex.ReplaceAllString(text, "[EMAIL_REDACTED]")
	text = secretKeyRegex.ReplaceAllString(text, "[SECRET_REDACTED]")
	text = tokenRegex.ReplaceAllString(text, "$1=[SECRET_REDACTED]")
	return text
}
