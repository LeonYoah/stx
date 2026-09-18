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

package collector

import (
	"net"
	"testing"
)

// TestGetOutboundIP tests the kernel routing outbound IP resolution.
// TestGetOutboundIP 测试内核路由出口 IP 解析。
func TestGetOutboundIP(t *testing.T) {
	c := NewMetricsCollector(nil)

	// 1. Test empty target address
	// 1. 测试空目标地址
	if ip := c.GetOutboundIP(""); ip != "" {
		t.Errorf("Expected empty IP for empty target, got %s", ip)
	}

	// 2. Test localhost target address
	// 2. 测试本地回环目标地址
	ip := c.GetOutboundIP("127.0.0.1:9000")
	if ip != "127.0.0.1" {
		t.Errorf("Expected 127.0.0.1 for localhost target, got %s", ip)
	}

	// 3. Test valid IP format for remote address without network traffic
	// 3. 测试远程地址返回合法的 IP 格式（不产生实际网络发包）
	remoteIP := c.GetOutboundIP("8.8.8.8:53")
	if remoteIP != "" {
		parsed := net.ParseIP(remoteIP)
		if parsed == nil {
			t.Errorf("Expected valid IP format, got %s", remoteIP)
		}
	}
}

// TestListLocalIPAddresses 验证本机地址枚举不含回环/未指定地址。
// TestListLocalIPAddresses verifies local address listing excludes loopback/unspecified.
func TestListLocalIPAddresses(t *testing.T) {
	c := NewMetricsCollector(nil)
	ips := c.ListLocalIPAddresses()
	for _, ip := range ips {
		parsed := net.ParseIP(ip)
		if parsed == nil {
			t.Fatalf("invalid IP returned: %s", ip)
		}
		if parsed.IsLoopback() || parsed.IsUnspecified() || parsed.IsLinkLocalUnicast() {
			t.Fatalf("unexpected non-routable IP in list: %s", ip)
		}
	}
}
