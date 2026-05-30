from functools import cached_property


class EmbeddingModel:
    def __init__(self, model_name: str, device: str = "cpu") -> None:
        self.model_name = model_name
        self.device = device

    @cached_property
    def model(self):
        from sentence_transformers import SentenceTransformer

        return SentenceTransformer(self.model_name, device=self.device)

    @property
    def is_loaded(self) -> bool:
        return "model" in self.__dict__

    def embed(self, texts: list[str], normalize: bool = True) -> list[list[float]]:
        vectors = self.model.encode(texts, normalize_embeddings=normalize)
        return vectors.tolist()
