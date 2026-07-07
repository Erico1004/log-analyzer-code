package handler

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"log-analyzer/database"
	"log-analyzer/llm"
	"log-analyzer/model"
	"log-analyzer/preprocessor"
	"log-analyzer/prompt"
	"log-analyzer/retriever"
)

// DiagnoseHandler 诊断处理器
// 对应论文4.2节：系统功能模块设计
type DiagnoseHandler struct {
	preprocessor  *preprocessor.LogPreprocessor
	retriever     *retriever.KnowledgeRetriever
	assembler     *prompt.PromptAssembler
	llmAdapter    *llm.DeepSeekAdapter
	diagnosisRepo *database.DiagnosisRepo
}

// NewDiagnoseHandler 创建诊断处理器实例
func NewDiagnoseHandler(llmAdapter *llm.DeepSeekAdapter) *DiagnoseHandler {
	return &DiagnoseHandler{
		preprocessor:  preprocessor.NewLogPreprocessor(),
		retriever:     retriever.NewKnowledgeRetriever(database.DB),
		assembler:     prompt.NewPromptAssembler(),
		llmAdapter:    llmAdapter,
		diagnosisRepo: database.NewDiagnosisRepo(),
	}
}

// DiagnoseRequest 诊断请求结构
// 对应论文4.2节输入定义
type DiagnoseRequest struct {
	Content  string `json:"content" binding:"required"` // 日志内容
	Strategy string `json:"strategy"`                   // 提示策略：ZERO_SHOT/FEW_SHOT/COT
	UseRAG   bool   `json:"use_rag"`                    // 是否启用RAG检索增强（新增）
}

// Handle 处理诊断请求
// 对应论文4.4节：系统流程设计
func (h *DiagnoseHandler) Handle(c *gin.Context) {
	var req DiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	startTime := time.Now()
	log.Printf("[请求] 收到诊断请求，日志长度: %d 字符，RAG开关: %v", len(req.Content), req.UseRAG)

	// ============================================================
	// 步骤1：日志预处理（对应论文4.2.1节）
	// ============================================================
	logCtx := h.preprocessor.Process(&model.RawLogInput{
		Content:    req.Content,
		SourceType: "PASTE",
	})
	log.Printf("[预处理] Token估算: %d, 关键错误: %v", logCtx.TokenCount, logCtx.KeyErrors)

	// ============================================================
	// 步骤2：知识检索（对应论文4.2.2节）
	// 根据RAG开关决定是否检索知识库
	// ============================================================
	var knowledgeItems []model.KnowledgeItem
	var retrievedIDs string

	if req.UseRAG {
		// RAG模式：从MySQL知识库检索相关案例
		items, err := h.retriever.Retrieve(logCtx, 5, 0.3)
		if err != nil {
			log.Printf("[检索] 失败: %v", err)
			knowledgeItems = []model.KnowledgeItem{}
		} else {
			knowledgeItems = items
			log.Printf("[检索] RAG模式 - 找到 %d 条相关知识", len(knowledgeItems))

			// 提取知识条目ID用于持久化
			ids := make([]string, len(items))
			for i, item := range items {
				ids[i] = fmt.Sprintf("%d", item.ID)
			}
			retrievedIDs = strings.Join(ids, ",")
		}
	} else {
		// 直接LLM模式（基线方法）
		log.Printf("[检索] 直接LLM模式 - 跳过知识检索（基线方法）")
		knowledgeItems = []model.KnowledgeItem{}
		retrievedIDs = ""
	}

	// ============================================================
	// 步骤3：提示组装（对应论文4.2.3节）
	// ============================================================
	strategy := parseStrategy(req.Strategy)
	promptObj := h.assembler.Assemble(logCtx, knowledgeItems, strategy)

	// ============================================================
	// 步骤4：大模型调用（对应论文4.2.4节）
	// ============================================================
	llmResp, err := h.llmAdapter.Invoke(promptObj, "deepseek-chat", 0.1, 2048)
	if err != nil {
		log.Printf("[错误] LLM调用失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LLM调用失败: " + err.Error()})
		return
	}
	log.Printf("[LLM] 调用成功，消耗Token: %d", llmResp.TotalTokens)

	// ============================================================
	// 步骤5：持久化诊断历史（对应论文4.3.2节）
	// ============================================================
	sessionID := uuid.New().String()

	diagnosis := model.DiagnosisResult{
		RootCause:       getString(llmResp.ParsedJSON, "root_cause"),
		AnalysisProcess: getString(llmResp.ParsedJSON, "analysis_process"),
		SolutionSteps:   getStringSlice(llmResp.ParsedJSON, "solution_steps"),
		Confidence:      getFloat64(llmResp.ParsedJSON, "confidence"),
	}

	history := &model.DiagnosisHistory{
		SessionID:       sessionID,
		LogHash:         logCtx.OriginalHash,
		LogSnippet:      truncateString(logCtx.ProcessedText, 500),
		RetrievedDocIDs: retrievedIDs,
		DiagnosisResult: diagnosis,
		ModelUsed:       "deepseek-chat",
		PromptStrategy:  string(strategy),
		TotalTokens:     llmResp.TotalTokens,
		LatencyMs:       int(time.Since(startTime).Milliseconds()),
	}

	if err := h.diagnosisRepo.Create(history); err != nil {
		log.Printf("[警告] 保存诊断历史失败: %v", err)
	}

	latency := time.Since(startTime).Milliseconds()

	// 确定运行模式（用于前端展示）
	mode := "RAG"
	if !req.UseRAG {
		mode = "DIRECT"
	}

	log.Printf("[完成] 模式: %s, 耗时: %dms, 置信度: %.2f%%, 知识条目: %d",
		mode, latency, diagnosis.Confidence*100, len(knowledgeItems))

	// ============================================================
	// 步骤6：返回诊断结果（对应论文4.4.2节）
	// ============================================================
	c.JSON(http.StatusOK, gin.H{
		"diagnosis": diagnosis,
		"metadata": gin.H{
			"session_id":      sessionID,
			"latency_ms":      latency,
			"knowledge_used":  len(knowledgeItems),
			"key_errors":      logCtx.KeyErrors,
			"prompt_strategy": string(strategy),
			"model":           "deepseek-chat",
			"mode":            mode, // 新增：RAG 或 DIRECT
		},
	})
}

// parseStrategy 解析提示策略
// 对应论文4.2.3节：提示策略枚举
func parseStrategy(s string) prompt.PromptStrategy {
	switch strings.ToUpper(s) {
	case "ZERO_SHOT":
		return prompt.ZeroShot
	case "COT":
		return prompt.CoT
	case "FEW_SHOT":
		return prompt.FewShot
	default:
		// 默认使用Few-shot（论文实验表明效果最优）
		return prompt.FewShot
	}
}

// extractIDs 提取知识条目ID列表
func extractIDs(items []model.KnowledgeItem) string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = fmt.Sprintf("%d", item.ID)
	}
	return strings.Join(ids, ",")
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getString 从map中安全获取字符串
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// getStringSlice 从map中安全获取字符串数组
func getStringSlice(m map[string]interface{}, key string) []string {
	if v, ok := m[key].([]interface{}); ok {
		result := make([]string, len(v))
		for i, item := range v {
			if s, ok := item.(string); ok {
				result[i] = s
			}
		}
		return result
	}
	return []string{}
}

// getFloat64 从map中安全获取浮点数
func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0.0
}
