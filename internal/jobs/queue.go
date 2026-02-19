package jobs

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/pkg/log"
)

type Executor func(ctx context.Context, job *TranslationJob) error

type Queue struct {
	workerCount int
	maxJobs     int
	store       Store

	mu         sync.RWMutex
	jobs       map[string]*TranslationJob
	dedupe     map[string]string
	idCounter  uint64
	started    bool
	pendingIDs chan string
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

func NewQueue(workerCount int, store Store) *Queue {
	if workerCount <= 0 {
		workerCount = 1
	}
	q := &Queue{
		workerCount: workerCount,
		maxJobs:     1000,
		store:       store,
		jobs:        make(map[string]*TranslationJob),
		dedupe:      make(map[string]string),
		pendingIDs:  make(chan string, 1024),
		stopCh:      make(chan struct{}),
	}
	q.hydrateFromStore(context.Background())
	return q
}

func (q *Queue) Enqueue(req EnqueueRequest) (*TranslationJob, bool) {
	now := time.Now()

	q.mu.Lock()
	if id, ok := q.dedupe[req.DedupeKey]; ok {
		if existing, exists := q.jobs[id]; exists {
			snapshot := cloneJob(existing)
			q.mu.Unlock()
			return snapshot, false
		}
		delete(q.dedupe, req.DedupeKey)
	}

	id := fmt.Sprintf("job-%d", atomic.AddUint64(&q.idCounter, 1))
	job := &TranslationJob{
		ID:        id,
		Source:    req.Source,
		DedupeKey: req.DedupeKey,
		Payload:   req.Payload,
		Status:    StatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}

	q.jobs[id] = job
	if req.DedupeKey != "" {
		q.dedupe[req.DedupeKey] = id
	}
	started := q.started
	snapshot := cloneJob(job)
	q.mu.Unlock()

	q.persistJob(snapshot)
	if started {
		q.enqueuePendingID(id)
	}
	return snapshot, true
}

func (q *Queue) Get(id string) (*TranslationJob, bool) {
	q.mu.RLock()
	job, ok := q.jobs[id]
	q.mu.RUnlock()
	if !ok {
		return nil, false
	}
	return cloneJob(job), true
}

func (q *Queue) List() []*TranslationJob {
	q.mu.RLock()
	defer q.mu.RUnlock()

	ret := make([]*TranslationJob, 0, len(q.jobs))
	for _, job := range q.jobs {
		ret = append(ret, cloneJob(job))
	}
	return ret
}

func (q *Queue) Start(exec Executor) {
	q.mu.Lock()
	if q.started {
		q.mu.Unlock()
		return
	}
	q.started = true

	pending := make([]string, 0)
	for id, job := range q.jobs {
		if job.Status == StatusPending {
			pending = append(pending, id)
		}
	}
	q.mu.Unlock()

	for _, id := range pending {
		q.enqueuePendingID(id)
	}

	for range q.workerCount {
		q.wg.Add(1)
		go q.worker(exec)
	}
}

func (q *Queue) Stop() {
	q.stopOnce.Do(func() {
		close(q.stopCh)
		q.wg.Wait()
	})
}

func (q *Queue) worker(exec Executor) {
	defer q.wg.Done()

	for {
		select {
		case <-q.stopCh:
			return
		case id := <-q.pendingIDs:
			job, ok := q.markRunning(id)
			if !ok {
				continue
			}

			err := exec(context.Background(), job)
			if err != nil {
				q.markFailed(id, err)
				continue
			}
			q.markSuccess(id)
		}
	}
}

func (q *Queue) enqueuePendingID(id string) {
	select {
	case q.pendingIDs <- id:
	default:
		go func() { q.pendingIDs <- id }()
	}
}

func (q *Queue) markRunning(id string) (*TranslationJob, bool) {
	q.mu.Lock()
	job, ok := q.jobs[id]
	if !ok || job.Status != StatusPending {
		q.mu.Unlock()
		return nil, false
	}
	job.Status = StatusRunning
	job.UpdatedAt = time.Now()
	snapshot := cloneJob(job)
	q.mu.Unlock()

	q.persistJob(snapshot)
	return snapshot, true
}

func (q *Queue) markSuccess(id string) {
	q.mu.Lock()
	job, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()
		return
	}
	job.Status = StatusSuccess
	job.Error = ""
	job.UpdatedAt = time.Now()
	q.releaseDedupeLocked(job)
	pruned := q.pruneTerminalJobsLocked()
	snapshot := cloneJob(job)
	q.mu.Unlock()

	q.persistJob(snapshot)
	q.deleteJobsFromStore(pruned)
}

func (q *Queue) markFailed(id string, err error) {
	q.mu.Lock()
	job, ok := q.jobs[id]
	if !ok {
		q.mu.Unlock()
		return
	}
	job.Status = StatusFailed
	if err != nil {
		job.Error = err.Error()
	}
	job.UpdatedAt = time.Now()
	q.releaseDedupeLocked(job)
	pruned := q.pruneTerminalJobsLocked()
	snapshot := cloneJob(job)
	q.mu.Unlock()

	q.persistJob(snapshot)
	q.deleteJobsFromStore(pruned)
}

func (q *Queue) releaseDedupeLocked(job *TranslationJob) {
	if job == nil || job.DedupeKey == "" {
		return
	}
	if id, ok := q.dedupe[job.DedupeKey]; ok && id == job.ID {
		delete(q.dedupe, job.DedupeKey)
	}
}

func (q *Queue) pruneTerminalJobsLocked() []string {
	if q.maxJobs <= 0 || len(q.jobs) <= q.maxJobs {
		return nil
	}

	type candidate struct {
		id        string
		updatedAt time.Time
	}
	terminal := make([]candidate, 0, len(q.jobs))
	for id, job := range q.jobs {
		if job == nil {
			continue
		}
		if job.Status == StatusPending || job.Status == StatusRunning {
			continue
		}
		terminal = append(terminal, candidate{id: id, updatedAt: job.UpdatedAt})
	}
	if len(terminal) == 0 {
		return nil
	}

	sort.Slice(terminal, func(i, j int) bool {
		return terminal[i].updatedAt.Before(terminal[j].updatedAt)
	})

	toRemove := len(q.jobs) - q.maxJobs
	if toRemove <= 0 {
		return nil
	}
	if toRemove > len(terminal) {
		toRemove = len(terminal)
	}

	pruned := make([]string, 0, toRemove)
	for i := 0; i < toRemove; i++ {
		id := terminal[i].id
		job := q.jobs[id]
		if job != nil {
			q.releaseDedupeLocked(job)
		}
		delete(q.jobs, id)
		pruned = append(pruned, id)
	}
	return pruned
}

func (q *Queue) deleteJobsFromStore(ids []string) {
	if q.store == nil || len(ids) == 0 {
		return
	}
	for _, id := range ids {
		if err := q.store.DeleteJobData(context.Background(), id); err != nil {
			log.Error("Failed to delete data for pruned job %s: %v", id, err)
		}
		if err := q.store.DeleteJob(context.Background(), id); err != nil {
			log.Error("Failed to delete pruned job %s from store: %v", id, err)
		}
	}
}

func (q *Queue) hydrateFromStore(ctx context.Context) {
	if q.store == nil {
		return
	}
	loaded, err := q.store.LoadJobs(ctx)
	if err != nil {
		log.Error("Failed to load jobs from store: %v", err)
		return
	}

	now := time.Now()
	toPersist := make([]*TranslationJob, 0)
	q.mu.Lock()
	for _, raw := range loaded {
		if raw == nil || raw.ID == "" {
			continue
		}
		job := cloneJob(raw)
		if job.Status == StatusRunning {
			job.Status = StatusPending
			job.UpdatedAt = now
			toPersist = append(toPersist, cloneJob(job))
		}
		q.jobs[job.ID] = job
		if (job.Status == StatusPending || job.Status == StatusRunning) && job.DedupeKey != "" {
			q.dedupe[job.DedupeKey] = job.ID
		}
		q.updateIDCounterLocked(job.ID)
	}
	q.mu.Unlock()

	for _, job := range toPersist {
		q.persistJob(job)
	}
}

func (q *Queue) updateIDCounterLocked(jobID string) {
	if !strings.HasPrefix(jobID, "job-") {
		return
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(jobID, "job-"), 10, 64)
	if err != nil {
		return
	}
	if n > q.idCounter {
		q.idCounter = n
	}
}

func (q *Queue) persistJob(job *TranslationJob) {
	if q.store == nil || job == nil {
		return
	}
	if err := q.store.UpsertJob(context.Background(), job); err != nil {
		log.Error("Failed to persist job %s: %v", job.ID, err)
	}
}

func cloneJob(job *TranslationJob) *TranslationJob {
	if job == nil {
		return nil
	}
	tmp := *job
	return &tmp
}
