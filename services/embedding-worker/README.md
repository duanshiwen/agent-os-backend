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

First run downloads the model into the HuggingFace cache.

## Docker

```bash
docker build -t agentos-embedding-worker services/embedding-worker
docker run -p 8091:8091 agentos-embedding-worker
```

## Environment

```text
EMBEDDING_MODEL=BAAI/bge-m3
EMBEDDING_DEVICE=cpu
EMBEDDING_DIMENSIONS=1024
EMBEDDING_NORMALIZE=true
EMBEDDING_MAX_BATCH_SIZE=16
EMBEDDING_MAX_TEXT_CHARS=12000
```
