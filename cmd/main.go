package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/service"
	"github.com/robfig/cron/v3"
)

func main() {
	// Initialize configuration
	cfg, err := config.NewFromEnv()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Create a context that listens for interrupt signals
	ctx, cancel := signalContext(context.Background())
	defer cancel()

	// Start cron service
	cron := cron.New()
	cronSvc := service.NewRunnableTransService(*cfg, cron)

	err = cronSvc.Schedule(ctx)
	if err != nil {
		log.Fatal("Failed to schedule service:", err)
	}

	cron.Start()

	// Keep the main routine alive until termination
	log.Println("Service started, waiting for interrupt signal...")
	<-ctx.Done()

	log.Println("Received termination signal, shutting down...")

	// Graceful shutdown
	_, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	cron.Stop()

	log.Println("Service shutdown complete")
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
