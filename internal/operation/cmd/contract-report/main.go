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

package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/LeonYoah/stx/internal/operation"
	"github.com/LeonYoah/stx/internal/operation/contract"
)

func main() {
	write := flag.Bool("write", false, "write the current report to the baseline file")
	allowNewGaps := flag.Bool("allow-new-gaps", false, "allow new historical gaps while writing the baseline")
	repositoryRoot := flag.String("root", ".", "repository root")
	flag.Parse()

	if err := operation.Validate(operation.Registry(), operation.RouteExceptions()); err != nil {
		fail("操作登记校验失败 / operation registry validation failed: %v", err)
	}
	report, err := contract.BuildReport(*repositoryRoot, operation.Registry(), operation.RouteExceptions())
	if err != nil {
		fail("生成报告失败 / failed to build report: %v", err)
	}

	baselinePath := filepath.Join(*repositoryRoot, "internal/operation/testdata/route_baseline.json")
	if *write {
		if err := guardNewHistoricalGaps(baselinePath, report, *allowNewGaps); err != nil {
			fail("拒绝更新基线 / refused to update baseline: %v", err)
		}
		report.BaselineDate = time.Now().Format("2006-01-02")
		content, err := contract.MarshalReport(report)
		if err != nil {
			fail("编码报告失败 / failed to encode report: %v", err)
		}
		if err := os.WriteFile(baselinePath, content, 0o644); err != nil {
			fail("写入报告失败 / failed to write report: %v", err)
		}
	}

	content, err := contract.MarshalReport(report)
	if err != nil {
		fail("编码报告失败 / failed to encode report: %v", err)
	}
	_, _ = os.Stdout.Write(content)
}

func guardNewHistoricalGaps(baselinePath string, current contract.Report, allowNew bool) error {
	previous, err := contract.LoadReport(baselinePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	newGaps := contract.FindNewHistoricalGaps(previous, current)
	if len(newGaps) > 0 && !allowNew {
		return fmt.Errorf("发现新的未登记路由 / new unregistered routes found: %v", newGaps)
	}
	return nil
}

func fail(format string, values ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", values...)
	os.Exit(1)
}
