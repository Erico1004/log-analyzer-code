-- pgvector 扩展
CREATE EXTENSION IF NOT EXISTS vector;

-- 知识表
CREATE TABLE IF NOT EXISTS knowledge_base (
    id SERIAL PRIMARY KEY,
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

-- 诊断历史表
CREATE TABLE IF NOT EXISTS diagnosis_histories (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(36) NOT NULL,
    log_hash VARCHAR(64) NOT NULL,
    log_snippet TEXT,
    retrieved_doc_ids VARCHAR(255),
    diagnosis_result JSONB,
    model_used VARCHAR(50),
    prompt_strategy VARCHAR(20),
    total_tokens INT DEFAULT 0,
    latency_ms INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);

-- 用户反馈表
CREATE TABLE IF NOT EXISTS user_feedbacks (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(36),
    feedback_type VARCHAR(20),
    comment TEXT,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
