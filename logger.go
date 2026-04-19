package main

import (
	"io"
	"log/slog"
	"os"
)

type Logger struct {
	*slog.Logger
}

func newLogger(stdout io.Writer, fileWriter io.Writer) *Logger {
	if stdout == nil {
		stdout = os.Stdout
	}
	var w io.Writer = stdout
	if fileWriter != nil {
		w = io.MultiWriter(stdout, fileWriter)
	}
	var handler slog.Handler
	if os.Getenv("LOG_FORMAT") == "json" {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	} else {
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	return &Logger{slog.New(handler)}
}
