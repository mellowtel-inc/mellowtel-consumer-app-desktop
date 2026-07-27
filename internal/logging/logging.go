// Package logging configures structured logging to both stderr and a rotating
// log file in the application config directory.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// LogPath returns the path to the log file inside dir.
func LogPath(dir string) string {
	return filepath.Join(dir, "mellowtel.log")
}

// Setup initialises the global zerolog logger, writing human-readable output to
// stderr and JSON lines to a log file. It returns the log file path and a
// closer that must be called on shutdown.
func Setup(dir string, debug bool) (string, io.Closer, error) {
	path := LogPath(dir)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", nil, fmt.Errorf("open log file %q: %w", path, err)
	}

	console := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "15:04:05"}
	multi := zerolog.MultiLevelWriter(console, file)

	level := zerolog.InfoLevel
	if debug {
		level = zerolog.DebugLevel
	}

	logger := zerolog.New(multi).Level(level).With().Timestamp().Logger()
	log.Logger = logger
	zerolog.DefaultContextLogger = &logger

	return path, file, nil
}
