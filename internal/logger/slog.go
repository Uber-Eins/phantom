package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/op/go-logging"
)

// slogHandler routes dependencies that use the standard slog package through
// the same x-ui formatter and in-memory buffer as the rest of the panel.
type slogHandler struct {
	attrs  []slog.Attr
	groups []string
}

func newSlogHandler() *slogHandler {
	return &slogHandler{}
}

func (h *slogHandler) Enabled(context.Context, slog.Level) bool {
	return true
}

func (h *slogHandler) Handle(_ context.Context, record slog.Record) error {
	var message strings.Builder
	message.WriteString(record.Message)
	for _, attr := range h.attrs {
		appendSlogAttr(&message, h.groups, attr)
	}
	record.Attrs(func(attr slog.Attr) bool {
		appendSlogAttr(&message, h.groups, attr)
		return true
	})

	level := logging.INFO
	switch {
	case record.Level >= slog.LevelError:
		level = logging.ERROR
	case record.Level >= slog.LevelWarn:
		level = logging.WARNING
	case record.Level <= slog.LevelDebug:
		level = logging.DEBUG
	}
	write(SourceXUI, level, message.String())
	return nil
}

func (h *slogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := *h
	clone.attrs = append(append([]slog.Attr(nil), h.attrs...), attrs...)
	return &clone
}

func (h *slogHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = append(append([]string(nil), h.groups...), name)
	return &clone
}

func appendSlogAttr(message *strings.Builder, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		nextGroups := groups
		if attr.Key != "" {
			nextGroups = append(append([]string(nil), groups...), attr.Key)
		}
		for _, child := range attr.Value.Group() {
			appendSlogAttr(message, nextGroups, child)
		}
		return
	}

	message.WriteByte(' ')
	if len(groups) > 0 {
		message.WriteString(strings.Join(groups, "."))
		message.WriteByte('.')
	}
	message.WriteString(attr.Key)
	message.WriteByte('=')
	if attr.Value.Kind() == slog.KindString {
		message.WriteString(fmt.Sprintf("%q", attr.Value.String()))
		return
	}
	message.WriteString(fmt.Sprint(attr.Value.Any()))
}
