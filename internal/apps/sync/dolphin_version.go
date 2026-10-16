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

package sync

import (
	"fmt"
	"strconv"
	"strings"
)

// dolphinAPIFamilyFromVersion 把 3.x 版本映射到 process（3.0–3.2）或 workflow（3.3+）API。
// dolphinAPIFamilyFromVersion maps a 3.x version onto the process (3.0–3.2) or workflow (3.3+) API family.
func dolphinAPIFamilyFromVersion(raw string) (string, error) {
	major, minor, _, err := parseDolphinVersion(raw)
	if err != nil {
		return "", err
	}
	if major == 0 && minor == 0 && strings.TrimSpace(raw) == "" {
		return "", nil
	}
	if major != 3 {
		return "", fmt.Errorf("%w: got %s", ErrDolphinUnsupportedVersion, strings.TrimSpace(raw))
	}
	if minor >= 3 {
		return SchedulerAPIStyleWorkflow, nil
	}
	return SchedulerAPIStyleProcess, nil
}

// parseDolphinVersion 解析 3.2.2 / v3.3.1-release 这类版本号；空字符串表示未指定。
// parseDolphinVersion parses versions such as 3.2.2 or v3.3.1-release; empty means unspecified.
func parseDolphinVersion(raw string) (major, minor, patch int, err error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 0, 0, 0, nil
	}
	value = strings.TrimPrefix(strings.ToLower(value), "v")
	value = strings.SplitN(value, "-", 2)[0]
	parts := strings.Split(value, ".")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return 0, 0, 0, fmt.Errorf("%w: invalid version %q", ErrDolphinUnsupportedVersion, raw)
	}
	major, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, 0, fmt.Errorf("%w: invalid version %q", ErrDolphinUnsupportedVersion, raw)
	}
	if len(parts) > 1 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("%w: invalid version %q", ErrDolphinUnsupportedVersion, raw)
		}
	}
	if len(parts) > 2 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return 0, 0, 0, fmt.Errorf("%w: invalid version %q", ErrDolphinUnsupportedVersion, raw)
		}
	}
	if major != 3 {
		return 0, 0, 0, fmt.Errorf("%w: got %s", ErrDolphinUnsupportedVersion, strings.TrimSpace(raw))
	}
	return major, minor, patch, nil
}
