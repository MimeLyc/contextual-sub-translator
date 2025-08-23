package main

import (
	"context"
	"log"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
	"github.com/MimeLyc/contextual-sub-translator/internal/service"
	"github.com/robfig/cron/v3"
)

func main() {
	// Initialize configuration
	cfg, err := config.New()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	cron := cron.New()
	cronSvc := service.NewRunnableTransService(*cfg, cron)

	err = cronSvc.Schedule(context.Background())
	if err != nil {
		panic(err)
	}
}
