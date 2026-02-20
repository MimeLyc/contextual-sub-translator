package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/httpapi"
	"github.com/MimeLyc/contextual-sub-translator/internal/jobs"
	"github.com/MimeLyc/contextual-sub-translator/internal/library"
	"github.com/MimeLyc/contextual-sub-translator/internal/media"
	"github.com/MimeLyc/contextual-sub-translator/internal/persistence"
	"github.com/MimeLyc/contextual-sub-translator/internal/service"
	"github.com/robfig/cron/v3"
)

type scheduler interface {
	Schedule(ctx context.Context) error
}

type cronEngine interface {
	Start()
	Stop() context.Context
}

type httpEngine interface {
	ListenAndServe(addr string) error
	Shutdown(ctx context.Context) error
}

func main() {
	settingsFile := config.RuntimeSettingsFilePath()
	configOpts := make([]config.Option, 0, 1)
	if settings, err := config.LoadRuntimeSettingsFile(settingsFile); err == nil {
		configOpts = append(configOpts, config.WithRuntimeSettings(settings))
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("warning: failed to load runtime settings file %s: %v", settingsFile, err)
	}

	// Initialize configuration
	cfg, err := config.NewFromEnv(configOpts...)
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	settingsStore, err := config.NewRuntimeSettingsStore(settingsFile, cfg.RuntimeSettings())
	if err != nil {
		log.Fatal("Failed to initialize runtime settings store:", err)
	}

	// Create a context that listens for interrupt signals
	ctx, cancel := signalContext(context.Background())
	defer cancel()

	store, err := persistence.NewSQLiteStore(cfg.DBPath())
	if err != nil {
		log.Fatal("Failed to initialize sqlite store:", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			log.Printf("warning: failed to close sqlite store: %v", closeErr)
		}
	}()

	cronScheduler := cron.New()
	jobQueue := jobs.NewQueue(max(1, cfg.Agent.BundleConcurrency), store)
	cronSvc := service.NewRunnableTransServiceWithQueueAndStore(*cfg, cronScheduler, jobQueue, store)

	sourceConfigs := []library.SourceConfig{
		{ID: "movies", Name: "Movies", Path: cfg.Media.MovieDir},
		{ID: "animations", Name: "Animations", Path: cfg.Media.AnimationDir},
		{ID: "teleplays", Name: "Teleplays", Path: cfg.Media.TeleplayDir},
		{ID: "tvshows", Name: "TV Shows", Path: cfg.Media.ShowDir},
		{ID: "documentaries", Name: "Documentaries", Path: cfg.Media.DocumentaryDir},
	}
	scanner := library.NewScanner(
		sourceConfigs,
		cfg.Translate.TargetLanguage,
		library.WithEmbeddedDetector(func(mediaPath string) (bool, bool, []string) {
			descriptions, err := media.NewOperator(mediaPath).ReadSubtitleDescription()
			if err != nil {
				return false, false, nil
			}
			langs := make([]string, 0, len(descriptions))
			seen := make(map[string]bool)
			for _, d := range descriptions {
				lang := d.Language
				if lang == "" {
					lang = "und"
				}
				if !seen[lang] {
					seen[lang] = true
					langs = append(langs, lang)
				}
			}
			return len(descriptions) > 0, descriptions.HasLanguage(cfg.Translate.TargetLanguage), langs
		}),
	)
	httpSrv := httpapi.NewServer(
		scanner,
		jobQueue,
		httpapi.WithJobDataStore(store),
		httpapi.WithRuntimeSettingsStore(settingsStore),
		httpapi.WithRuntimeSettingsApplier(func(next config.RuntimeSettings) error {
			if err := cronSvc.ApplyRuntimeSettings(next); err != nil {
				return err
			}
			return scanner.UpdateTargetLanguage(next.TargetLanguage)
		}),
		httpapi.WithUI(cfg.HTTP.UIStaticDir, cfg.HTTP.UIEnabled),
	)

	err = runWithComponents(ctx, cfg, &cronSvc, cronScheduler, httpSrv)
	if err != nil {
		log.Fatal("Failed to run services:", err)
	}
}

// signalContext returns a context that is cancelled when SIGINT or SIGTERM is received
func signalContext(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-sigChan:
			log.Println("Received interrupt signal")
			cancel()
		case <-ctx.Done():
			// Context was cancelled elsewhere
		}
	}()

	return ctx, cancel
}

func runWithComponents(
	ctx context.Context,
	cfg *config.Config,
	cronSvc scheduler,
	cronScheduler cronEngine,
	httpSrv httpEngine,
) error {
	if err := cronSvc.Schedule(ctx); err != nil {
		return fmt.Errorf("failed to schedule service: %w", err)
	}

	cronScheduler.Start()

	httpErrCh := make(chan error, 1)
	if cfg.HTTP.UIEnabled {
		go func() {
			err := httpSrv.ListenAndServe(cfg.HTTP.Addr)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				httpErrCh <- err
			}
		}()
	}

	log.Println("Service started, waiting for interrupt signal...")

	select {
	case <-ctx.Done():
	case err := <-httpErrCh:
		return fmt.Errorf("http server failed: %w", err)
	}

	log.Println("Received termination signal, shutting down...")
	cronScheduler.Stop()

	if cfg.HTTP.UIEnabled {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := httpSrv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shutdown http server: %w", err)
		}
	}

	log.Println("Service shutdown complete")
	return nil
}
