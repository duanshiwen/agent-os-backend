from pydantic import BaseModel, Field


class HealthResponse(BaseModel):
    status: str
    model_loaded: bool
    model: str
    dimensions: int
    device: str


class MetadataResponse(BaseModel):
    provider: str
    model: str
    dimensions: int
    normalized: bool
    max_batch_size: int
    max_text_chars: int


class EmbedRequest(BaseModel):
    texts: list[str] = Field(default_factory=list)
    normalize: bool = True


class EmbedResponse(BaseModel):
    provider: str
    model: str
    dimensions: int
    normalized: bool
    vectors: list[list[float]]
