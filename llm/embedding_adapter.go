package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"time"
)

type EmbeddingAdapter struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewEmbeddingAdapter(apiKey, model string) *EmbeddingAdapter {
	return &EmbeddingAdapter{
		apiKey:     apiKey,
		model:      model,
		baseURL:    "https://api.siliconflow.cn/v1/embeddings",
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type EmbeddingRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type EmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (e *EmbeddingAdapter) Embed(text string) ([]float64, error) {
	if e.apiKey == "" {
		return nil, fmt.Errorf("EMBEDDING_API_KEY 未配置")
	}

	reqBody := EmbeddingRequest{
		Model: e.model,
		Input: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedding 请求序列化失败: %w", err)
	}

	var lastErr error
	for retry := 0; retry < 3; retry++ {
		if retry > 0 {
			time.Sleep(time.Second * time.Duration(retry))
		}

		embedding, err := e.doRequest(jsonData)
		if err == nil {
			return embedding, nil
		}
		lastErr = err
		log.Printf("[Embedding] 重试 %d: %v", retry+1, err)
	}

	return nil, fmt.Errorf("embedding 调用失败，已重试3次: %w", lastErr)
}

func (e *EmbeddingAdapter) doRequest(jsonData []byte) ([]float64, error) {
	req, err := http.NewRequest("POST", e.baseURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API 返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var embedResp EmbeddingResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("embedding 响应解析失败: %w", err)
	}

	if len(embedResp.Data) == 0 {
		return nil, fmt.Errorf("embedding 响应中没有数据")
	}

	return embedResp.Data[0].Embedding, nil
}

func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func (e *EmbeddingAdapter) EmbedBatch(texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		embedding, err := e.Embed(text)
		if err != nil {
			log.Printf("[Embedding] 第 %d 条失败: %v", i, err)
			results[i] = nil
		} else {
			results[i] = embedding
		}
		time.Sleep(100 * time.Millisecond)
	}
	return results, nil
}
