package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log-analyzer/prompt"
	"net/http"
	"strings"
	"time"
)

type DeepSeekAdapter struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewDeepSeekAdapter(apiKey string) *DeepSeekAdapter {
	return &DeepSeekAdapter{
		apiKey:     apiKey,
		baseURL:    "https://api.deepseek.com/v1/chat/completions",
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type ChatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

type LLMResponse struct {
	RawContent  string                 `json:"raw_content"`
	ParsedJSON  map[string]interface{} `json:"parsed_json"`
	ModelUsed   string                 `json:"model_used"`
	TotalTokens int                    `json:"total_tokens"`
	LatencyMs   int64                  `json:"latency_ms"`
}

func (d *DeepSeekAdapter) Invoke(p *prompt.Prompt, modelName string, temperature float64, maxTokens int) (*LLMResponse, error) {
	startTime := time.Now()

	reqBody := ChatRequest{
		Model: modelName,
		Messages: []Message{
			{Role: "system", Content: p.SystemPrompt},
			{Role: "user", Content: p.UserPrompt},
		},
		Temperature: temperature,
		MaxTokens:   maxTokens,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("请求序列化失败: %w", err)
	}

	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(time.Second * time.Duration(retry))
		}

		resp, err := d.doRequest(jsonData, startTime)
		if err == nil {
			return resp, nil
		}
	}

	return nil, fmt.Errorf("调用失败，已重试3次")
}

func (d *DeepSeekAdapter) doRequest(jsonData []byte, startTime time.Time) (*LLMResponse, error) {
	req, err := http.NewRequest("POST", d.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.apiKey)

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, err
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("响应中没有choices")
	}

	rawContent := chatResp.Choices[0].Message.Content
	parsedJSON := d.parseJSON(rawContent)

	return &LLMResponse{
		RawContent:  rawContent,
		ParsedJSON:  parsedJSON,
		ModelUsed:   "deepseek-chat",
		TotalTokens: chatResp.Usage.TotalTokens,
		LatencyMs:   time.Since(startTime).Milliseconds(),
	}, nil
}

func (d *DeepSeekAdapter) parseJSON(content string) map[string]interface{} {
	var result map[string]interface{}

	if err := json.Unmarshal([]byte(content), &result); err == nil {
		return result
	}

	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start != -1 && end != -1 && end > start {
		jsonStr := content[start : end+1]
		if err := json.Unmarshal([]byte(jsonStr), &result); err == nil {
			return result
		}
	}

	return map[string]interface{}{
		"root_cause":       "解析失败",
		"analysis_process": content,
		"solution_steps":   []string{},
		"confidence":       0.0,
	}
}
