global:
  resolve_timeout: 5m

route:
  receiver: stx-webhook
  group_by: [alertname, cluster, instance]
  group_wait: 30s
  group_interval: 5m
  repeat_interval: 10m

receivers:
  - name: stx-webhook
    webhook_configs:
      - url: __STX_WEBHOOK_URL__
        send_resolved: true
