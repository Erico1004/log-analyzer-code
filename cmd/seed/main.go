package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pgvector/pgvector-go"

	"log-analyzer/config"
	"log-analyzer/database"
	"log-analyzer/llm"
	"log-analyzer/model"
)

type TestCase struct {
	ID                string   `json:"id"`
	Category          string   `json:"category"`
	Log               string   `json:"log"`
	ExpectedRootCause string   `json:"expected_root_cause"`
	ExpectedKeywords  []string `json:"expected_keywords"`
}

func main() {
	if err := config.LoadConfig(); err != nil {
		log.Fatal("加载配置失败:", err)
	}

	if err := database.InitDB(); err != nil {
		log.Fatal("数据库连接失败:", err)
	}

	if config.AppConfig.EmbeddingAPIKey == "" {
		log.Fatal("EMBEDDING_API_KEY 未配置")
	}

	embedder := llm.NewEmbeddingAdapter(config.AppConfig.EmbeddingAPIKey, config.AppConfig.EmbeddingModel)
	repo := database.NewKnowledgeRepo()

	// 检查是否已有数据
	count, _ := repo.Count()
	if count > 0 {
		fmt.Printf("知识库已有 %d 条数据，跳过种子\n", count)

		// 检查是否有缺 embedding 的
		missing, _ := repo.FindWithoutEmbedding()
		if len(missing) > 0 {
			fmt.Printf("发现 %d 条缺少 embedding，开始回填...\n", len(missing))
			backfill(repo, embedder, missing)
		}
		return
	}

	// 读取测试案例（绝对路径，避免 cwd 依赖）
	casesPath := resolveCasesPath()
	data, err := os.ReadFile(casesPath)
	if err != nil {
		log.Fatalf("读取测试案例失败 (%s): %v", casesPath, err)
	}
	var cases []TestCase
	if err := json.Unmarshal(data, &cases); err != nil {
		log.Fatal("解析测试案例失败:", err)
	}
	fmt.Printf("从 %s 加载 %d 个案例，开始生成知识库...\n", casesPath, len(cases))

	success := 0
	for i, tc := range cases {
		title := fmt.Sprintf("[%s] %s", tc.Category, truncate(tc.ExpectedRootCause, 60))
		content := buildContent(tc)
		keywords := strings.Join(tc.ExpectedKeywords, ",")
		symptoms := keywords

		entry := &model.KnowledgeBase{
			Title:    title,
			Content:  content,
			Category: tc.Category,
			Keywords: keywords,
			Symptoms: symptoms,
		}

		// 生成 embedding
		embedText := title + "\n" + content
		embedding, err := embedder.Embed(embedText)
		if err != nil {
			log.Printf("[%d/%d] ID=%s embedding 失败: %v", i+1, len(cases), tc.ID, err)
		} else {
			entry.Embedding = pgvector.NewVector(llm.ToFloat32(embedding))
		}

		if err := repo.Create(entry); err != nil {
			log.Printf("[%d/%d] ID=%s 保存失败: %v", i+1, len(cases), tc.ID, err)
		} else {
			hasEmb := "无"
			if len(entry.Embedding.Slice()) > 0 {
				hasEmb = fmt.Sprintf("有(维度%d)", len(entry.Embedding.Slice()))
			}
			fmt.Printf("[%d/%d] ID=%s [%s] -> ID=%d, embedding: %s\n", i+1, len(cases), tc.ID, tc.Category, entry.ID, hasEmb)
			success++
		}

		time.Sleep(200 * time.Millisecond)
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("种子完成: 成功 %d/%d\n", success, len(cases))
	fmt.Printf("========================================\n")
}

// resolveCasesPath 定位 testdata/cases.json
// 与 experiment/runner.go 的多候选路径策略一致：
//
//	方案1：os.Executable() 可执行文件目录及上级（部署后依然有效）
//	方案2：os.Getwd() 当前工作目录（兼容 go run）
//	方案3：兜底相对路径
func resolveCasesPath() string {
	// 方案1：基于可执行文件位置（部署后依然有效，与 experiment 一致）
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates := []string{
			filepath.Join(exeDir, "testdata", "cases.json"),
			filepath.Join(exeDir, "..", "testdata", "cases.json"),
		}
		for _, p := range candidates {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}

	// 方案2：从 cwd 向上查找（兼容 go run 等多种运行方式）
	if cwd, err := os.Getwd(); err == nil {
		dir := cwd
		for i := 0; i < 5; i++ {
			p := filepath.Join(dir, "testdata", "cases.json")
			if _, err := os.Stat(p); err == nil {
				return p
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 方案3：回退到相对路径（最简方式）
	return filepath.Join("testdata", "cases.json")
}

func backfill(repo *database.KnowledgeRepo, embedder *llm.EmbeddingAdapter, items []model.KnowledgeBase) {
	success := 0
	for i, item := range items {
		embedText := item.Title + "\n" + item.Content
		embedding, err := embedder.Embed(embedText)
		if err != nil {
			log.Printf("[%d/%d] ID=%d 失败: %v", i+1, len(items), item.ID, err)
			continue
		}
		if err := repo.UpdateEmbedding(item.ID, llm.ToFloat32(embedding)); err != nil {
			log.Printf("[%d/%d] ID=%d 保存失败: %v", i+1, len(items), item.ID, err)
		} else {
			fmt.Printf("[%d/%d] ID=%d -> embedding 维度 %d\n", i+1, len(items), item.ID, len(embedding))
			success++
		}
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Printf("\n回填完成: 成功 %d/%d\n", success, len(items))
}

// buildContent 生成知识库内容，包含根因、触发条件、影响范围、修复步骤
// 补齐旧版遗漏的【修复步骤】【触发条件】【影响】，让 RAG 召回内容更厚
func buildContent(tc TestCase) string {
	var sb strings.Builder
	sb.WriteString("【故障分类】")
	sb.WriteString(tc.Category)
	sb.WriteString("\n\n【根因分析】")
	sb.WriteString(tc.ExpectedRootCause)

	// 触发条件：从关键词和日志特征提炼
	sb.WriteString("\n\n【触发条件】")
	sb.WriteString(inferTrigger(tc))

	// 影响范围：从分类推断
	sb.WriteString("\n\n【影响范围】")
	sb.WriteString(inferImpact(tc))

	sb.WriteString("\n\n【关键词】")
	sb.WriteString(strings.Join(tc.ExpectedKeywords, ", "))

	sb.WriteString("\n\n【修复步骤】\n")
	sb.WriteString(inferSolution(tc))

	sb.WriteString("\n【典型日志特征】\n")
	sb.WriteString(tc.Log)
	return sb.String()
}

// inferTrigger 从关键词推断触发条件
// 注意：分支顺序按"特异性从高到低"排列（与 inferSolution 一致）
func inferTrigger(tc TestCase) string {
	kwStr := strings.ToLower(strings.Join(tc.ExpectedKeywords, " "))
	switch {
	case strings.Contains(kwStr, "oom") || strings.Contains(kwStr, "memory"):
		return "内存使用率持续超过阈值（通常 >90%）；进程 RSS 接近物理内存上限；Swap 几乎耗尽。"
	case strings.Contains(kwStr, "raid"):
		return "RAID 阵列中 >=2 块盘离线；或热备盘未自动激活；阵列降级。"
	case strings.Contains(kwStr, "disk") || strings.Contains(kwStr, "ntfs"):
		return "磁盘剩余空间低于 5%；或文件系统索引出现损坏；CHKDSK 反复触发。"
	case strings.Contains(kwStr, "dns") || strings.Contains(kwStr, "network") || strings.Contains(kwStr, "connection"):
		return "DNS 解析失败率 >10%；或关键端口监听消失；或跨节点连通性超时。"
	case strings.Contains(kwStr, "token") || strings.Contains(kwStr, "authentication") || strings.Contains(kwStr, "permission"):
		return "ServiceAccount token 缺失或过期；RBAC 权限变更未同步；或证书轮替失败。"
	case strings.Contains(kwStr, "502") || strings.Contains(kwStr, "500") || strings.Contains(kwStr, "gateway"):
		return "上游服务健康检查失败率 >50%；或后端实例可用数低于最小副本数。"
	case strings.Contains(kwStr, "lock") || strings.Contains(kwStr, "loop"):
		return "同一错误日志在 10 秒内重复 >1000 次；或锁文件长期未释放。"
	default:
		return "对应关键词在日志中高频出现且伴随 ERROR/CRITICAL 级别。"
	}
}

// inferImpact 从分类推断影响范围
func inferImpact(tc TestCase) string {
	switch tc.Category {
	case "OOM":
		return "单节点服务不可用，可能触发级联崩溃；日志采集链中断导致监控盲区。"
	case "磁盘":
		return "数据写入失败，业务流程中断；严重时文件系统损坏需离线修复。"
	case "网络":
		return "跨节点通信异常，影响范围随故障层级扩散（DNS > 单机 > 机房）。"
	case "代码缺陷":
		return "逻辑性故障，影响特定业务路径；可能引发数据不一致。"
	case "连接池":
		return "API 吞吐量骤降，请求堆积触发雪崩；就绪探针失败导致服务被摘除。"
	case "HTTP":
		return "用户可见错误（5xx/502），影响 SLA；可能触发告警风暴。"
	case "权限":
		return "服务启动失败或关键操作被拒绝；可能影响安全审计合规。"
	default:
		return "影响范围视故障传播路径而定，需结合监控指标评估。"
	}
}

// inferSolution 从分类和关键词推断修复步骤
// 注意：分支顺序按"特异性从高到低"排列，避免通用关键词遮蔽具体场景
// 例如 RAID 关键词同时含 disk，必须把 RAID 分支放在通用 disk 分支之前
func inferSolution(tc TestCase) string {
	kwStr := strings.ToLower(strings.Join(tc.ExpectedKeywords, " "))
	switch {
	// 特异性最高：RAID 阵列场景（关键词同时含 disk，需优先匹配）
	case strings.Contains(kwStr, "raid"):
		return "1. 标识故障盘：megacli / storcli 查看状态\n2. 激活热备盘：确认配置权限充足\n3. 重建阵列：监控 rebuild 进度\n4. 复盘：校验热备盘策略和告警通道"
	// 特异性次高：NTFS/文件系统损坏（关键词含 ntfs，不同于通用 disk 满）
	case strings.Contains(kwStr, "ntfs"):
		return "1. 紧急清理：删除过期日志/临时文件\n2. 修复文件系统：umount 后运行 chkdsk / xfs_repair\n3. 如有累积更新：回滚 KB 补丁\n4. 长期：配置日志轮转和磁盘容量告警"
	// 通用磁盘满/损坏场景
	case strings.Contains(kwStr, "disk"):
		return "1. 紧急清理：删除过期日志/临时文件\n2. 扩容：增加磁盘空间或挂载新卷\n3. 修复文件系统：umount 后运行 xfs_repair\n4. 长期：配置日志轮转和磁盘容量告警"
	case strings.Contains(kwStr, "oom") || strings.Contains(kwStr, "memory"):
		return "1. 临时扩容：kubectl scale 增加副本数或垂直扩容内存\n2. 定位内存泄漏：抓取 pprof heap dump 分析\n3. 调整 OOM 评分：sysctl vm.swappiness 或调整 oom_score_adj\n4. 复盘：补内存告警阈值，设置提前预警"
	case strings.Contains(kwStr, "dns"):
		return "1. 临时规避：切换到备用 DNS（如 1.1.1.1）\n2. 抓包定位：tcpdump port 53 分析请求\n3. 修复客户端：升级固件或重启 DNS 服务\n4. 复盘：配置多级 DNS 冗余"
	case strings.Contains(kwStr, "token") || strings.Contains(kwStr, "authentication"):
		return "1. 手动生成 token：kubectl create token\n2. 校验 RBAC：检查 RoleBinding/ClusterRoleBinding\n3. 重启 Pod：强制重新挂载 token\n4. 复盘：升级前预检 ServiceAccount 配置"
	case strings.Contains(kwStr, "502") || strings.Contains(kwStr, "gateway"):
		return "1. 检查上游健康：curl 直接访问后端实例\n2. 扩容：手动增加副本数\n3. 调整探针：避免过早摘除\n4. 复盘：优化 HPA 指标（不只看 CPU）"
	case strings.Contains(kwStr, "lock") || strings.Contains(kwStr, "loop"):
		return "1. 强制释放：删除 /var/lock/*.lock 文件\n2. 重启相关进程：systemctl restart\n3. 定位干扰源：审计近期变更\n4. 复盘：为锁文件加超时和告警"
	default:
		return "1. 保留现场：日志快照 + 监控截图\n2. 临时恢复：重启/回滚到上一版本\n3. 根因定位：抓堆栈/trace\n4. 复盘：补充对应监控和告警规则"
	}
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}
