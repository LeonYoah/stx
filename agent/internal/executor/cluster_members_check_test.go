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

package executor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func startHazelcastStub(t *testing.T, body string, status int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/hazelcast/rest/cluster" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	_, port, err := net.SplitHostPort(server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func TestHandleCheckClusterMembers_ReportsMembers(t *testing.T) {
	port := startHazelcastStub(t, `{"members":[{"address":"[10.0.0.1]:5801","liteMember":false,"localMember":true},{"address":"[10.0.0.2]:5801","liteMember":false},{"address":"[10.0.0.3]:5801","liteMember":true}]}`, http.StatusOK)

	result, err := handleCheckClusterMembers(context.Background(), map[string]string{"port": port})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %+v", result)
	}
	if result.Details["member_count"] != "3" || result.Details["data_member_count"] != "2" {
		t.Fatalf("unexpected details: %+v", result.Details)
	}
	if result.Details["members"] != "10.0.0.1:5801,10.0.0.2:5801,10.0.0.3:5801" {
		t.Fatalf("expected member addresses, got %q", result.Details["members"])
	}
}

func TestHandleCheckClusterMembers_FailuresAreReported(t *testing.T) {
	if result, _ := handleCheckClusterMembers(context.Background(), map[string]string{"port": "abc"}); result.Success {
		t.Fatal("invalid port must fail")
	}
	port := startHazelcastStub(t, `{"members":[]}`, http.StatusOK)
	if result, _ := handleCheckClusterMembers(context.Background(), map[string]string{"port": port}); result.Success {
		t.Fatal("empty member list must fail")
	}
	port = startHazelcastStub(t, `oops`, http.StatusInternalServerError)
	if result, _ := handleCheckClusterMembers(context.Background(), map[string]string{"port": port}); result.Success {
		t.Fatal("HTTP error must fail")
	}
}
