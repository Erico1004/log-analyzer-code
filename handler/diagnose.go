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
type DiagnoseHandler struct {
	preprocessor     *preprocessor.LogPreprocessor
	retriever        *retriever.KnowledgeRetriever
	assembler        *prompt.PromptAssembler
	llmAdapter       *llm.DeepSeekAdapter
	embeddingAdapter *llm.EmbeddingAdapter
	diagnosisRepo    *database.DiagnosisRepo
	knowledgeRepo    *database.KnowledgeRepo
}

func NewDiagnoseHandler(llmAdapter *llm.DeepSeekAdapter, embeddingAdapter *llm.EmbeddingAdapter) *DiagnoseHandler {
	h := &DiagnoseHandler{
		preprocessor:     preprocessor.NewLogPreprocessor(),
		assembler:        prompt.NewPromptAssembler(),
		llmAdapter:       llmAdapter,
		embeddingAdapter: embeddingAdapter,
		diagnosisRepo:    database.NewDiagnosisRepo(),
		knowledgeRepo:    database.NewKnowledgeRepo(),
	}
	if database.DB != nil {
		h.retriever = retriever.NewKnowledgeRetriever(database.DB, embeddingAdapter)
	}
	return h
}

// DiagnoseRequest 诊断请求
type DiagnoseRequest struct {
	Content  string `json:"content" binding:"required"`
	Strategy string `json:"strategy"`
	UseRAG   bool   `json:"use_rag"`
}

// Handle 处理诊断请求
func (h *DiagnoseHandler) Handle(c *gin.Context) {
	var req DiagnoseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	startTime := time.Now()
	log.Printf("[请求] 日志长度: %d 字符, RAG: %v", len(req.Content), req.UseRAG)

	// 0. 去重检查：相同日志内容在24小时内已诊断过则返回缓存结果
	logHash := h.preprocessor.Hash(req.Content)
	if existing := h.diagnosisRepo.FindByLogHash(logHash); existing != nil {
		log.Printf("[去重] 命中缓存: ID=%d, 返回已有诊断", existing.ID)
		c.JSON(http.StatusOK, gin.H{
			"diagnosis":       existing,
			"auto_learn":      false,
			"knowledge_count": 0,
			"retrieved_ids":   "",
			"mode":            "cache",
			"latency_ms":      time.Since(startTime).Milliseconds(),
			"log_hash":        logHash,
		})
		return
	}

	// 步骤1：日志预处理
	logCtx := h.preprocessor.Process(&model.RawLogInput{
		Content:    req.Content,
		SourceType: "PASTE",
	})
	log.Printf("[预处理] Token: %d, 关键错误: %v", logCtx.TokenCount, logCtx.KeyErrors)

	// 步骤2：知识检索
	var knowledgeItems []model.KnowledgeItem
	var retrievedIDs string

	if req.UseRAG {
		if h.retriever != nil {
			items, err := h.retriever.Retrieve(logCtx, 5, 0.3)
			if err != nil {
				log.Printf("[检索] 失败: %v", err)
				knowledgeItems = []model.KnowledgeItem{}
			} else {
				knowledgeItems = items
				log.Printf("[检索] 找到 %d 条相关知识", len(knowledgeItems))
				ids := make([]string, len(items))
				for i, item := range items {
					ids[i] = fmt.Sprintf("%d", item.ID)
				}
				retrievedIDs = strings.Join(ids, ",")
			}
		} else {
			log.Printf("[检索] 跳过: retriever 未初始化（可能数据库不可用）")
		}
	} else {
		knowledgeItems = []model.KnowledgeItem{}
	}

	// 步骤3：策略选择
	// RAG 开启但未命中知识库时，自动切换 CoT 进行深度推理
	strategy := parseStrategy(req.Strategy)
	ragMissed := req.UseRAG && len(knowledgeItems) == 0
	if ragMissed {
		strategy = prompt.CoT
		log.Printf("[策略] RAG 未命中，自动切换 CoT 深度推理")
	}

	// 步骤4：提示组装
	promptObj := h.assembler.Assemble(logCtx, knowledgeItems, strategy)

	// 步骤5：大模型调用
	llmResp, err := h.llmAdapter.Invoke(promptObj, "deepseek-chat", 0.1, 2048)
	if err != nil {
		log.Printf("[错误] LLM调用失败: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LLM调用失败: " + err.Error()})
		return
	}
	log.Printf("[LLM] Token: %d", llmResp.TotalTokens)

	// 步骤6：解析诊断结果
	diagnosis := model.DiagnosisResult{
		RootCause:       getString(llmResp.ParsedJSON, "root_cause"),
		AnalysisProcess: getString(llmResp.ParsedJSON, "analysis_process"),
		SolutionSteps:   getStringSlice(llmResp.ParsedJSON, "solution_steps"),
		Confidence:      getFloat64(llmResp.ParsedJSON, "confidence"),
	}

	if diagnosis.RootCause == "" {
		keys := make([]string, 0, len(llmResp.ParsedJSON))
		for k := range llmResp.ParsedJSON {
			keys = append(keys, k)
		}
		c.JSON(http.StatusBadGateway, gin.H{
			"error":        "LLM 返回格式异常，未能提取有效诊断结果",
			"raw_response": llmResp.RawContent,
			"parsed_keys":  keys,
		})
		return
	}

	// 步骤7：自动学习
	// RAG 未命中时，将本次诊断结果存入知识库，下次相同故障可直接检索到
	autoLearned := false
	var learnedID int
	if ragMissed && diagnosis.RootCause != "" {
		kbEntry := &model.KnowledgeBase{
			Title:    truncateString(diagnosis.RootCause, 80),
			Content:  buildKnowledgeContent(diagnosis),
			Category: inferCategory(logCtx.KeyErrors),
			Keywords: strings.Join(logCtx.KeyErrors, ","),
			Symptoms: strings.Join(logCtx.KeyErrors, ","),
		}

		if h.embeddingAdapter != nil {
			embedText := kbEntry.Title + "\n" + kbEntry.Content
			embedding, err := h.embeddingAdapter.Embed(embedText)
			if err != nil {
				log.Printf("[自动学习] embedding 生成失败: %v", err)
			} else {
				kbEntry.Embedding = embedding
			}
		}

		if err := h.knowledgeRepo.Create(kbEntry); err != nil {
			log.Printf("[自动学习] 保存失败: %v", err)
		} else {
			autoLearned = true
			learnedID = kbEntry.ID
			log.Printf("[自动学习] 新知识已保存, ID: %d, 分类: %s, 有embedding: %v", learnedID, kbEntry.Category, len(kbEntry.Embedding) > 0)
		}
	}

	// 步骤8：持久化诊断历史
	sessionID := uuid.New().String()
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
	mode := "RAG"
	if !req.UseRAG {
		mode = "DIRECT"
	}

	log.Printf("[完成] 模式: %s, 策略: %s, 耗时: %dms, 置信度: %.1f%%, 知识: %d, 自动学习: %v",
		mode, strategy, latency, diagnosis.Confidence*100, len(knowledgeItems), autoLearned)

	c.JSON(http.StatusOK, gin.H{
		"diagnosis": diagnosis,
		"metadata": gin.H{
			"session_id":           sessionID,
			"log_hash":             logCtx.OriginalHash,
			"latency_ms":           latency,
			"knowledge_used":       len(knowledgeItems),
			"key_errors":           logCtx.KeyErrors,
			"prompt_strategy":      string(strategy),
			"model":                "deepseek-chat",
			"mode":                 mode,
			"auto_learned":         autoLearned,
			"learned_knowledge_id": learnedID,
		},
	})
}

// buildKnowledgeContent 将诊断结果转化为知识库条目内容
func buildKnowledgeContent(d model.DiagnosisResult) string {
	var sb strings.Builder
	sb.WriteString("【根因】")
	sb.WriteString(d.RootCause)
	sb.WriteString("\n\n【分析过程】")
	sb.WriteString(d.AnalysisProcess)
	sb.WriteString("\n\n【修复步骤】\n")
	for i, step := range d.SolutionSteps {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
	}
	return sb.String()
}

// inferCategory 根据关键错误推断故障分类
func inferCategory(keyErrors []string) string {
	text := strings.ToUpper(strings.Join(keyErrors, " "))
	categories := map[string][]string{
		"内存":   {"OOM", "OUT OF MEMORY", "MEMORY", "HEAP"},
		"磁盘":   {"DISK", "SPACE", "NO SPACE", "IOEXCEPTION"},
		"网络":   {"CONNECTION", "TIMEOUT", "NETWORK", "DNS", "SOCKET"},
		"数据库":  {"DATABASE", "SQL", "DEADLOCK", "ORA-", "MYSQL"},
		"认证权限": {"PERMISSION", "DENIED", "AUTH", "UNAUTHORIZED"},
		"进程":   {"KILLED", "CORE DUMP", "CRASH", "FATAL"},
		"配置":   {"CONFIG", "PARAMETER", "PARSE", "INVALID"},
		"容器":   {"KUBERNETES", "K8S", "DOCKER", "CONTAINER", "POD"},
	}
	for cat, keywords := range categories {
		for _, kw := range keywords {
			if strings.Contains(text, kw) {
				return cat
			}
		}
	}
	return "自动学习"
}

func parseStrategy(s string) prompt.PromptStrategy {
	switch strings.ToUpper(s) {
	case "ZERO_SHOT":
		return prompt.ZeroShot
	case "COT":
		return prompt.CoT
	case "FEW_SHOT":
		return prompt.FewShot
	default:
		return prompt.FewShot
	}
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

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

func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key].(float64); ok {
		return v
	}
	return 0.0
}
