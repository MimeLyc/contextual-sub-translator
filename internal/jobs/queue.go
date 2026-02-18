package jobs

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type Executor func(ctx context.Context, job *TranslationJob) error

type Queue struct {
	workerCount int
	maxJobs     int

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

func NewQueue(workerCount int) *Queue {
	if workerCount <= 0 {
		workerCount = 1
	}
	return &Queue{
		workerCount: workerCount,
		maxJobs:     1000,
		jobs:        make(map[string]*TranslationJob),
		dedupe:      make(map[string]string),
		pendingIDs:  make(chan string, 1024),
		stopCh:      make(chan struct{}),
	}
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
	q.dedupe[req.DedupeKey] = id
	started := q.started
	snapshot := cloneJob(job)
	q.mu.Unlock()

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
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok || job.Status != StatusPending {
		return nil, false
	}
	job.Status = StatusRunning
	job.UpdatedAt = time.Now()
	return cloneJob(job), true
}

func (q *Queue) markSuccess(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return
	}
	job.Status = StatusSuccess
	job.Error = ""
	job.UpdatedAt = time.Now()
	q.releaseDedupeLocked(job)
	q.pruneTerminalJobsLocked()
}

func (q *Queue) markFailed(id string, err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	job, ok := q.jobs[id]
	if !ok {
		return
	}
	job.Status = StatusFailed
	if err != nil {
		job.Error = err.Error()
	}
	job.UpdatedAt = time.Now()
	q.releaseDedupeLocked(job)
	q.pruneTerminalJobsLocked()
}

func (q *Queue) releaseDedupeLocked(job *TranslationJob) {
	if job == nil || job.DedupeKey == "" {
		return
	}
	if id, ok := q.dedupe[job.DedupeKey]; ok && id == job.ID {
		delete(q.dedupe, job.DedupeKey)
	}
}

func (q *Queue) pruneTerminalJobsLocked() {
	if q.maxJobs <= 0 || len(q.jobs) <= q.maxJobs {
		return
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
		return
	}

	sort.Slice(terminal, func(i, j int) bool {
		return terminal[i].updatedAt.Before(terminal[j].updatedAt)
	})

	toRemove := len(q.jobs) - q.maxJobs
	if toRemove <= 0 {
		return
	}
	if toRemove > len(terminal) {
		toRemove = len(terminal)
	}

	for i := 0; i < toRemove; i++ {
		id := terminal[i].id
		job := q.jobs[id]
		if job != nil {
			q.releaseDedupeLocked(job)
		}
		delete(q.jobs, id)
	}
}

func cloneJob(job *TranslationJob) *TranslationJob {
	if job == nil {
		return nil
	}
	tmp := *job
	return &tmp
}
