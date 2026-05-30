package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDeterministicEmbeddingProviderStable(t *testing.T) {
	provider := NewDeterministicEmbeddingProvider("test", 16)
	first, err := provider.EmbedTexts(context.Background(), []string{"blue ocean strategy"})
	if err != nil {
		t.Fatalf("embed first: %v", err)
	}
	second, err := provider.EmbedTexts(context.Background(), []string{"blue ocean strategy"})
	if err != nil {
		t.Fatalf("embed second: %v", err)
	}
	if len(first) != 1 || len(first[0].Vector) != 16 {
		t.Fatalf("unexpected first vector: %+v", first)
	}
	for i := range first[0].Vector {
		if first[0].Vector[i] != second[0].Vector[i] {
			t.Fatalf("deterministic vector changed at %d", i)
		}
	}
}

func TestLocalHTTPEmbeddingProvider(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metadata":
			_ = json.NewEncoder(w).Encode(map[string]any{"provider": "local-bge-m3", "model": "BAAI/bge-m3", "dimensions": 4, "normalized": true, "max_batch_size": 2, "max_text_chars": 12000})
		case "/embed":
			_ = json.NewEncoder(w).Encode(map[string]any{"provider": "local-bge-m3", "model": "BAAI/bge-m3", "dimensions": 4, "normalized": true, "vectors": [][]float32{{1, 0, 0, 0}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider := NewLocalHTTPEmbeddingProvider(server.URL, "BAAI/bge-m3", 4, time.Second, 2)
	vectors, err := provider.EmbedTexts(context.Background(), []string{"hello"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0].Vector) != 4 || vectors[0].Vector[0] != 1 {
		t.Fatalf("unexpected vectors: %+v", vectors)
	}
}
