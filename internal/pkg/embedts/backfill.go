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
 * See the License for the LICENSE file for more details.
 */

package embedts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

// LoadtestMetric 是容量回灌专用指标名，不与 SeaTunnel / 作业序列混用。
// LoadtestMetric is the capacity-backfill metric; it is kept off SeaTunnel / job series.
const LoadtestMetric = "stx_loadtest_gauge"

const (
	maxLoadtestSeries = 80_000
	probeSeries       = 1_000
)

// BackfillConfig 控制把约 TargetBytes 的历史块写进 Dir（默认 25GiB / 31d / 15s）。
// BackfillConfig writes about TargetBytes of historical blocks into Dir (default 25GiB / 31d / 15s).
type BackfillConfig struct {
	Dir         string
	TargetBytes int64
	Series      int
	Step        time.Duration
	Block       time.Duration
	Lookback    time.Duration
	Workers     int
	Logger      *slog.Logger
	// Now 固定回灌对齐时刻；零值则用当前 UTC。测试与断点续跑都靠它保持窗口稳定。
	// Now pins the alignment instant; zero means current UTC. Tests and resume keep windows stable with it.
	Now time.Time
}

// BackfillResult 是一次回灌的规模与落盘结果。
// BackfillResult is the scale and on-disk outcome of one backfill.
type BackfillResult struct {
	Dir         string
	Series      int
	Step        time.Duration
	BlockCount  int
	Bytes       int64
	Lookback    time.Duration
	MinTimeMs   int64
	MaxTimeMs   int64
	Elapsed     time.Duration
	ProbeBytes  int64
	TargetBytes int64
}

// WriteLoadtestBlocks 按 2h 块把 stx_loadtest_gauge 历史样本写到 dir，直到接近 TargetBytes。
// 用 BlockWriter 而不是 Head append：31 天前的点会超出 appendable 窗口。
// WriteLoadtestBlocks writes historical stx_loadtest_gauge samples as 2h blocks until TargetBytes.
// BlockWriter is required: samples from 31d ago fall outside the head appendable window.
func WriteLoadtestBlocks(cfg BackfillConfig) (BackfillResult, error) {
	cfg = cfg.withDefaults()
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return BackfillResult{}, err
	}
	endMs := alignBlock(cfg.nowMs(), cfg.Block.Milliseconds())
	startMs := endMs - cfg.Lookback.Milliseconds()
	if startMs >= endMs {
		return BackfillResult{}, fmt.Errorf("lookback %s is shorter than one block", cfg.Lookback)
	}

	probeDir, err := os.MkdirTemp("", "stx-tsdb-probe-*")
	if err != nil {
		return BackfillResult{}, err
	}
	defer os.RemoveAll(probeDir)
	probeCfg := cfg
	probeCfg.Dir = probeDir
	probeMint := endMs - cfg.Block.Milliseconds()
	probeBytes, err := writeLoadtestBlock(probeCfg, probeSeries, probeMint, endMs)
	if err != nil {
		return BackfillResult{}, fmt.Errorf("probe block: %w", err)
	}
	blocks := int((endMs - startMs) / cfg.Block.Milliseconds())
	if blocks < 1 {
		blocks = 1
	}
	series := cfg.Series
	if series <= 0 {
		if probeBytes <= 0 {
			return BackfillResult{}, fmt.Errorf("probe block wrote 0 bytes")
		}
		// 按探测块线性外推序列数；压住上限以免单块索引撑爆 8G API。
		// Scale series from the probe block; cap so one block's index fits an 8G API.
		series = int(math.Ceil(float64(cfg.TargetBytes) / float64(probeBytes) / float64(blocks) * float64(probeSeries)))
		if series < probeSeries {
			series = probeSeries
		}
		if series > maxLoadtestSeries {
			series = maxLoadtestSeries
		}
	}

	started := time.Now()
	logf := cfg.Logger
	logf.Info("loadtest backfill start", "dir", cfg.Dir, "target_bytes", cfg.TargetBytes, "series", series, "blocks", blocks, "probe_bytes", probeBytes)

	// 已有块按 minTime 占住窗口；中断重跑从缺口继续，避免把最老窗口再写一遍就提前凑满 TargetBytes。
	// Existing blocks occupy windows by minTime; a retry fills gaps instead of rewriting the oldest range and hitting TargetBytes early.
	covered := loadCoveredWindows(cfg.Dir)
	jobs := make(chan [2]int64)
	var (
		mu      sync.Mutex
		written int
		first   error
	)
	var wg sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for w := range jobs {
				if dirSize(cfg.Dir) >= cfg.TargetBytes {
					continue
				}
				n, err := writeLoadtestBlock(cfg, series, w[0], w[1])
				mu.Lock()
				if err != nil && first == nil {
					first = err
				} else if err == nil {
					written++
					logf.Info("loadtest block", "n", written, "block_bytes", n, "total_bytes", dirSize(cfg.Dir), "mint", w[0], "maxt", w[1])
				}
				mu.Unlock()
			}
		}()
	}
	for mint := startMs; mint < endMs; mint += cfg.Block.Milliseconds() {
		if firstErr := func() error {
			mu.Lock()
			defer mu.Unlock()
			return first
		}(); firstErr != nil {
			break
		}
		maxt := mint + cfg.Block.Milliseconds()
		if windowCovered(covered, mint, maxt) {
			continue
		}
		if dirSize(cfg.Dir) >= cfg.TargetBytes {
			break
		}
		jobs <- [2]int64{mint, maxt}
	}
	close(jobs)
	wg.Wait()
	if first != nil {
		return BackfillResult{}, first
	}

	total := dirSize(cfg.Dir)
	return BackfillResult{
		Dir:         cfg.Dir,
		Series:      series,
		Step:        cfg.Step,
		BlockCount:  written,
		Bytes:       total,
		Lookback:    cfg.Lookback,
		MinTimeMs:   startMs,
		MaxTimeMs:   endMs,
		Elapsed:     time.Since(started),
		ProbeBytes:  probeBytes,
		TargetBytes: cfg.TargetBytes,
	}, nil
}

func (cfg BackfillConfig) nowMs() int64 {
	now := cfg.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now.UTC().UnixMilli()
}

func (cfg BackfillConfig) withDefaults() BackfillConfig {
	if cfg.TargetBytes <= 0 {
		cfg.TargetBytes = 25 << 30
	}
	if cfg.Step <= 0 {
		cfg.Step = 15 * time.Second
	}
	if cfg.Block <= 0 {
		cfg.Block = 2 * time.Hour
	}
	if cfg.Lookback <= 0 {
		cfg.Lookback = 31 * 24 * time.Hour
	}
	if cfg.Workers < 1 {
		cfg.Workers = 2
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}

func writeLoadtestBlock(cfg BackfillConfig, series int, mint, maxt int64) (int64, error) {
	stepMs := cfg.Step.Milliseconds()
	if stepMs <= 0 {
		return 0, fmt.Errorf("step must be positive")
	}
	before := dirSize(cfg.Dir)
	last := maxt - stepMs
	if last < mint {
		return 0, fmt.Errorf("block window [%d,%d) is shorter than step %d", mint, maxt, stepMs)
	}
	w, err := tsdb.NewBlockWriter(cfg.Logger, cfg.Dir, cfg.Block.Milliseconds())
	if err != nil {
		return 0, err
	}
	defer w.Close()

	ctx := context.Background()
	lsets := make([]labels.Labels, series)
	refs := make([]storage.SeriesRef, series)
	for i := 0; i < series; i++ {
		lsets[i] = labels.FromStrings(
			labels.MetricName, LoadtestMetric,
			"job", "loadtest",
			"series", strconv.Itoa(i),
		)
	}
	app := w.Appender(ctx)
	const commitAfter = 10_000
	sampleCount := 0
	// Head appender 要求时间戳全局前进；按 ts 外层、序列内层写，避免下一条序列回到块起点被判 OOB。
	// The head appender needs globally increasing timestamps; write ts-outer so the next series
	// does not jump back to the block start and trip ErrOutOfBounds.
	for ts := mint; ts <= last; ts += stepMs {
		for i := 0; i < series; i++ {
			refs[i], err = app.Append(refs[i], lsets[i], ts, noisyLoadtestValue(i, ts))
			if err != nil {
				_ = app.Rollback()
				return 0, fmt.Errorf("append series=%d ts=%d: %w", i, ts, err)
			}
			sampleCount++
			if sampleCount > commitAfter {
				if err = app.Commit(); err != nil {
					return 0, err
				}
				app = w.Appender(ctx)
				sampleCount = 0
			}
		}
	}
	if err = app.Commit(); err != nil {
		return 0, err
	}
	if _, err = w.Flush(ctx); err != nil {
		return 0, err
	}
	return dirSize(cfg.Dir) - before, nil
}

func loadCoveredWindows(dir string) [][2]int64 {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out [][2]int64
	for _, e := range entries {
		if !e.IsDir() || len(e.Name()) != 26 {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var meta struct {
			MinTime int64 `json:"minTime"`
			MaxTime int64 `json:"maxTime"`
		}
		if json.Unmarshal(raw, &meta) != nil || meta.MaxTime <= meta.MinTime {
			continue
		}
		out = append(out, [2]int64{meta.MinTime, meta.MaxTime})
	}
	return out
}

func windowCovered(windows [][2]int64, mint, maxt int64) bool {
	for _, w := range windows {
		if mint >= w[0] && mint < w[1] {
			return true
		}
		if w[0] >= mint && w[0] < maxt {
			return true
		}
	}
	return false
}

func alignBlock(ms, blockMs int64) int64 {
	if blockMs <= 0 {
		return ms
	}
	return ms - ms%blockMs
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}

// noisyLoadtestValue 用伪随机 mantissa，降低 XOR 压缩率，才能用可控序列数把块做到数十 GiB。
// noisyLoadtestValue uses a pseudo-random mantissa so XOR compresses poorly and tens of GiB fit in a bounded series count.
func noisyLoadtestValue(series int, ts int64) float64 {
	u := uint64(series+1)*0x9E3779B97F4A7C15 ^ uint64(ts)*0xBF58476D1CE4E5B9
	u ^= u >> 30
	u *= 0xBF58476D1CE4E5B9
	u ^= u >> 27
	return math.Float64frombits(0x3FF0000000000000 | (u >> 12))
}
