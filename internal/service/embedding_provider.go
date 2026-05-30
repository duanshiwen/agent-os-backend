package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	EmbeddingProviderDisabled      = "disabled"
	EmbeddingProviderDeterministic = "deterministic"
	EmbeddingProviderLocalHTTP     = "local_http"
	DefaultEmbeddingModel          = "BAAI/bge-m3"
	DefaultEmbeddingProviderName   = "local-bge-m3"
	DefaultEmbeddingDimensions     = 1024
)

var ErrEmbeddingUnavailable = errors.New("embedding provider unavailable")

type EmbeddingProvider interface {
	Name() string
	Model() string
	Dimensions() int
	EmbedTexts(ctx context.Context, texts []string) ([]EmbeddingVector, error)
	Health(ctx context.Context) (*EmbeddingProviderHealth, error)
}

type EmbeddingVector struct {
	Text   string
	Vector []float32
}

type EmbeddingProviderHealth struct {
	Status      string `json:"status"`
	ModelLoaded bool   `json:"model_loaded"`
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model"`
	Dimensions  int    `json:"dimensions"`
	Device      string `json:"device,omitempty"`
}

type EmbeddingProviderConfig struct {
	Provider     string
	Endpoint     string
	Model        string
	Dimensions   int
	Timeout      time.Duration
	MaxBatchSize int
}

func NewEmbeddingProvider(cfg EmbeddingProviderConfig) EmbeddingProvider {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = EmbeddingProviderDisabled
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = DefaultEmbeddingModel
	}
	dimensions := cfg.Dimensions
	if dimensions <= 0 {
		dimensions = DefaultEmbeddingDimensions
	}
	switch provider {
	case EmbeddingProviderDeterministic:
		return NewDeterministicEmbeddingProvider(model, dimensions)
	case EmbeddingProviderLocalHTTP:
		return NewLocalHTTPEmbeddingProvider(cfg.Endpoint, model, dimensions, cfg.Timeout, cfg.MaxBatchSize)
	default:
		return DisabledEmbeddingProvider{model: model, dimensions: dimensions}
	}
}

type DisabledEmbeddingProvider struct {
	model      string
	dimensions int
}

func (p DisabledEmbeddingProvider) Name() string    { return EmbeddingProviderDisabled }
func (p DisabledEmbeddingProvider) Model() string   { return p.model }
func (p DisabledEmbeddingProvider) Dimensions() int { return p.dimensions }
func (p DisabledEmbeddingProvider) EmbedTexts(context.Context, []string) ([]EmbeddingVector, error) {
	return nil, ErrEmbeddingUnavailable
}
func (p DisabledEmbeddingProvider) Health(context.Context) (*EmbeddingProviderHealth, error) {
	return &EmbeddingProviderHealth{Status: "disabled", ModelLoaded: false, Provider: p.Name(), Model: p.model, Dimensions: p.dimensions}, nil
}

type DeterministicEmbeddingProvider struct {
	model      string
	dimensions int
}

func NewDeterministicEmbeddingProvider(model string, dimensions int) *DeterministicEmbeddingProvider {
	if model == "" {
		model = "deterministic-test"
	}
	if dimensions <= 0 {
		dimensions = DefaultEmbeddingDimensions
	}
	return &DeterministicEmbeddingProvider{model: model, dimensions: dimensions}
}

func (p *DeterministicEmbeddingProvider) Name() string    { return EmbeddingProviderDeterministic }
func (p *DeterministicEmbeddingProvider) Model() string   { return p.model }
func (p *DeterministicEmbeddingProvider) Dimensions() int { return p.dimensions }
func (p *DeterministicEmbeddingProvider) Health(context.Context) (*EmbeddingProviderHealth, error) {
	return &EmbeddingProviderHealth{Status: "ok", ModelLoaded: true, Provider: p.Name(), Model: p.model, Dimensions: p.dimensions}, nil
}
func (p *DeterministicEmbeddingProvider) EmbedTexts(ctx context.Context, texts []string) ([]EmbeddingVector, error) {
	_ = ctx
	out := make([]EmbeddingVector, 0, len(texts))
	for _, text := range texts {
		out = append(out, EmbeddingVector{Text: text, Vector: deterministicVector(text, p.dimensions)})
	}
	return out, nil
}

func deterministicVector(text string, dimensions int) []float32 {
	if dimensions <= 0 {
		dimensions = DefaultEmbeddingDimensions
	}
	vector := make([]float32, dimensions)
	terms := strings.Fields(strings.ToLower(text))
	if len(terms) == 0 {
		terms = []string{text}
	}
	for _, term := range terms {
		h := sha256.Sum256([]byte(term))
		idx := int(binary.BigEndian.Uint32(h[:4]) % uint32(dimensions))
		sign := float32(1)
		if h[4]%2 == 1 {
			sign = -1
		}
		vector[idx] += sign
	}
	normalizeVector(vector)
	return vector
}

func normalizeVector(vector []float32) {
	var sum float64
	for _, v := range vector {
		sum += float64(v * v)
	}
	if sum == 0 {
		return
	}
	norm := float32(math.Sqrt(sum))
	for i := range vector {
		vector[i] /= norm
	}
}

type LocalHTTPEmbeddingProvider struct {
	endpoint     string
	model        string
	dimensions   int
	maxBatchSize int
	client       *http.Client
}

func NewLocalHTTPEmbeddingProvider(endpoint, model string, dimensions int, timeout time.Duration, maxBatchSize int) *LocalHTTPEmbeddingProvider {
	if model == "" {
		model = DefaultEmbeddingModel
	}
	if dimensions <= 0 {
		dimensions = DefaultEmbeddingDimensions
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if maxBatchSize <= 0 {
		maxBatchSize = 16
	}
	return &LocalHTTPEmbeddingProvider{endpoint: strings.TrimRight(endpoint, "/"), model: model, dimensions: dimensions, maxBatchSize: maxBatchSize, client: &http.Client{Timeout: timeout}}
}

func (p *LocalHTTPEmbeddingProvider) Name() string    { return DefaultEmbeddingProviderName }
func (p *LocalHTTPEmbeddingProvider) Model() string   { return p.model }
func (p *LocalHTTPEmbeddingProvider) Dimensions() int { return p.dimensions }

type localHTTPMetadata struct {
	Provider     string `json:"provider"`
	Model        string `json:"model"`
	Dimensions   int    `json:"dimensions"`
	Normalized   bool   `json:"normalized"`
	MaxBatchSize int    `json:"max_batch_size"`
	MaxTextChars int    `json:"max_text_chars"`
}

func (p *LocalHTTPEmbeddingProvider) Health(ctx context.Context) (*EmbeddingProviderHealth, error) {
	if p.endpoint == "" {
		return nil, fmt.Errorf("%w: EMBEDDING_ENDPOINT is empty", ErrEmbeddingUnavailable)
	}
	meta, err := p.metadata(ctx)
	if err != nil {
		return nil, err
	}
	return &EmbeddingProviderHealth{Status: "ok", ModelLoaded: true, Provider: meta.Provider, Model: meta.Model, Dimensions: meta.Dimensions}, nil
}

func (p *LocalHTTPEmbeddingProvider) EmbedTexts(ctx context.Context, texts []string) ([]EmbeddingVector, error) {
	if p.endpoint == "" {
		return nil, fmt.Errorf("%w: EMBEDDING_ENDPOINT is empty", ErrEmbeddingUnavailable)
	}
	if len(texts) == 0 {
		return []EmbeddingVector{}, nil
	}
	if len(texts) > p.maxBatchSize {
		return nil, fmt.Errorf("embedding batch too large: %d > %d", len(texts), p.maxBatchSize)
	}
	meta, err := p.metadata(ctx)
	if err != nil {
		return nil, err
	}
	if meta.Model != "" && meta.Model != p.model {
		return nil, fmt.Errorf("embedding model mismatch: backend=%s worker=%s", p.model, meta.Model)
	}
	if meta.Dimensions != 0 && meta.Dimensions != p.dimensions {
		return nil, fmt.Errorf("embedding dimensions mismatch: backend=%d worker=%d", p.dimensions, meta.Dimensions)
	}
	body, _ := json.Marshal(map[string]any{"texts": texts, "normalize": true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbeddingUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: embed returned %d", ErrEmbeddingUnavailable, resp.StatusCode)
	}
	var payload struct {
		Provider   string      `json:"provider"`
		Model      string      `json:"model"`
		Dimensions int         `json:"dimensions"`
		Vectors    [][]float32 `json:"vectors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	if payload.Dimensions != 0 && payload.Dimensions != p.dimensions {
		return nil, fmt.Errorf("embedding response dimensions mismatch: backend=%d worker=%d", p.dimensions, payload.Dimensions)
	}
	if len(payload.Vectors) != len(texts) {
		return nil, fmt.Errorf("embedding response count mismatch: texts=%d vectors=%d", len(texts), len(payload.Vectors))
	}
	out := make([]EmbeddingVector, 0, len(texts))
	for i, vector := range payload.Vectors {
		if len(vector) != p.dimensions {
			return nil, fmt.Errorf("embedding vector dimensions mismatch: expected=%d got=%d", p.dimensions, len(vector))
		}
		out = append(out, EmbeddingVector{Text: texts[i], Vector: vector})
	}
	return out, nil
}

func (p *LocalHTTPEmbeddingProvider) metadata(ctx context.Context) (*localHTTPMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/metadata", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmbeddingUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: metadata returned %d", ErrEmbeddingUnavailable, resp.StatusCode)
	}
	var meta localHTTPMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, err
	}
	return &meta, nil
}
