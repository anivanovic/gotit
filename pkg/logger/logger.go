package logger

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func getZapLevel(level string) (zapcore.Level, error) {
	level = strings.TrimSpace(strings.ToLower(level))
	switch level {
	case "debug":
		return zapcore.DebugLevel, nil
	case "info", "":
		return zapcore.InfoLevel, nil
	case "warn":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("unknown log level [debug,info,warn,error]: %q", level)
	}
}

func getZapEncoder(format string) (zapcore.Encoder, error) {
	format = strings.TrimSpace(strings.ToLower(format))
	switch format {
	case "text":
		return zapcore.NewConsoleEncoder(textEncoderConfig), nil
	case "color", "":
		return zapcore.NewConsoleEncoder(colortextEncoderConfig), nil
	case "json":
		return zapcore.NewJSONEncoder(jsonEncoderConfig), nil
	default:
		return nil, fmt.Errorf("unknown log format [text,color,json]: %q", format)
	}
}

var (
	textEncoderConfig = zapcore.EncoderConfig{
		MessageKey:     "M",
		LevelKey:       "L",
		TimeKey:        "T",
		NameKey:        "N",
		CallerKey:      "C",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}

	colortextEncoderConfig = zapcore.EncoderConfig{
		MessageKey:     "M",
		LevelKey:       "L",
		TimeKey:        "T",
		NameKey:        "N",
		CallerKey:      "C",
		StacktraceKey:  "S",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}

	jsonEncoderConfig = zapcore.EncoderConfig{
		MessageKey:     "message",
		LevelKey:       "level",
		TimeKey:        "time",
		NameKey:        "logger",
		CallerKey:      "caller",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.RFC3339TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
		EncodeName:     zapcore.FullNameEncoder,
	}
)

// NewLogger returns a new Logger.
func newLogger(
	level zapcore.Level,
	encoder zapcore.Encoder,
) *zap.Logger {
	return zap.New(
		zapcore.NewCore(
			encoder,
			zapcore.Lock(zapcore.AddSync(os.Stderr)),
			zap.NewAtomicLevelAt(level),
		),
	)
}

// NewCommandLine will return logger configured for terminal.
func NewCommandLine(level string, format string) (*zap.Logger, error) {
	if level == "silent" {
		return zap.NewNop(), nil
	}

	zapLevel, err := getZapLevel(level)
	if err != nil {
		return nil, err
	}
	encoder, err := getZapEncoder(format)
	if err != nil {
		return nil, err
	}
	return newLogger(zapLevel, encoder), nil
}
