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
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

// engineHTTPSinkMissing 是集群缺 HTTP 连接器时引擎 REST v2 的真实响应。
// engineHTTPSinkMissing is the real engine REST v2 response when the cluster lacks the HTTP connector.
const engineHTTPSinkMissing = `sync: submit job failed: status=500 body={"status":"fail","message":"FactoryException: ErrorCode:[API-06], ErrorDescription:[Factory initialize failed] - Unable to create a sink for identifier 'Http'."}`

func TestClassifyEngineSubmitError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantType   string
		wantIdent  string
		wantMissed bool
	}{
		{name: "http sink", err: errors.New(engineHTTPSinkMissing), wantType: "sink", wantIdent: "Http", wantMissed: true},
		{name: "source", err: errors.New("body=Unable to create a source for identifier 'MySQL-CDC'."), wantType: "source", wantIdent: "MySQL-CDC", wantMissed: true},
		{name: "transform mixed case", err: errors.New("unable to create a Transform for identifier 'Sql'"), wantType: "transform", wantIdent: "Sql", wantMissed: true},
		{name: "caused by plugin not found", err: errors.New("Unable to create a sink for identifier 'Http'. | caused by: RuntimeException: Plugin PluginIdentifier{engineType='seatunnel', pluginType='sink', pluginName='Http'} not found."), wantType: "sink", wantIdent: "Http", wantMissed: true},
		{name: "caused by factory discovery miss", err: errors.New("Unable to create a source for identifier 'MySQL-CDC'. Caused by: FactoryException: Could not find any factory for identifier 'MySQL-CDC' that implements 'TableSourceFactory' in the classpath."), wantType: "source", wantIdent: "MySQL-CDC", wantMissed: true},
		// API-06 外层措辞包着深层失败（如 JDBC 连不通）不是缺连接器，必须原样透传。
		// A deeper failure wrapped in the API-06 wording (e.g. JDBC unreachable) is not a missing
		// connector and must pass through unchanged.
		{name: "caused by jdbc unreachable", err: errors.New("Source[0]-MySQL-CDC official tablePath resolution failed: FactoryException: ErrorCode:[API-06], ErrorDescription:[Factory initialize failed] - Unable to create a source for identifier 'MySQL-CDC'. | caused by: CatalogException: ErrorCode:[API-03], ErrorDescription:[Catalog initialize failed] - Failed connecting to the configured JDBC URL via JDBC.")},
		{name: "caused by catalog table miss", err: errors.New("Unable to create a source for identifier 'MySQL-CDC'. | caused by: SeaTunnelException: Can not find catalog table with factoryId [MySQL]")},
		{name: "other failure", err: errors.New("sync: submit job failed: status=500 body=plugin not found")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyEngineSubmitError(tc.err)
			if !tc.wantMissed {
				if got != tc.err {
					t.Fatalf("non-matching errors must be returned unchanged, got %v", got)
				}
				return
			}
			var missing *ConnectorMissingError
			if !errors.As(got, &missing) {
				t.Fatalf("want ConnectorMissingError, got %T %v", got, got)
			}
			if missing.PluginType != tc.wantType || missing.Identifier != tc.wantIdent {
				t.Fatalf("got type=%q ident=%q", missing.PluginType, missing.Identifier)
			}
			if !errors.Is(got, ErrConnectorMissing) || syncErrorCode(got) != ErrorCodeConnectorMissing {
				t.Fatalf("errors.Is / error code must match connector missing")
			}
			if !strings.Contains(got.Error(), tc.err.Error()) {
				t.Fatalf("raw engine message must be kept for troubleshooting: %s", got.Error())
			}
		})
	}
}

func TestClassifyEngineSubmitErrorKeepsUnknownOutcome(t *testing.T) {
	// 结果未知必须原样返回：调用方要登记待确认作业，不能当成确定失败。
	// Unknown outcomes must pass through: callers record a pending job instead of a definite failure.
	unknown := newEngineCallUnknownError(errors.New(engineHTTPSinkMissing), "v2", "http://engine:8080", "http://engine:8080/submit-job")
	if got := classifyEngineSubmitError(unknown); got != unknown {
		t.Fatalf("unknown outcome must be returned unchanged, got %v", got)
	}
	if classifyEngineSubmitError(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}

func TestSubmitConnectorMissingMapsToConflict(t *testing.T) {
	service, engine, task := newOutcomeTestService(t, nil)
	engine.submitErr = errors.New(engineHTTPSinkMissing)
	_, err := service.SubmitTaskWithOptions(context.Background(), task.ID, 1, &SubmitOptions{})
	if !errors.Is(err, ErrConnectorMissing) {
		t.Fatalf("submit must surface ErrConnectorMissing, got %v", err)
	}
	if status := (&Handler{}).getStatusCodeForError(err); status != http.StatusConflict {
		t.Fatalf("status = %d, want 409", status)
	}
}
