package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

type Format int

const (
	FormatText Format = iota
	FormatJSON
)

var (
	mu            sync.Mutex
	configuredLvl           = LevelInfo
	configuredFmt           = FormatText
	output        io.Writer = os.Stdout
)

// Configure sets the global logger behavior and routes standard library logs
// through this package so old log.Printf calls remain consistent.
func Configure(levelRaw, formatRaw string) {
	mu.Lock()
	defer mu.Unlock()

	configuredLvl = parseLevel(levelRaw)
	configuredFmt = parseFormat(formatRaw)

	log.SetFlags(0)
	log.SetOutput(stdLogAdapter{})

	emitLocked(LevelInfo, "logger configured", map[string]any{
		"level":  configuredLvl.String(),
		"format": configuredFmt.String(),
	})
}

func Debugf(format string, args ...any) {
	logf(LevelDebug, format, args...)
}

func Infof(format string, args ...any) {
	logf(LevelInfo, format, args...)
}

func Warnf(format string, args ...any) {
	logf(LevelWarn, format, args...)
}

func Errorf(format string, args ...any) {
	logf(LevelError, format, args...)
}

func Debugw(message string, fields map[string]any) {
	logw(LevelDebug, message, fields)
}

func Infow(message string, fields map[string]any) {
	logw(LevelInfo, message, fields)
}

func Warnw(message string, fields map[string]any) {
	logw(LevelWarn, message, fields)
}

func Errorw(message string, fields map[string]any) {
	logw(LevelError, message, fields)
}

func logf(level Level, format string, args ...any) {
	logw(level, fmt.Sprintf(format, args...), nil)
}

func logw(level Level, message string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()

	if level < configuredLvl {
		return
	}

	emitLocked(level, message, fields)
}

func emitLocked(level Level, message string, fields map[string]any) {
	ts := time.Now().UTC().Format(time.RFC3339Nano)

	if configuredFmt == FormatJSON {
		payload := map[string]any{
			"ts":    ts,
			"level": level.String(),
			"msg":   message,
		}
		for k, v := range fields {
			if strings.TrimSpace(k) == "" {
				continue
			}
			payload[k] = v
		}

		encoded, err := json.Marshal(payload)
		if err != nil {
			_, _ = fmt.Fprintf(output, "ts=%s level=ERROR msg=%s\n", ts, strconv.Quote("log marshal error: "+err.Error()))
			return
		}

		_, _ = fmt.Fprintln(output, string(encoded))
		return
	}

	var b strings.Builder
	b.WriteString("ts=")
	b.WriteString(ts)
	b.WriteString(" level=")
	b.WriteString(strings.ToLower(level.String()))
	b.WriteString(" msg=")
	b.WriteString(strconv.Quote(message))

	keys := make([]string, 0, len(fields))
	for k := range fields {
		if strings.TrimSpace(k) == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		b.WriteString(" ")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(formatValue(fields[k]))
	}

	_, _ = fmt.Fprintln(output, b.String())
}

func formatValue(v any) string {
	switch val := v.(type) {
	case nil:
		return "null"
	case string:
		return strconv.Quote(val)
	case error:
		return strconv.Quote(val.Error())
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func parseLevel(raw string) Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return LevelDebug
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

func parseFormat(raw string) Format {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "json":
		return FormatJSON
	default:
		return FormatText
	}
}

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "INFO"
	}
}

func (f Format) String() string {
	if f == FormatJSON {
		return "json"
	}
	return "text"
}

type stdLogAdapter struct{}

func (stdLogAdapter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}

	mu.Lock()
	defer mu.Unlock()
	emitLocked(LevelInfo, msg, map[string]any{"source": "stdlog"})

	return len(p), nil
}
