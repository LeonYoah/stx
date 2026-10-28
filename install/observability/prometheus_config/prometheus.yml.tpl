global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - __RULE_FILES__

alerting:
  alertmanagers:
    - static_configs:
        - targets: ["__ALERTMANAGER_TARGET__"]

scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets: ["__PROMETHEUS_TARGET__"]

  - job_name: alertmanager
    static_configs:
      - targets: ["__ALERTMANAGER_TARGET__"]

  # SeaTunnel targets come from STX HTTP SD.
  # SeaTunnel 抓取目标由 STX HTTP SD 提供。
  - job_name: seatunnel_engine_http
    metrics_path: /hazelcast/rest/instance/metrics
    http_sd_configs:
      - url: __STX_SD_URL__
        refresh_interval: 30s
    relabel_configs:
      - source_labels: [job]
        regex: seatunnel_engine_http
        action: keep

  # STX job-level metrics exported by the control plane.
  # STX 控制面导出的作业级指标。
  - job_name: stx_sync_jobs
    metrics_path: /api/v1/monitoring/prometheus/jobs/metrics
    http_sd_configs:
      - url: __STX_SD_URL__
        refresh_interval: 30s
    relabel_configs:
      - source_labels: [job]
        regex: stx_sync_jobs
        action: keep
