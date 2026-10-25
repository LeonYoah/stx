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
	"net/http/httptest"
	"testing"
)

func TestSeaTunnelEngineClient_Realtime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ctx/metrics/realtime/jobs":
			_, _ = w.Write([]byte(`{"jobs":[{"jobId":1094861283840000001,"enabled":true,"bucketMs":5000,"retentionMinutes":10,"latestBucketStartMs":1000}],"collector":{"pollIntervalMs":5000}}`))
		case "/ctx/metrics/realtime/jobs/42/vertices":
			if r.URL.Query().Get("windowMs") != "180000" {
				t.Errorf("unexpected windowMs %q", r.URL.Query().Get("windowMs"))
			}
			_, _ = w.Write([]byte(`{"bucketMs":5000,"fromMs":1,"toMs":2,"vertices":[{"vertexId":1,"points":[{"ts":1000,"sourceReadRatio":0.5}]}]}`))
		case "/ctx/metrics/realtime/jobs/42/edges":
			_, _ = w.Write([]byte(`{"bucketMs":5000,"fromMs":1,"toMs":2,"edges":[{"queueId":-5,"targetVertexId":3,"points":[{"ts":1000,"bpRatio":0.6}]}]}`))
		case "/ctx/metrics/realtime/jobs/7/vertices":
			_, _ = w.Write([]byte(`{}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewSeaTunnelEngineClient()
	endpoint := &EngineEndpoint{BaseURL: server.URL, ContextPath: "ctx"}
	ctx := context.Background()

	jobs, err := client.GetRealtimeJobs(ctx, endpoint)
	if err != nil {
		t.Fatalf("GetRealtimeJobs: %v", err)
	}
	if len(jobs.Jobs) != 1 || jobs.Jobs[0].JobID.String() != "1094861283840000001" || !jobs.Jobs[0].Enabled {
		t.Fatalf("unexpected jobs (large id must keep precision): %+v", jobs.Jobs)
	}

	vertices, err := client.GetRealtimeVertices(ctx, endpoint, "42", 180000)
	if err != nil {
		t.Fatalf("GetRealtimeVertices: %v", err)
	}
	if !vertices.Known || len(vertices.Vertices) != 1 || vertices.Vertices[0].VertexID != 1 || vertices.Edges == nil {
		t.Fatalf("unexpected vertices: %+v", vertices)
	}

	edges, err := client.GetRealtimeEdges(ctx, endpoint, "42", 180000)
	if err != nil {
		t.Fatalf("GetRealtimeEdges: %v", err)
	}
	if len(edges.Edges) != 1 || edges.Edges[0].QueueID != -5 || edges.Edges[0].TargetVertexID != 3 {
		t.Fatalf("unexpected edges: %+v", edges)
	}

	unknown, err := client.GetRealtimeVertices(ctx, endpoint, "7", 180000)
	if err != nil {
		t.Fatalf("GetRealtimeVertices unknown: %v", err)
	}
	if unknown.Known || unknown.Vertices == nil {
		t.Fatalf("expected unknown job with empty arrays, got %+v", unknown)
	}
}

func TestSeaTunnelEngineClient_RealtimeErrorMapping(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{name: "non master", status: http.StatusNotFound, body: "<html>Error 404 Not a master node</html>", wantErr: ErrEngineNotMaster},
		{name: "disabled", status: http.StatusServiceUnavailable, body: "Realtime metrics disabled", wantErr: ErrEngineRealtimeDisabled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			_, err := NewSeaTunnelEngineClient().GetRealtimeJobs(context.Background(), &EngineEndpoint{BaseURL: server.URL})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, want %v", err, tc.wantErr)
			}
		})
	}

	t.Run("plain 404 is a generic error", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		defer server.Close()
		_, err := NewSeaTunnelEngineClient().GetRealtimeJobs(context.Background(), &EngineEndpoint{BaseURL: server.URL})
		if err == nil || errors.Is(err, ErrEngineNotMaster) {
			t.Fatalf("expected generic error, got %v", err)
		}
	})
}
