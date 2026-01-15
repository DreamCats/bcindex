package bcindex

import (
	"context"
	"strings"
)

type VectorRuntime struct {
	cfg      VectorConfig
	store    VectorStore
	embedder *VolcesEmbeddingClient
}

func NewVectorRuntime(cfg VectorConfig) (*VectorRuntime, error) {
	embedder, err := NewVolcesEmbeddingClient(cfg)
	if err != nil {
		return nil, err
	}
	store, err := newVectorStore(cfg)
	if err != nil {
		return nil, err
	}
	return &VectorRuntime{
		cfg:      cfg,
		store:    store,
		embedder: embedder,
	}, nil
}

func (r *VectorRuntime) EnsureCollection(ctx context.Context) error {
	return r.store.EnsureCollection(ctx, r.cfg.QdrantCollection, r.cfg.VolcesDimensions)
}

func (r *VectorRuntime) Close() error {
	if r.store == nil {
		return nil
	}
	return r.store.Close()
}

func newVectorStore(cfg VectorConfig) (VectorStore, error) {
	if strings.TrimSpace(cfg.QdrantPath) != "" {
		return NewLocalVectorStore(cfg.QdrantPath)
	}
	return NewQdrantStore(cfg)
}
