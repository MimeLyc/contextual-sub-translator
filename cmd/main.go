package main

import (
	"fmt"
	"log"

	"github.com/MimeLyc/contextual-sub-translator/internal/config"
)

func main() {
	// Initialize configuration
	cfg, err := config.New()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}
	fmt.Println(cfg)
}
