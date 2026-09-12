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

package otel_trace

import (
	"context"
	"log"
	"sync"

	"github.com/LeonYoah/stx/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	Tracer        trace.Tracer
	shutdownFuncs []func(context.Context) error
	initialized   bool
	initOnce      sync.Once
	enabled       bool
)

// Init initializes the OpenTelemetry tracing based on configuration.
// Init 根据配置初始化 OpenTelemetry 追踪。
// This should be called after config is loaded.
// 这应该在配置加载后调用。
func Init() {
	initOnce.Do(func() {
		// Check if telemetry is enabled in config
		// 检查配置中是否启用了遥测
		if !config.Config.Telemetry.Enabled {
			log.Println("[Trace] OpenTelemetry tracing is disabled / OpenTelemetry 追踪已禁用")
			// Use noop tracer when disabled / 禁用时使用空操作追踪器
			Tracer = noop.NewTracerProvider().Tracer("noop")
			enabled = false
			initialized = true
			return
		}

		log.Println("[Trace] Initializing OpenTelemetry tracing... / 正在初始化 OpenTelemetry 追踪...")

		// 初始化 Propagator
		prop := newPropagator()
		otel.SetTextMapPropagator(prop)

		// 初始化 Trace Provider
		tracerProvider, err := newTracerProvider()
		if err != nil {
			log.Printf("[Trace] Failed to init trace provider, using noop tracer: %v / 初始化追踪提供者失败，使用空操作追踪器: %v", err, err)
			Tracer = noop.NewTracerProvider().Tracer("noop")
			enabled = false
			initialized = true
			return
		}

		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
		otel.SetTracerProvider(tracerProvider)

		// 初始化 Tracer
		Tracer = tracerProvider.Tracer("github.com/LeonYoah/stx")
		enabled = true
		initialized = true
		log.Println("[Trace] OpenTelemetry tracing initialized / OpenTelemetry 追踪已初始化")
	})
}

// IsEnabled returns whether tracing is enabled.
// IsEnabled 返回追踪是否已启用。
func IsEnabled() bool {
	return enabled
}

func Shutdown(ctx context.Context) {
	for _, fn := range shutdownFuncs {
		_ = fn(ctx)
	}
	shutdownFuncs = nil
}

func Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if Tracer == nil {
		// Return noop span if not initialized / 如果未初始化则返回空操作 span
		return ctx, noop.Span{}
	}
	return Tracer.Start(ctx, name, opts...)
}
