/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package logger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/LeonYoah/stx/agent/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	rootLogger *zap.Logger
	initOnce   sync.Once
	initErr    error
)

// Init 初始化 Agent 日志：
// - 同时输出到控制台和日志文件
// - 日志文件路径使用 cfg.Log.File（默认 /var/log/stx-agent/agent.log）
// API 与 internal/logger 一致：DebugF/InfoF/WarnF/ErrorF(ctx, format, args...)
func Init(cfg *config.Config) error {
	initOnce.Do(func() {
		if cfg == nil {
			initErr = fmt.Errorf("nil config")
			return
		}

		w, err := buildWriter(cfg)
		if err != nil {
			initErr = err
			return
		}

		encoderCfg := zapcore.EncoderConfig{
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

		level := parseLevel(cfg.Log.Level)

		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderCfg),
			w,
			level,
		)

		rootLogger = zap.New(core,
			zap.AddCaller(),
			zap.AddCallerSkip(1),
		)
	})

	return initErr
}

func buildWriter(cfg *config.Config) (zapcore.WriteSyncer, error) {
	logPath := cfg.Log.File
	if logPath == "" {
		logPath = config.DefaultLogFile
	}

	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("[AgentLogger] create log dir err: %w", err)
	}

	fileWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    cfg.Log.MaxSize,
		MaxBackups: cfg.Log.MaxBackups,
		MaxAge:     cfg.Log.MaxAge,
		Compress:   false,
	}

	console := zapcore.AddSync(os.Stdout)
	file := zapcore.AddSync(fileWriter)

	return zapcore.NewMultiWriteSyncer(console, file), nil
}

func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info", "":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "dpanic":
		return zapcore.DPanicLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// L 返回底层 *zap.SugaredLogger，便于在复杂场景下直接使用
func L() *zap.SugaredLogger {
	if rootLogger == nil {
		z, _ := zap.NewDevelopment()
		return z.Sugar()
	}
	return rootLogger.Sugar()
}

// DebugF 与 internal/logger 一致：首参为 context，用于后续扩展（如 trace）
func DebugF(ctx context.Context, format string, args ...interface{}) {
	if rootLogger == nil {
		L().Debugf(format, args...)
		return
	}
	rootLogger.Debug(fmt.Sprintf(format, args...))
}

// InfoF 与 internal/logger 一致
func InfoF(ctx context.Context, format string, args ...interface{}) {
	if rootLogger == nil {
		L().Infof(format, args...)
		return
	}
	rootLogger.Info(fmt.Sprintf(format, args...))
}

// WarnF 与 internal/logger 一致
func WarnF(ctx context.Context, format string, args ...interface{}) {
	if rootLogger == nil {
		L().Warnf(format, args...)
		return
	}
	rootLogger.Warn(fmt.Sprintf(format, args...))
}

// ErrorF 与 internal/logger 一致
func ErrorF(ctx context.Context, format string, args ...interface{}) {
	if rootLogger == nil {
		L().Errorf(format, args...)
		return
	}
	rootLogger.Error(fmt.Sprintf(format, args...))
}
