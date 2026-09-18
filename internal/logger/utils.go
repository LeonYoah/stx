/*
 * MIT License
 *
 * Copyright (c) 2025 linux.do
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

package logger

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/LeonYoah/stx/internal/config"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logWriter         zapcore.WriteSyncer
	initLogWriterOnce sync.Once
	initLogWriterErr  error
)

// GetLogWriter 获取日志输出写入器
func GetLogWriter() (zapcore.WriteSyncer, error) {
	initLogWriterOnce.Do(func() {
		logWriter, initLogWriterErr = initWriter()
	})

	return logWriter, initLogWriterErr
}

func initWriter() (zapcore.WriteSyncer, error) {
	logConfig := config.Config.Log

	var writers []zapcore.WriteSyncer

	// 兼容旧配置：默认 stdout
	output := logConfig.Output
	if output == "" {
		output = "stdout"
	}

	// 是否写入文件（file 或 both）
	if output == "file" || output == "both" {
		logPath := logConfig.FilePath
		if logPath == "" {
			logPath = "./logs/stx.log"
		}

		logDir := filepath.Dir(logPath)
		if err := os.MkdirAll(logDir, 0750); err != nil {
			return nil, fmt.Errorf("[Logger] create log file dir err: %w", err)
		}

		logOutput := &lumberjack.Logger{
			Filename:   logPath,
			MaxSize:    logConfig.MaxSize,
			MaxBackups: logConfig.MaxBackups,
			MaxAge:     logConfig.MaxAge,
			Compress:   logConfig.Compress,
		}
		writers = append(writers, zapcore.AddSync(logOutput))
	}

	// 是否输出到控制台（stdout 或 both）
	if output == "stdout" || output == "both" {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}

	// 兜底：如果配置了未知值，仍然输出到 stdout
	if len(writers) == 0 {
		writers = append(writers, zapcore.AddSync(os.Stdout))
	}

	if len(writers) == 1 {
		return writers[0], nil
	}
	return zapcore.NewMultiWriteSyncer(writers...), nil
}

// getEncoder 获取日志编码器
func getEncoder() zapcore.Encoder {
	// 编码器配置
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	if config.Config.Log.Format == "json" {
		return zapcore.NewJSONEncoder(encoderConfig)
	}
	return zapcore.NewConsoleEncoder(encoderConfig)
}

// getLogLevel 获取日志级别
func getLogLevel() zapcore.Level {
	level := config.Config.Log.Level

	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		log.Fatalf("[Logger] invalid log level: %s\n", level)
		return zapcore.InfoLevel
	}
}

func getTraceIDFields(ctx context.Context) []zap.Field {
	span := trace.SpanFromContext(ctx)
	spanContext := span.SpanContext()
	return []zap.Field{
		zap.String("traceID", spanContext.TraceID().String()),
		zap.String("spanID", spanContext.SpanID().String()),
	}
}
