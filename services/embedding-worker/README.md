# AgentOS Embedding Worker

Local HTTP embedding worker for KB Hub semantic search.

## Default

- Model: `BAAI/bge-m3`
- Dimensions: `1024`
- API: FastAPI

## Endpoints

- `GET /health`
- `GET /metadata`
- `POST /embed`

## Run locally

```bash
cd services/embedding-worker
pip install .
uvicorn app.main:app --host 0.0.0.0 --port 8091
```

First run downloads the model into the HuggingFace cache. BGE-M3 is a multi-GB model, so make sure the cache location has several GB free.

## Docker

```bash
docker build -t agentos-embedding-worker services/embedding-worker
docker run -p 8091:8091 \
  -e HF_HOME=/models/huggingface \
  -e TRANSFORMERS_CACHE=/models/huggingface/transformers \
  -e SENTENCE_TRANSFORMERS_HOME=/models/sentence-transformers \
  -v "$(pwd)/.cache/embedding-models:/models" \
  agentos-embedding-worker
```

Docker Compose uses `${EMBEDDING_MODEL_CACHE_DIR:-./.cache/embedding-models}` as a host bind-mounted cache by default. This avoids Docker named-volume disk limits on local machines.

## Environment

```text
EMBEDDING_MODEL=BAAI/bge-m3
EMBEDDING_DEVICE=cpu
EMBEDDING_DIMENSIONS=1024
EMBEDDING_NORMALIZE=true
EMBEDDING_MAX_BATCH_SIZE=16
EMBEDDING_MAX_TEXT_CHARS=12000
```
