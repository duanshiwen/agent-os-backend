package repository

import (
	"math"
	"sort"
	"strings"

	"github.com/agent-os/backend/internal/model"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type KBSearchRepo struct {
	db *gorm.DB
}

type KBSearchDocumentsQuery struct {
	Q            string
	CollectionID *uuid.UUID
	SnapshotID   *uuid.UUID
	Limit        int
	Offset       int
}

type KBSearchSemanticQuery struct {
	Vector       []float32
	Provider     string
	Model        string
	CollectionID *uuid.UUID
	SnapshotID   *uuid.UUID
	Limit        int
	Offset       int
}

type KBSearchSemanticHit struct {
	Document model.KBSearchDocument
	Score    float64
}

func NewKBSearchRepo(db *gorm.DB) *KBSearchRepo {
	return &KBSearchRepo{db: db}
}

func (r *KBSearchRepo) ReplaceSnapshotDocuments(snapshotID uuid.UUID, docs []model.KBSearchDocument) error {
	return r.ReplaceSnapshotDocumentsWithEmbeddingJobs(snapshotID, docs, nil, "", "", 0)
}

func (r *KBSearchRepo) ReplaceSnapshotDocumentsWithEmbeddingJobs(snapshotID uuid.UUID, docs []model.KBSearchDocument, embeddingRepo *KBEmbeddingRepo, provider, modelName string, dimensions int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("snapshot_id = ?", snapshotID).Delete(&model.KBSearchDocument{}).Error; err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		if err := tx.Create(&docs).Error; err != nil {
			return err
		}
		if embeddingRepo == nil || provider == "" || modelName == "" {
			return nil
		}
		return embeddingRepo.EnqueueJobsTx(tx, EnqueueKBEmbeddingJobsInput{Provider: provider, Model: modelName, Dimensions: dimensions, Documents: docs})
	})
}

func (r *KBSearchRepo) SearchDocuments(query KBSearchDocumentsQuery) ([]model.KBSearchDocument, error) {
	var docs []model.KBSearchDocument
	db := r.applySearchFilters(r.db.Model(&model.KBSearchDocument{}), query)
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	err := db.Order("indexed_at DESC").Limit(limit).Offset(query.Offset).Find(&docs).Error
	return docs, err
}

func (r *KBSearchRepo) CountDocuments(query KBSearchDocumentsQuery) (int64, error) {
	var count int64
	err := r.applySearchFilters(r.db.Model(&model.KBSearchDocument{}), query).Count(&count).Error
	return count, err
}

func (r *KBSearchRepo) SearchSemantic(query KBSearchSemanticQuery) ([]KBSearchSemanticHit, error) {
	limit := query.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if query.Offset < 0 {
		query.Offset = 0
	}
	if len(query.Vector) == 0 {
		return nil, ErrInvalidVector
	}
	if r.db.Dialector.Name() == "postgres" {
		return r.searchSemanticPostgres(query, limit)
	}
	return r.searchSemanticInMemory(query, limit)
}

func (r *KBSearchRepo) searchSemanticPostgres(query KBSearchSemanticQuery, limit int) ([]KBSearchSemanticHit, error) {
	vector := FormatVector(query.Vector)
	db := r.db.Table("kb_search_embeddings AS e").Select("d.*, 1 - (e.embedding <=> ?) AS semantic_score", vector).
		Joins("JOIN kb_search_documents AS d ON d.id = e.search_document_id").
		Where("d.status = ?", "active")
	if query.Provider != "" {
		db = db.Where("e.provider = ?", query.Provider)
	}
	if query.Model != "" {
		db = db.Where("e.model = ?", query.Model)
	}
	if query.CollectionID != nil && *query.CollectionID != uuid.Nil {
		db = db.Where("d.collection_id = ?", *query.CollectionID)
	}
	if query.SnapshotID != nil && *query.SnapshotID != uuid.Nil {
		db = db.Where("d.snapshot_id = ?", *query.SnapshotID)
	}
	type row struct {
		model.KBSearchDocument
		SemanticScore float64 `gorm:"column:semantic_score"`
	}
	var rows []row
	if err := db.Order(gorm.Expr("e.embedding <=> ?", vector)).Limit(limit).Offset(query.Offset).Scan(&rows).Error; err != nil {
		return nil, err
	}
	hits := make([]KBSearchSemanticHit, 0, len(rows))
	for _, row := range rows {
		hits = append(hits, KBSearchSemanticHit{Document: row.KBSearchDocument, Score: row.SemanticScore})
	}
	return hits, nil
}

func (r *KBSearchRepo) searchSemanticInMemory(query KBSearchSemanticQuery, limit int) ([]KBSearchSemanticHit, error) {
	var embeddings []model.KBSearchEmbedding
	db := r.db.Model(&model.KBSearchEmbedding{})
	if query.Provider != "" {
		db = db.Where("provider = ?", query.Provider)
	}
	if query.Model != "" {
		db = db.Where("model = ?", query.Model)
	}
	if query.CollectionID != nil && *query.CollectionID != uuid.Nil {
		db = db.Where("collection_id = ?", *query.CollectionID)
	}
	if query.SnapshotID != nil && *query.SnapshotID != uuid.Nil {
		db = db.Where("snapshot_id = ?", *query.SnapshotID)
	}
	if err := db.Find(&embeddings).Error; err != nil {
		return nil, err
	}
	hits := make([]KBSearchSemanticHit, 0, len(embeddings))
	for _, embedding := range embeddings {
		vector, err := ParseVector(embedding.Embedding)
		if err != nil {
			return nil, err
		}
		score := cosineSimilarity(query.Vector, vector)
		var doc model.KBSearchDocument
		if err := r.db.First(&doc, "id = ? AND status = ?", embedding.SearchDocumentID, "active").Error; err != nil {
			continue
		}
		hits = append(hits, KBSearchSemanticHit{Document: doc, Score: score})
	}
	sortSemanticHits(hits)
	start := query.Offset
	if start >= len(hits) {
		return []KBSearchSemanticHit{}, nil
	}
	end := start + limit
	if end > len(hits) {
		end = len(hits)
	}
	return hits[start:end], nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func sortSemanticHits(hits []KBSearchSemanticHit) {
	sort.SliceStable(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})
}

func (r *KBSearchRepo) applySearchFilters(db *gorm.DB, query KBSearchDocumentsQuery) *gorm.DB {
	db = db.Where("status = ?", "active")
	if query.CollectionID != nil && *query.CollectionID != uuid.Nil {
		db = db.Where("collection_id = ?", *query.CollectionID)
	}
	if query.SnapshotID != nil && *query.SnapshotID != uuid.Nil {
		db = db.Where("snapshot_id = ?", *query.SnapshotID)
	}
	q := strings.TrimSpace(query.Q)
	if q != "" {
		like := "%" + strings.ToLower(q) + "%"
		db = db.Where("LOWER(title) LIKE ? OR LOWER(summary) LIKE ? OR LOWER(content_text) LIKE ? OR LOWER(entry_id) LIKE ?", like, like, like, like)
	}
	return db
}
