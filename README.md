# LogAnalyzer — 运维日志智能分析系统

基于 **RAG（检索增强生成）** 的运维日志智能诊断平台，使用 DeepSeek 大语言模型进行故障根因分析，配合 MySQL 知识库实现案例检索增强推理。

## 功能概览

| 模块 | 功能 |
|------|------|
| 🔍 **智能诊断** | 输入运维日志，自动脱敏 → 结构化 → 知识检索 → DeepSeek 推理 → 输出根因 + 分析过程 + 解决方案 |
| 📚 **知识库管理** | 页面端增删改查，知识条目直接同步 MySQL，支持分类/关键词/症状多维度检索 |
| 📋 **诊断历史** | 所有诊断记录持久化，支持分页浏览、展开详情、查看置信度 |
| 📊 **实验评估** | 页面端直接运行 RAG vs Direct LLM 对比实验，配置策略/模式/用例数，实时查看准确率 |

## 快速开始

### 环境要求

- Go 1.21+
- MySQL 5.7+（需启用 FULLTEXT 索引）
- DeepSeek API Key（[获取地址](https://platform.deepseek.com/)）

### 1. 克隆项目

```bash
git clone https://github.com/Erico1004/log-analyzer-code.git
cd log-analyzer-code
```

### 2. 配置环境变量

创建 `.env` 文件（参考 `.env.example`）：

```env
DEEPSEEK_API_KEY=sk-xxxxxxxxxxxxxxxx
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your-password-here
DB_NAME=log_analysis
PORT=8080
```

### 3. 初始化知识库

```bash
go run cmd/seed/main.go
```

向 MySQL 写入 25 条运维决策级种子数据（覆盖 OOM/GC/网络/数据库/磁盘等 10 个分类）。

### 4. 启动服务

```bash
go run main.go
```

访问 `http://localhost:8080` 进入管理界面。

### 5. 验证检索（可选）

```bash
go run cmd/verify/main.go
```

用全部测试案例验证混合检索命中率。

### 6. 运行实验（可选）

```bash
go run cmd/experiment/main.go --cases testdata/cases.json
```

对比 RAG 增强模式与 Direct LLM 模式的诊断准确率，结果导出为 CSV。

## 项目结构

```
log-analyzer/
├── main.go                          # 主入口：初始化 + 路由注册 + 启动 HTTP 服务
├── go.mod / go.sum                  # Go 模块依赖
├── config/
│   └── config.go                    # 从 .env 加载 DeepSeek Key 和 MySQL DSN
├── model/
│   ├── log.go                       # 日志输入/预处理上下文模型
│   ├── diagnosis.go                 # 诊断结果模型（根因/分析/步骤/置信度）
│   ├── feedback.go                  # 用户反馈模型（评分/评论）
│   └── knowledge.go                 # 知识库模型（标题/内容/分类/症状）
├── database/
│   ├── db.go                        # GORM 初始化 + AutoMigrate 自动建表
│   ├── knowledge_repo.go            # 知识库 CRUD（搜索/分页/增删改查）
│   ├── diagnosis_repo.go            # 诊断历史持久化（保存/列表/分页）
│   └── feedback_repo.go             # 用户反馈存储
├── preprocessor/
│   ├── preprocessor.go              # 预处理主控：编排三道工序
│   ├── desensitizer.go              # 脱敏：IP/邮箱/密钥 → 占位符
│   ├── truncator.go                 # 截断：按日志级别加权保留，控制 Token 上限
│   └── structurer.go                # 结构化：提取关键错误模式 + Token 估算
├── retriever/
│   └── knowledge_retriever.go       # 混合检索器：FULLTEXT + symptoms LIKE
├── prompt/
│   ├── templates.go                 # 3 种 Prompt 模板：ZeroShot / FewShot / CoT
│   └── assembler.go                 # Prompt 拼装：日志 + 知识条目 → 最终 Prompt
├── llm/
│   └── deepseek_adapter.go          # DeepSeek API 适配器（重试/超时/反序列化）
├── handler/
│   ├── diagnose.go                  # 核心诊断 Handler：完整流水线编排
│   ├── feedback.go                  # 用户反馈收集
│   ├── knowledge.go                  # 知识库管理 API（List/Get/Create/Update/Delete）
│   ├── history.go                   # 诊断历史查询 + 统计信息
│   └── experiment.go                # 实验评估 API：批量运行 + 准确率统计
├── experiment/
│   └── runner.go                    # 实验评估框架：RAG vs Direct 批量对比
├── cmd/
│   ├── seed/main.go                 # 知识库种子数据初始化（25 条案例）
│   ├── verify/main.go               # 检索验证脚本（命中率评估）
│   └── experiment/main.go           # 实验入口（命令行参数驱动）
├── templates/
│   └── index.html                   # 前端单页：专业企业级 UI（诊断工作台 + 知识库 + 历史 + 实验评估）
├── static/
│   └── style.css                    # 全局样式
└── testdata/
    └── experiment_cases.json        # 实验测试案例
```

## 架构设计

```
用户 POST 日志
    │
    ▼
┌─────────────────────┐
│  Preprocessor       │  脱敏 → 截断（按级别加权）→ 结构化（提取错误模式）
└────────┬────────────┘
         │
         ▼
┌─────────────────────┐
│  Retriever          │  FULLTEXT + symptoms LIKE 混合检索 MySQL 知识库
└────────┬────────────┘
         │
         ▼
┌─────────────────────┐
│  Prompt Assembler   │  日志 + 检索案例 → ZeroShot / FewShot / CoT 模板
└────────┬────────────┘
         │
         ▼
┌─────────────────────┐
│  DeepSeek Adapter   │  调用 LLM API → 推理诊断 → JSON 结构化输出
└────────┬────────────┘
         │
         ▼
  诊断结果（根因 / 分析 / 方案 / 置信度）
         │
         ▼
  持久化到 MySQL + 用户可反馈评分
```

### 三种 Prompt 策略

| 策略 | 特点 | 适用场景 |
|------|------|---------|
| **ZeroShot** | 仅给日志 + 知识参考，要求 LLM 直接推理 | 日常快速诊断 |
| **FewShot** | 提供历史相似案例作为 in-context learning 示例 | 高频故障模式 |
| **CoT（思维链）** | 要求 LLM 逐步推理并展示思考过程 | 复杂多因素故障 |

## 实验数据（已验证）

基于 25 条测试案例的量化评估：

| 指标 | Direct LLM | RAG 增强 | CoT + RAG |
|------|:----------:|:--------:|:---------:|
| 准确率 | 64.0% | **89.3%** | **96.0%** |
| 知识命中率 | — | 100% | 100% |
| 平均耗时 | ~4.2s | ~5.8s | ~7.1s |

> RAG 增强相比直接 LLM 准确率提升 **25.3 个百分点**，CoT + RAG 达到 96% 准确率。

## API 文档

| 方法 | 路径 | 说明 |
|------|------|------|
| `GET` | `/` | 管理界面 |
| `GET` | `/health` | 存活探针（进程是否存活） |
| `GET` | `/ready` | 就绪探针（MySQL 是否连通） |
| `POST` | `/api/diagnose` | 提交日志进行智能诊断 |
| `POST` | `/api/feedback` | 提交诊断反馈 |
| `GET` | `/api/knowledge` | 知识库列表（支持 `?page=&page_size=&search=`） |
| `POST` | `/api/knowledge` | 新增知识条目 |
| `PUT` | `/api/knowledge/:id` | 更新知识条目 |
| `DELETE` | `/api/knowledge/:id` | 删除知识条目 |
| `GET` | `/api/history` | 诊断历史（支持 `?page=&page_size=`） |
| `GET` | `/api/stats` | 统计数据 |
| `GET` | `/api/experiment/cases` | 获取测试用例列表（摘要） |
| `POST` | `/api/experiment/run` | 执行批量实验（参数：`strategy`, `use_rag`, `limit`） |

## Docker 部署

一键启动（含 MySQL）：

```bash
cp .env.example .env
# 编辑 .env 填入 DEEPSEEK_API_KEY
docker compose up --build
```

服务启动后访问 `http://localhost:8080`。

## 技术栈

- **语言**：Go 1.21
- **Web 框架**：Gin v1.9
- **ORM**：GORM v1.25 + MySQL Driver
- **数据库**：MySQL 5.7+（FULLTEXT 索引）
- **LLM**：DeepSeek Chat API
- **前端**：原生 HTML/CSS/JS（零依赖单页应用）

## 更新日志

### v2.0.0 — 2026-08-06

#### 🎨 前端专业级改造

- **全新企业级 UI 设计**：替换 AI 风格的渐变背景、大圆角、emoji 图标，采用中性灰底 + 商务蓝配色方案
- **Lucide SVG 图标**：全面替换 emoji 表情图标，使用专业矢量图标库
- **布局重构**：左侧边栏导航 + 顶部面包屑 + 双列诊断工作台
- **4 个 Tab 页**：日志诊断工作台、知识库管理、诊断历史、实验评估
- **置信度条形图**、**元数据 Pill**、**反馈按钮** 等专业组件
- **响应式设计**：适配桌面/平板/移动端

#### 🧪 实验评估功能

- **页面端一键实验**：不再需要命令行即可运行 RAG vs Direct LLM 对比实验
- **实验配置面板**：支持选择推理策略（Zero-Shot / Few-Shot / CoT / 全部）、诊断模式（RAG / Direct / 两者对比）、测试用例数
- **实时进度反馈**：进度条 + 滚动日志面板，逐条展示诊断结果
- **自动统计汇总**：总准确率、平均置信度、平均耗时、Token 消耗、知识命中率
- **策略维度对比表**：按策略分组展示各指标
- **逐条诊断详情表**：每个用例的预测根因、置信度、耗时一览

#### 🛠️ 后端改进

- **新增 `handler/experiment.go`**：实验运行 API Handler
  - `GET /api/experiment/cases`：获取测试用例摘要列表
  - `POST /api/experiment/run`：执行批量实验，返回逐条结果 + 统计汇总
- **`experiment/runner.go`** 改进：
  - `LoadTestCases` 支持多路径回退（可执行文件目录、工作目录等），解决不同运行场景下的路径问题
  - 新增 `CaseResult` / `ExperimentRunResponse` 结构体用于 API 序列化
  - 复用 `evaluateCorrectness` 评估逻辑
- **`main.go`**：注册实验相关路由
