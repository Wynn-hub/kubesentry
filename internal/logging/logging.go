// Package logging configures the process-wide slog default logger so every
// log line carries the basename of the source file that emitted it.
package logging

import (
	"log/slog"
	"os"
	"path/filepath"
)

// Init installs a text-handler slog default logger with source location
// (file basename + line) attached to every record.
func Init() {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		AddSource:   true,
		ReplaceAttr: shortenSource,
	})
	slog.SetDefault(slog.New(handler))
}

func shortenSource(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.SourceKey {
		if src, ok := a.Value.Any().(*slog.Source); ok {
			src.File = filepath.Base(src.File)
		}
	}
	return a
}
