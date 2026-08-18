-- pgvector 扩展
CREATE EXTENSION IF NOT EXISTS vector;

-- 知识表（与 model.KnowledgeBase 对齐）
CREATE TABLE IF NOT EXISTS knowledge_base (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    category VARCHAR(50),
    keywords TEXT,
    symptoms TEXT,
    embedding vector(1024),
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- HNSW 向量索引（O(log n) ANN 检索）
CREATE INDEX IF NOT EXISTS idx_kb_embedding_hnsw
ON knowledge_base USING hnsw (embedding vector_cosine_ops)
WITH (m = 16, ef_construction = 64);

-- GIN 全文检索索引
CREATE INDEX IF NOT EXISTS idx_kb_fts
ON knowledge_base USING gin (
    to_tsvector('simple', coalesce(content, '') || ' ' || coalesce(keywords, ''))
);

-- 诊断历史表（与 model.DiagnosisHistory 对齐，表名单数）
CREATE TABLE IF NOT EXISTS diagnosis_history (
    id BIGSERIAL PRIMARY KEY,
    session_id CHAR(36) NOT NULL,
    log_hash CHAR(64) NOT NULL,
    log_snippet TEXT,
    retrieved_doc_ids VARCHAR(500),
    diagnosis_result JSON NOT NULL,
    model_used VARCHAR(50) NOT NULL,
    prompt_strategy VARCHAR(20) NOT NULL,
    total_tokens BIGINT,
    latency_ms BIGINT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_diagnosis_history_session_id
ON diagnosis_history (session_id);

-- 用户反馈表（与 model.UserFeedback 对齐，表名单数）
CREATE TABLE IF NOT EXISTS user_feedback (
    id BIGSERIAL PRIMARY KEY,
    session_id CHAR(36) NOT NULL,
    feedback SMALLINT NOT NULL,
    user_comment TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_user_feedback_session_id
ON user_feedback (session_id);
