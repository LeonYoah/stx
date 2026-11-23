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

package cluster

import (
	"context"
	"errors"
	"testing"
)

func TestRemoveNodeRejectsRunningProcess(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	svc := NewService(repo, NewMockHostProvider(), nil)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "remove-running-guard",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node := &ClusterNode{
		ClusterID:     cluster.ID,
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		APIPort:       8080,
		Status:        NodeStatusRunning,
		ProcessPID:    4242,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("add node: %v", err)
	}

	if err := svc.RemoveNode(ctx, cluster.ID, node.ID); !errors.Is(err, ErrNodeStillRunning) {
		t.Fatalf("expected ErrNodeStillRunning, got %v", err)
	}
	still, err := repo.GetNodeByID(ctx, node.ID)
	if err != nil || still == nil {
		t.Fatalf("running node must remain after rejected remove: %v", err)
	}
}

func TestDeleteClusterRejectsWhenNodeStillRunningDespiteStoppedCluster(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	svc := NewService(repo, NewMockHostProvider(), nil)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "delete-running-node-guard",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	if err := svc.UpdateStatus(ctx, cluster.ID, ClusterStatusError); err != nil {
		t.Fatalf("set cluster error status: %v", err)
	}
	node := &ClusterNode{
		ClusterID:     cluster.ID,
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		Status:        NodeStatusRunning,
		ProcessPID:    777,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("add node: %v", err)
	}

	if err := svc.Delete(ctx, cluster.ID, false); !errors.Is(err, ErrClusterHasRunningTask) {
		t.Fatalf("expected ErrClusterHasRunningTask, got %v", err)
	}
	if _, err := svc.Get(ctx, cluster.ID); err != nil {
		t.Fatalf("cluster must remain after rejected delete: %v", err)
	}
}

func TestRemoveNodeAllowsStoppedNode(t *testing.T) {
	db, cleanup := setupServiceTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	svc := NewService(repo, NewMockHostProvider(), nil)
	ctx := context.Background()

	cluster, err := svc.Create(ctx, &CreateClusterRequest{
		Name:           "remove-stopped-ok",
		DeploymentMode: DeploymentModeHybrid,
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	node := &ClusterNode{
		ClusterID:     cluster.ID,
		HostID:        1,
		Role:          NodeRoleMasterWorker,
		InstallDir:    "/opt/seatunnel",
		HazelcastPort: 5801,
		Status:        NodeStatusStopped,
		ProcessPID:    0,
	}
	if err := repo.AddNode(ctx, node); err != nil {
		t.Fatalf("add node: %v", err)
	}
	if err := svc.RemoveNode(ctx, cluster.ID, node.ID); err != nil {
		t.Fatalf("stopped node must be removable: %v", err)
	}
}
