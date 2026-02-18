package main

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeScheduler struct {
	called bool
}

func (f *fakeScheduler) Schedule(context.Context) error {
	f.called = true
	return nil
}

type fakeCron struct {
	started bool
	stopped bool
}

func (f *fakeCron) Start() {
	f.started = true
}

func (f *fakeCron) Stop() context.Context {
	f.stopped = true
	return context.Background()
}

type fakeHTTP struct {
	listenCalled chan struct{}
	shutdownOnce sync.Once
	shutdownCh   chan struct{}
}

func newFakeHTTP() *fakeHTTP {
	return &fakeHTTP{
		listenCalled: make(chan struct{}),
		shutdownCh:   make(chan struct{}),
	}
}

func (f *fakeHTTP) ListenAndServe(string) error {
	close(f.listenCalled)
	<-f.shutdownCh
	return http.ErrServerClosed
}

func (f *fakeHTTP) Shutdown(context.Context) error {
	f.shutdownOnce.Do(func() { close(f.shutdownCh) })
	return nil
}

func TestMain_StartsCronAndHTTP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{
		HTTP: config.HTTPConfig{
			Addr:      "127.0.0.1:0",
			UIEnabled: true,
		},
	}
	scheduler := &fakeScheduler{}
	cronEngine := &fakeCron{}
	httpSrv := newFakeHTTP()

	doneCh := make(chan error, 1)
	go func() {
		doneCh <- runWithComponents(ctx, cfg, scheduler, cronEngine, httpSrv)
	}()

	select {
	case <-httpSrv.listenCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("http server did not start")
	}

	cancel()

	select {
	case err := <-doneCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("runWithComponents did not exit after cancellation")
	}

	assert.True(t, scheduler.called)
	assert.True(t, cronEngine.started)
	assert.True(t, cronEngine.stopped)
}
