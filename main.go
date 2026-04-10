package main

import (
	"log"
	"os"
)

func main() {
	authMode := os.Getenv("AUTH_MODE")
	if authMode == "" {
		authMode = "none"
	}
	valid := map[string]bool{"none": true, "password": true, "mtls": true}
	if !valid[authMode] {
		log.Fatalf("invalid AUTH_MODE %q: must be none, password, or mtls", authMode)
	}
	log.Printf("perch starting with AUTH_MODE=%s", authMode)
}
