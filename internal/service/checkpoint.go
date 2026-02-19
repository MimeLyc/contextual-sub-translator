package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/MimeLyc/contextual-sub-translator/internal/persistence"
)

type batchCheckpointStore interface {
	Load(start, end int) ([]string, bool)
	Save(ctx context.Context, start, end int, translated []string) error
}

type batchCheckpointStoreContextKey struct{}

func withBatchCheckpointStore(ctx context.Context, store batchCheckpointStore) context.Context {
	if store == nil {
		return ctx
	}
	return context.WithValue(ctx, batchCheckpointStoreContextKey{}, store)
}

func batchCheckpointStoreFromContext(ctx context.Context) batchCheckpointStore {
	if ctx == nil {
		return nil
	}
	store, _ := ctx.Value(batchCheckpointStoreContextKey{}).(batchCheckpointStore)
	return store
}

type persistentBatchCheckpointStore struct {
	store *persistence.SQLiteStore
	jobID string

	mu     sync.RWMutex
	cached map[string][]string
}

func newPersistentBatchCheckpointStore(ctx context.Context, store *persistence.SQLiteStore, jobID string) (*persistentBatchCheckpointStore, error) {
	if store == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if jobID == "" {
		return nil, fmt.Errorf("job id is empty")
	}

	checkpoints, err := store.LoadBatchCheckpoints(ctx, jobID)
	if err != nil {
		return nil, err
	}

	cached := make(map[string][]string, len(checkpoints))
	for _, cp := range checkpoints {
		cached[batchKey(cp.BatchStart, cp.BatchEnd)] = append([]string(nil), cp.TranslatedLines...)
	}

	return &persistentBatchCheckpointStore{
		store:  store,
		jobID:  jobID,
		cached: cached,
	}, nil
}

func (s *persistentBatchCheckpointStore) Load(start, end int) ([]string, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ret, ok := s.cached[batchKey(start, end)]
	if !ok {
		return nil, false
	}
	return append([]string(nil), ret...), true
}

func (s *persistentBatchCheckpointStore) Save(ctx context.Context, start, end int, translated []string) error {
	if s == nil {
		return nil
	}
	copyData := append([]string(nil), translated...)
	if err := s.store.SaveBatchCheckpoint(ctx, s.jobID, start, end, copyData); err != nil {
		return err
	}
	s.mu.Lock()
	s.cached[batchKey(start, end)] = copyData
	s.mu.Unlock()
	return nil
}

func batchKey(start, end int) string {
	return fmt.Sprintf("%d:%d", start, end)
}
