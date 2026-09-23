[paths]
data = __GRAFANA_DATA__
logs = __GRAFANA_LOGS__
plugins = __GRAFANA_PLUGINS__
provisioning = __GRAFANA_PROVISIONING__

[server]
http_addr = 0.0.0.0
http_port = 3000
domain = __GRAFANA_DOMAIN__
root_url = __GRAFANA_ROOT_URL__
serve_from_sub_path = true
enforce_domain = false

[security]
admin_user = __GRAFANA_ADMIN_USER__
admin_password = __GRAFANA_ADMIN_PASSWORD__
allow_embedding = true

[users]
allow_sign_up = false

[auth.anonymous]
enabled = true
org_role = Viewer

[live]
# Disable Grafana Live websocket channel by default to reduce iframe embed noise/retry overhead.
# 默认关闭 Grafana Live WebSocket，减少嵌入场景下的重试噪音与开销。
max_connections = 0

[plugins]
preinstall =

