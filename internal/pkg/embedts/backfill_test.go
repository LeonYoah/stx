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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteLoadtestBlocks_ResumesMissingWindows(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	cfg := BackfillConfig{
		Dir:         dir,
		TargetBytes: 1 << 30,
		Series:      4,
		Step:        15 * time.Second,
		Block:       2 * time.Hour,
		Lookback:    2 * time.Hour,
		Workers:     1,
		Now:         now,
	}
	first, err := WriteLoadtestBlocks(cfg)
	require.NoError(t, err)
	require.Equal(t, 1, first.BlockCount)
	n1 := countULIDDirs(t, dir)
	require.Equal(t, 1, n1)

	cfg.Lookback = 4 * time.Hour
	second, err := WriteLoadtestBlocks(cfg)
	require.NoError(t, err)
	require.Equal(t, 1, second.BlockCount)
	require.Equal(t, 2, countULIDDirs(t, dir))

	third, err := WriteLoadtestBlocks(cfg)
	require.NoError(t, err)
	require.Equal(t, 0, third.BlockCount)
	require.Equal(t, 2, countULIDDirs(t, dir))
}

func TestWriteLoadtestBlocks_Smoke(t *testing.T) {
	dir := t.TempDir()
	result, err := WriteLoadtestBlocks(BackfillConfig{
		Dir:         dir,
		TargetBytes: 64 << 10,
		Series:      8,
		Step:        15 * time.Second,
		Block:       2 * time.Hour,
		Lookback:    2 * time.Hour,
		Workers:     1,
	})
	require.NoError(t, err)
	require.Equal(t, 8, result.Series)
	require.GreaterOrEqual(t, result.BlockCount, 1)
	require.Greater(t, result.Bytes, int64(0))
	require.Greater(t, countULIDDirs(t, dir), 0)
}

func TestCapacity_Backfill(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("STX_TSDB_BACKFILL"))
	if raw == "" {
		t.Skip("set STX_TSDB_BACKFILL=25g (or 512m) and go test -timeout longer than the write")
	}
	cfg := BackfillConfig{
		Dir:     strings.TrimSpace(os.Getenv("STX_TSDB_BACKFILL_DIR")),
		Workers: 2,
	}
	if cfg.Dir == "" {
		cfg.Dir = filepath.Join("..", "..", "..", "data", "tsdb-staging")
	}
	switch strings.ToLower(raw) {
	case "25g", "25gi", "25gib":
		cfg.TargetBytes = 25 << 30
	case "512m", "512mb", "512mi":
		cfg.TargetBytes = 512 << 20
		cfg.Lookback = 6 * time.Hour
	default:
		if n, err := strconv.ParseInt(strings.TrimSuffix(strings.ToLower(raw), "g"), 10, 64); err == nil && n > 0 {
			cfg.TargetBytes = n << 30
		} else {
			t.Fatalf("invalid STX_TSDB_BACKFILL %q", raw)
		}
	}
	if n := envInt("STX_TSDB_BACKFILL_SERIES", 0); n > 0 {
		cfg.Series = n
	}
	if n := envInt("STX_TSDB_BACKFILL_WORKERS", 0); n > 0 {
		cfg.Workers = n
	}
	t.Logf("backfill dir=%s target=%d", cfg.Dir, cfg.TargetBytes)
	result, err := WriteLoadtestBlocks(cfg)
	require.NoError(t, err)
	require.Greater(t, result.Bytes, int64(0))
	require.GreaterOrEqual(t, result.BlockCount, 1)
	t.Logf("backfill done series=%d blocks=%d bytes=%d elapsed=%s probe=%d",
		result.Series, result.BlockCount, result.Bytes, result.Elapsed, result.ProbeBytes)
}

func countULIDDirs(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	n := 0
	for _, e := range entries {
		if e.IsDir() && len(e.Name()) == 26 {
			n++
		}
	}
	return n
}
