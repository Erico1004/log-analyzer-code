package prompt

import (
	"bytes"
	"log-analyzer/model"
	"text/template"
)

type PromptStrategy string

const (
	ZeroShot PromptStrategy = "ZERO_SHOT"
	FewShot  PromptStrategy = "FEW_SHOT"
	CoT      PromptStrategy = "COT"
)

type Prompt struct {
	SystemPrompt   string `json:"system_prompt"`
	UserPrompt     string `json:"user_prompt"`
	FullPrompt     string `json:"full_prompt"`
	ExpectedFormat string `json:"expected_format"`
}

type PromptAssembler struct {
	templates map[PromptStrategy]*template.Template
}

func NewPromptAssembler() *PromptAssembler {
	templates := make(map[PromptStrategy]*template.Template)

	templates[ZeroShot] = template.Must(template.New("zero_shot").Parse(ZeroShotTemplate))
	templates[FewShot] = template.Must(template.New("few_shot").Parse(FewShotTemplate))
	templates[CoT] = template.Must(template.New("cot").Parse(CoTTemplate))

	return &PromptAssembler{templates: templates}
}

func (a *PromptAssembler) Assemble(ctx *model.LogContext, items []model.KnowledgeItem, strategy PromptStrategy) *Prompt {
	tmpl := a.templates[strategy]
	if tmpl == nil {
		tmpl = a.templates[FewShot]
	}

	data := map[string]interface{}{
		"LogContent":     ctx.ProcessedText,
		"KnowledgeItems": items,
	}

	var buf bytes.Buffer
	tmpl.Execute(&buf, data)
	userPrompt := buf.String()

	return &Prompt{
		SystemPrompt:   "你是一名资深的运维工程师，请严格按JSON格式输出诊断结果。",
		UserPrompt:     userPrompt,
		FullPrompt:     userPrompt,
		ExpectedFormat: `{"root_cause": "...", "analysis_process": "...", "solution_steps": [...], "confidence": 0.XX}`,
	}
}
