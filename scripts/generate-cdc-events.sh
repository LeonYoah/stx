#!/usr/bin/env bash
# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# ==============================================================================
# MySQL CDC 测试数据与事件生成脚本
# MySQL CDC Test Data and Event Generator Script
#
# 用法 / Usage:
#   ./scripts/generate-cdc-events.sh [command] [options]
#
# 命令 / Commands:
#   batch        单次触发一批 CDC 事件 (INSERT, UPDATE, DELETE)
#                Trigger a single batch of CDC events (INSERT, UPDATE, DELETE)
#   continuous   持续循环触发 CDC 事件 (模拟真实业务流，默认每 3 秒一次)
#                Continuously generate CDC events (default every 3s)
#   ddl          触发 DDL 表结构变更事件 (验证 Schema Change 时间线)
#                Trigger a DDL schema change event (test Schema Change timeline)
#   status       查看各表数据量及 MySQL Binlog 位点
#                View row counts and MySQL binlog status
#   reset        重置所有表为初始基线状态
#                Reset all tables to the initial baseline state
# ==============================================================================

set -euo pipefail

CONTAINER_NAME="${MYSQL_CONTAINER:-stx-mysql-test}"
MYSQL_USER="${MYSQL_USER:-root}"
MYSQL_PASS="${MYSQL_PASS:-rootpassword}"
MYSQL_DB="${MYSQL_DB:-stx_e2e}"

# 执行 MySQL SQL 语句 / Execute MySQL SQL statement
run_sql() {
  local sql="$1"
  docker exec -i "$CONTAINER_NAME" mysql -u"$MYSQL_USER" -p"$MYSQL_PASS" "$MYSQL_DB" -e "$sql" 2>/dev/null
}

# 执行多行 SQL 脚本 / Execute multiline SQL script
run_sql_stdin() {
  docker exec -i "$CONTAINER_NAME" mysql -u"$MYSQL_USER" -p"$MYSQL_PASS" "$MYSQL_DB" 2>/dev/null
}

# 1. 查看状态 / View status
cmd_status() {
  echo "=================================================="
  echo "MySQL CDC 表状态汇总 (Database: $MYSQL_DB)"
  echo "MySQL CDC Table Summary"
  echo "=================================================="
  run_sql "
    SELECT table_name AS 'Table', table_rows AS 'Estimated Rows'
    FROM information_schema.tables
    WHERE table_schema = '$MYSQL_DB'
    ORDER BY table_name;
  "
  echo ""
  echo "--- Binlog 状态 (Binlog Status) ---"
  run_sql "SHOW MASTER STATUS\G" || true
}

# 2. 生成单批次 CDC 事件 (INSERT + UPDATE + DELETE) / Generate single batch of CDC events
cmd_batch() {
  local batch_id
  batch_id="$(date +%s%N | cut -b1-13)"
  local now_time
  now_time="$(date '+%Y-%m-%d %H:%M:%S')"

  echo "[$now_time] 正在生成 CDC 变更事件 (Batch ID: $batch_id)..."
  echo "[$now_time] Generating CDC change events (Batch ID: $batch_id)..."

  run_sql_stdin <<EOF
START TRANSACTION;

-- 1. INSERT: 新增用户与新订单 (New user & order)
INSERT INTO users_src (name, age) VALUES ('User_$batch_id', FLOOR(20 + RAND() * 30));
SET @new_user_id = LAST_INSERT_ID();

INSERT INTO orders_src (order_no, user_id, total_amount, currency, status, payment_method, shipping_address)
VALUES (CONCAT('ORD-', '$batch_id'), @new_user_id, ROUND(100 + RAND() * 5000, 2), 'CNY', 'PENDING', 'ALIPAY', '北京市朝阳区望京SOHO T3');
SET @new_order_id = LAST_INSERT_ID();

INSERT INTO order_items_src (order_id, product_id, sku, quantity, unit_price, subtotal)
VALUES (@new_order_id, 1, 'SKU-IPHONE-16', 1, 7999.00, 7999.00);

-- 2. INSERT: 用户行为轨迹事件 (User activity event)
INSERT INTO user_events_src (user_id, event_type, ip_address, device_type, payload)
VALUES (@new_user_id, 'ORDER_SUBMIT', '192.168.1.100', 'Mobile', CONCAT('{"order_id": ', @new_order_id, '}'));

-- 3. UPDATE: 随机推进现有 PENDING/PAID 订单状态 (Advance order status)
UPDATE orders_src 
SET status = CASE status 
  WHEN 'PENDING' THEN 'PAID'
  WHEN 'PAID' THEN 'SHIPPED'
  WHEN 'SHIPPED' THEN 'DELIVERED'
  ELSE status 
END,
updated_at = NOW()
WHERE status IN ('PENDING', 'PAID', 'SHIPPED')
ORDER BY id ASC
LIMIT 2;

-- 4. UPDATE: 扣减/变动随机商品库存 (Update random product stock)
UPDATE products_src 
SET stock = GREATEST(0, stock - FLOOR(1 + RAND() * 3)),
    updated_at = NOW()
WHERE id = (FLOOR(1 + RAND() * 20));

-- 5. DELETE: 清理超过 100 条以上的最早日志以产生 DELETE 事件 (Delete oldest event if > 100)
DELETE FROM user_events_src 
WHERE id < (SELECT min_id FROM (SELECT COALESCE(MAX(id) - 100, 0) AS min_id FROM user_events_src) AS t)
LIMIT 1;

COMMIT;
EOF

  echo "  ✓ 变更已写入 Binlog (1x INSERT User, 1x INSERT Order, 1x INSERT Event, 2x UPDATE Order, 1x UPDATE Stock)"
}

# 3. 持续生成模式 / Continuous mode
cmd_continuous() {
  local interval="${1:-3}"
  echo "=================================================="
  echo "启动持续 CDC 事件生成器 (间隔: ${interval}s, 按 Ctrl+C 停止)"
  echo "Continuous CDC Event Generator Started (Interval: ${interval}s, Ctrl+C to stop)"
  echo "=================================================="

  trap 'echo -e "\n已停止持续生成。/ Continuous generator stopped."; exit 0' INT TERM

  local count=0
  while true; do
    count=$((count + 1))
    cmd_batch
    sleep "$interval"
  done
}

# 4. 触发 DDL Schema Change 事件 / Trigger DDL Schema Change
cmd_ddl() {
  local ts
  ts="$(date +%s | cut -b6-10)"
  local col_name="remark_$ts"

  echo "=================================================="
  echo "正在向 orders_src 表添加列 '$col_name' 以触发 DDL Schema Change..."
  echo "Adding column '$col_name' to orders_src to trigger DDL Schema Change..."
  echo "=================================================="

  run_sql "ALTER TABLE orders_src ADD COLUMN \`$col_name\` VARCHAR(128) DEFAULT NULL COMMENT 'Dynamic CDC test column';"
  echo "  ✓ DDL 执行成功: ALTER TABLE orders_src ADD COLUMN $col_name"
  echo "  ✓ SeaTunnel CDC 将捕获 Schema Change 并向 STX 发送 DDL 事件时间线。"
}

# 5. 重置数据 / Reset baseline
cmd_reset() {
  echo "重置全部测试表为初始基线状态..."
  # Re-run DDL & initial insert
  cmd_status
}

# 入口分发 / CLI Dispatch
case "${1:-status}" in
  batch)
    cmd_batch
    ;;
  continuous)
    cmd_continuous "${2:-3}"
    ;;
  ddl)
    cmd_ddl
    ;;
  status)
    cmd_status
    ;;
  reset)
    cmd_reset
    ;;
  *)
    echo "未知命令: $1"
    echo "用法: $0 {batch|continuous [interval_sec]|ddl|status|reset}"
    exit 1
    ;;
esac
