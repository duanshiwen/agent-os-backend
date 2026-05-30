from fastapi import FastAPI, HTTPException

from .config import settings
from .model import EmbeddingModel
from .schema import EmbedRequest, EmbedResponse, HealthResponse, MetadataResponse

app = FastAPI(title="AgentOS Embedding Worker", version="0.1.0")
_model = EmbeddingModel(settings.model_name, settings.device)


@app.get("/health", response_model=HealthResponse)
def health() -> HealthResponse:
    loaded = _model.is_loaded
    return HealthResponse(
        status="ok" if loaded else "warming_up",
        model_loaded=loaded,
        model=settings.model_name,
        dimensions=settings.dimensions,
        device=settings.device,
    )


@app.get("/metadata", response_model=MetadataResponse)
def metadata() -> MetadataResponse:
    return MetadataResponse(
        provider="local-bge-m3",
        model=settings.model_name,
        dimensions=settings.dimensions,
        normalized=settings.normalize,
        max_batch_size=settings.max_batch_size,
        max_text_chars=settings.max_text_chars,
    )


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest) -> EmbedResponse:
    if len(req.texts) > settings.max_batch_size:
        raise HTTPException(status_code=400, detail={"code": "batch_too_large", "message": "too many texts"})
    for text in req.texts:
        if len(text) > settings.max_text_chars:
            raise HTTPException(status_code=400, detail={"code": "text_too_long", "message": "text too long"})
    try:
        vectors = _model.embed(req.texts, normalize=req.normalize)
    except Exception as exc:  # pragma: no cover - keeps API error shape stable
        raise HTTPException(status_code=500, detail={"code": "embedding_failed", "message": str(exc)}) from exc
    return EmbedResponse(
        provider="local-bge-m3",
        model=settings.model_name,
        dimensions=settings.dimensions,
        normalized=req.normalize,
        vectors=vectors,
    )
