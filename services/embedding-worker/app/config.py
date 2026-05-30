import os
from dataclasses import dataclass


@dataclass(frozen=True)
class Settings:
    model_name: str = os.getenv("EMBEDDING_MODEL", "BAAI/bge-m3")
    device: str = os.getenv("EMBEDDING_DEVICE", "cpu")
    dimensions: int = int(os.getenv("EMBEDDING_DIMENSIONS", "1024"))
    normalize: bool = os.getenv("EMBEDDING_NORMALIZE", "true").lower() in {"1", "true", "yes", "on"}
    max_batch_size: int = int(os.getenv("EMBEDDING_MAX_BATCH_SIZE", "16"))
    max_text_chars: int = int(os.getenv("EMBEDDING_MAX_TEXT_CHARS", "12000"))


settings = Settings()
