// Command seedtsdb 向内嵌 tsdb 灌入若干天的作业计数器历史数据，用于本地验证
// 长窗口趋势 / 读写量分桶 / no_data 展示。仅本地开发用，不进交付物。
// Command seedtsdb seeds days of job counter history into the embedded tsdb so long-window
// trends, volume bucketing and no_data rendering can be verified locally. Dev-only helper.
package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"

	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

func main() {
	dir := flag.String("dir", "data/tsdb", "tsdb data dir")
	days := flag.Int("days", 200, "days of history to seed")
	infraDays := flag.Int("infra-days", 0, "days of host/engine series to seed (0 = none)")
	seedJobs := flag.Bool("jobs", true, "seed job counters (set false when job data already exists)")
	interval := flag.Int("interval", 120, "sample interval seconds")
	flag.Parse()

	ctx := context.Background()
	err := embedts.Start(ctx, embedts.Options{
		Dir:          *dir,
		QueryTimeout: 8 * time.Second,
		MaxSamples:   embedts.DefaultMaxSamples,
	})
	if err != nil {
		panic(err)
	}
	defer func() { _ = embedts.Close() }()

	// 播种期间暂停压实，否则 head 落块后更早的样本会因越过 minValidTime 被判 out of bounds。
	// Pause compaction while seeding: once the head gets truncated, earlier samples fall below
	// minValidTime and are rejected as out of bounds.
	db, ok := embedts.AppendableV2().(*tsdb.DB)
	if !ok {
		panic("embedded tsdb not ready")
	}
	db.DisableCompactions()

	end := time.Now()
	start := end.Add(-time.Duration(*days) * 24 * time.Hour)
	stepMs := int64(*interval) * 1000
	points := int(end.Sub(start).Milliseconds() / stepMs)

	type seriesDef struct {
		lset       labels.Labels
		rowsPerSec float64
		phase      float64
		mult       float64
		value      float64
	}
	jobs := []struct {
		jobID, taskID, clusterID string
		rowsPerSec               float64
		phase                    float64
	}{
		{"11", "101", "1", 3200, 0},
		{"12", "102", "1", 1800, 1.1},
		{"13", "103", "1", 950, 2.4},
		{"21", "201", "2", 5400, 0.6},
		{"22", "202", "2", 760, 3.2},
		{"23", "203", "2", 2100, 4.4},
	}
	counters := []struct {
		name string
		mult float64 // rows/s 到该计数器增速的倍率 / multiplier from rows/s to this counter's rate
	}{
		{"stx_job_source_received_rows_total", 1},
		{"stx_job_sink_write_rows_total", 0.97},
		{"stx_job_source_received_bytes_total", 640},
		{"stx_job_sink_write_bytes_total", 626},
	}

	var series []*seriesDef
	for _, j := range jobs {
		for _, c := range counters {
			series = append(series, &seriesDef{
				lset: labels.FromStrings(
					labels.MetricName, c.name,
					"job_id", j.jobID,
					"task_id", j.taskID,
					"cluster_id", j.clusterID,
				),
				rowsPerSec: j.rowsPerSec,
				phase:      j.phase,
				mult:       c.mult,
			})
		}
	}

	// 所有序列共用一个 appender：appender 创建时记录可写下限（head.MaxTime - chunkRange/2），
	// 分多次建 appender 会因第一条序列把 head 推到 "now" 而拒绝旧样本。
	// One appender for every series: the appendable floor (head.MaxTime - chunkRange/2) is
	// captured when the appender is created, so a second appender would reject old samples.
	// gaugeDef：给定取值函数的序列，用于主机 / 引擎类瞬时指标。
	// gaugeDef: series driven by a value function, for host and engine instant metrics.
	type gaugeDef struct {
		lset labels.Labels
		fn   func(ts int64) float64
	}
	var gauges []gaugeDef
	// 主机/引擎序列只灌最近 infraDays 天，不必和作业计数器同长。
	// Host/engine series only span the last infraDays days, not the full job history.
	infraStartMs := end.Add(-time.Duration(*infraDays) * 24 * time.Hour).UnixMilli()

	sine := func(off, amp, phase float64) func(int64) float64 {
		return func(ts int64) float64 {
			day := float64(ts/3600000%24) / 24 * 2 * math.Pi
			return off + amp*math.Sin(day+phase)
		}
	}
	// 主机指标：host_id=1/2 两台裸机（job="stx_host"）。
	// Host metrics for two bare-metal hosts (job="stx_host").
	if *infraDays > 0 {
		for _, h := range []struct {
			id       string
			memTotal float64
			phase    float64
		}{
			{"1", 64 << 30, 0},
			{"2", 32 << 30, 2.1},
		} {
			hostBase := labels.FromStrings("job", "stx_host", "host_id", h.id)
			hadd := func(name string, fn func(int64) float64, extra ...string) {
				b := labels.NewBuilder(hostBase)
				for i := 0; i+1 < len(extra); i += 2 {
					b.Set(extra[i], extra[i+1])
				}
				b.Set(labels.MetricName, name)
				gauges = append(gauges, gaugeDef{lset: b.Labels(), fn: fn})
			}
			for _, mode := range []struct {
				name string
				rate float64
			}{{"user", 0.35}, {"system", 0.12}, {"iowait", 0.03}, {"idle", 0.5}} {
				m := mode
				gauges = append(gauges, gaugeDef{
					lset: labels.FromStrings("job", "stx_host", "host_id", h.id, "mode", m.name, labels.MetricName, "stx_host_cpu_seconds_total"),
					fn: func(ts int64) float64 {
						return m.rate * float64(ts-start.UnixMilli()) / 1000
					},
				})
			}
			hadd("stx_host_memory_bytes", func(int64) float64 { return h.memTotal }, "kind", "total")
			hadd("stx_host_memory_bytes", sine(h.memTotal*0.45, h.memTotal*0.2, h.phase), "kind", "available")
			hadd("stx_host_memory_bytes", sine(h.memTotal*0.1, h.memTotal*0.05, h.phase), "kind", "cached")
			hadd("stx_host_memory_bytes", func(int64) float64 { return 2 << 30 }, "kind", "buffers")
			hadd("stx_host_memory_bytes", sine(h.memTotal*0.3, h.memTotal*0.1, h.phase), "kind", "free")
			for i, k := range []string{"1", "5", "15"} {
				hadd("stx_host_load", sine(4+float64(i), 1.5, h.phase), "kind", k)
			}
			for _, net := range []struct {
				name string
				rate float64
			}{{"stx_host_net_receive_bytes_total", 4 << 20}, {"stx_host_net_transmit_bytes_total", 3 << 20}} {
				n := net
				gauges = append(gauges, gaugeDef{
					lset: labels.FromStrings("job", "stx_host", "host_id", h.id, "iface", "eth0", labels.MetricName, n.name),
					fn: func(ts int64) float64 {
						return n.rate * float64(ts-start.UnixMilli()) / 1000
					},
				})
			}
			for _, d := range []struct {
				name string
				rate float64
			}{{"stx_host_disk_read_bytes_total", 1 << 20}, {"stx_host_disk_write_bytes_total", 8 << 20}, {"stx_host_disk_io_time_seconds_total", 0.25}} {
				dd := d
				gauges = append(gauges, gaugeDef{
					lset: labels.FromStrings("job", "stx_host", "host_id", h.id, "device", "vda", labels.MetricName, dd.name),
					fn: func(ts int64) float64 {
						return dd.rate * float64(ts-start.UnixMilli()) / 1000
					},
				})
			}
			fsTotal := float64(500 << 30)
			hadd("stx_host_fs_size_bytes", func(int64) float64 { return fsTotal }, "mountpoint", "/")
			hadd("stx_host_fs_avail_bytes", sine(fsTotal*0.5, fsTotal*0.1, h.phase), "mountpoint", "/")
			hadd("stx_host_fs_files", func(int64) float64 { return 33e6 }, "mountpoint", "/")
			hadd("stx_host_fs_files_free", func(int64) float64 { return 30e6 }, "mountpoint", "/")
			hadd("stx_host_tcp_established", sine(300, 80, h.phase))
			hadd("stx_host_fds_allocated", sine(900, 200, h.phase))
		}
		// 引擎指标：cluster_id=1 两个实例（job="seatunnel_engine_http"）。
		// Engine metrics: two instances of cluster 1 (job="seatunnel_engine_http").
		for i, inst := range []string{"10.0.0.11:5801", "10.0.0.12:5801"} {
			engBase := labels.FromStrings("job", "seatunnel_engine_http", "cluster_id", "1", "instance", inst)
			eadd := func(name string, fn func(int64) float64, extra ...string) {
				b := labels.NewBuilder(engBase)
				for i := 0; i+1 < len(extra); i += 2 {
					b.Set(extra[i], extra[i+1])
				}
				b.Set(labels.MetricName, name)
				gauges = append(gauges, gaugeDef{lset: b.Labels(), fn: fn})
			}
			heapMax := float64(8 << 30)
			ph := float64(i) * 1.7
			eadd("jvm_memory_bytes_used", sine(heapMax*0.5, heapMax*0.15, ph), "area", "heap")
			eadd("jvm_memory_bytes_max", func(int64) float64 { return heapMax }, "area", "heap")
			eadd("jvm_memory_bytes_used", sine(float64(1<<30), float64(200<<20), ph), "area", "nonheap")
			eadd("jvm_memory_bytes_max", func(int64) float64 { return 2 << 30 }, "area", "nonheap")
			for _, c := range []struct {
				name string
				rate float64
			}{{"process_cpu_seconds_total", 0.45}, {"jvm_gc_collection_seconds_sum", 0.02}, {"job_thread_pool_rejection_total", 0.001}} {
				cc := c
				gauges = append(gauges, gaugeDef{lset: labels.NewBuilder(engBase).Set(labels.MetricName, cc.name).Labels(),
					fn: func(ts int64) float64 {
						return cc.rate * float64(ts-start.UnixMilli()) / 1000
					}})
			}
			eadd("jvm_threads_current", sine(400, 60, ph))
			eadd("jvm_threads_deadlocked", func(int64) float64 { return 0 })
			eadd("process_open_fds", sine(700, 150, ph))
			eadd("process_max_fds", func(int64) float64 { return 65535 })
			eadd("process_start_time_seconds", func(int64) float64 { return float64(start.Unix()) })
			eadd("up", func(int64) float64 { return 1 })
			eadd("cluster_info", func(int64) float64 { return 1 }, "master", map[bool]string{true: "true", false: "false"}[i == 0])
			eadd("hazelcast_executor_queueSize", sine(20, 15, ph))
			eadd("hazelcast_executor_queueRemainingCapacity", func(int64) float64 { return 100 })
			eadd("job_count", sine(6, 2, ph), "type", "running")
			eadd("job_count", func(int64) float64 { return 0 }, "type", "failed")
			eadd("job_count", sine(40, 10, ph), "type", "finished")
		}
	}

	app := embedts.Appender(ctx)
	for i := 0; i < points; i++ {
		ts := start.UnixMilli() + int64(i)*stepMs
		// 日内正弦波动 + 月度慢速漂移，让趋势图有可读的形状。
		// Diurnal sine wave plus a slow monthly drift so charts show a readable shape.
		dayPhase := float64(ts/3600000%24) / 24 * 2 * math.Pi
		monthPhase := float64(ts-start.UnixMilli()) / float64(30*24*time.Hour.Milliseconds())
		if !*seedJobs {
			for _, g := range gauges {
				if ts < infraStartMs {
					break
				}
				if _, err := app.Append(0, g.lset, ts, g.fn(ts)); err != nil {
					panic(err)
				}
			}
			continue
		}
		for _, s := range series {
			rate := s.rowsPerSec * s.mult *
				(0.75 + 0.25*math.Sin(dayPhase+s.phase)) *
				(0.8 + 0.4*math.Sin(monthPhase*2*math.Pi+s.phase))
			s.value += rate * float64(*interval)
			if _, err := app.Append(0, s.lset, ts, s.value); err != nil {
				panic(err)
			}
		}
		for _, g := range gauges {
			if ts < infraStartMs {
				break
			}
			if _, err := app.Append(0, g.lset, ts, g.fn(ts)); err != nil {
				panic(err)
			}
		}
	}
	if err := app.Commit(); err != nil {
		panic(err)
	}
	total := points * (len(series) + len(gauges))

	// 收尾把 head 压成块再放开压实，让服务启动时接近稳态。
	// Flush the head into blocks before re-enabling compaction so the server starts near steady state.
	if err := db.Compact(ctx); err != nil {
		fmt.Printf("compact after seed failed: %v\n", err)
	}
	db.EnableCompactions()
	fmt.Printf("done: %d samples over %d days in %s\n", total, *days, *dir)
}
