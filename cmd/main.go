package main

import (
	"Software-Course-Project/internal/config"
	"log"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}
	log.Println("Config loaded successfully:", cfg)
}
