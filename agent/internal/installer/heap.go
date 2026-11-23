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

package installer

import (
	"fmt"
)

const (
	// legacyHeapGBMax 是历史「按 GB 存储」的上限：1–128 视为 GB，需换算为 MB。
	// legacyHeapGBMax is the legacy GB-storage ceiling: values 1–128 mean GB and must become MB.
	legacyHeapGBMax = 128
	// minHeapMB 是安装向导允许的最小堆（MB），与验收「512 → -Xms512m」对齐。
	// minHeapMB is the minimum wizard heap in MB, matching acceptance "512 → -Xms512m".
	minHeapMB = 512
	// maxHeapMB 是允许的最大堆（128 GiB，以 MB 计）。
	// maxHeapMB is the maximum allowed heap (128 GiB expressed in MB).
	maxHeapMB = 128 * 1024
)

// NormalizeHeapSizeMB 把安装参数中的堆大小规范为 MB。
// 历史控制面/向导按 GB 存 1–128；新契约按 MB 存（≥512）。1–128 乘 1024，其余原样返回。
// NormalizeHeapSizeMB normalizes a heap size to MB. Legacy control-plane/wizard values stored
// 1–128 as GB; the new contract stores MB (≥512). Values 1–128 are multiplied by 1024; others pass through.
func NormalizeHeapSizeMB(value int) int {
	if value <= 0 {
		return value
	}
	if value <= legacyHeapGBMax {
		return value * 1024
	}
	return value
}

// ValidateHeapSizeMB 校验已规范为 MB 的堆大小是否在 [512, 131072] 内。
// ValidateHeapSizeMB checks that a normalized heap size in MB lies in [512, 131072].
func ValidateHeapSizeMB(heapMB int) error {
	if heapMB < minHeapMB {
		return fmt.Errorf("heap size must be at least %d MB (got %d)", minHeapMB, heapMB)
	}
	if heapMB > maxHeapMB {
		return fmt.Errorf("heap size must be at most %d MB (got %d)", maxHeapMB, heapMB)
	}
	return nil
}

// FormatHeapXms 生成 -Xms 标志（始终用 m 后缀，避免误写成 g）。
// FormatHeapXms builds an -Xms flag (always the m suffix so values are never written as g by mistake).
func FormatHeapXms(heapMB int) string {
	return fmt.Sprintf("-Xms%dm", heapMB)
}

// FormatHeapXmx 生成 -Xmx 标志（始终用 m 后缀）。
// FormatHeapXmx builds an -Xmx flag (always the m suffix).
func FormatHeapXmx(heapMB int) string {
	return fmt.Sprintf("-Xmx%dm", heapMB)
}
