package main

import (
	"log"

	"github.com/NazarKozynets/http-1.1-api-lab/internal/http1"
)

func main() {
	if err := http1.StartServer(); err != nil {
		log.Fatal(err)
	}
}