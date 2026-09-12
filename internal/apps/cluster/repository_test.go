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
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB(t *testing.T) (*gorm.DB, func()) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "cluster_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tempDir, "test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	// Auto-migrate the Cluster and ClusterNode models
	if err := db.AutoMigrate(&Cluster{}, &ClusterNode{}); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to migrate: %v", err)
	}

	cleanup := func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		os.RemoveAll(tempDir)
	}

	return db, cleanup
}

// genValidClusterName generates valid cluster names (alphanumeric, 1-100 chars)
func genValidClusterName() gopter.Gen {
	return gen.RegexMatch("[a-zA-Z][a-zA-Z0-9_-]{0,99}").SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 100
	})
}

// genDeploymentMode generates valid deployment modes
func genDeploymentMode() gopter.Gen {
	return gen.OneConstOf(DeploymentModeHybrid, DeploymentModeSeparated)
}

// **Feature: stx-agent, Property 10: Cluster Name Uniqueness Validation**
// **Validates: Requirements 7.1**
// For any cluster creation request, if a cluster with the same name already exists,
// the system SHALL reject the creation and return an error indicating the name conflict.

func TestProperty_ClusterNameUniqueness(t *testing.T) {
	// **Feature: stx-agent, Property 10: Cluster Name Uniqueness Validation**
	// **Validates: Requirements 7.1**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("duplicate cluster names are rejected", prop.ForAll(
		func(name string, mode1 DeploymentMode, mode2 DeploymentMode) bool {
			// Setup fresh database for each test
			db, cleanup := setupTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			ctx := context.Background()

			// Create first cluster
			cluster1 := &Cluster{
				Name:           name,
				DeploymentMode: mode1,
			}
			err := repo.Create(ctx, cluster1)
			if err != nil {
				t.Logf("Failed to create first cluster: %v", err)
				return false
			}

			// Attempt to create second cluster with same name but different deployment mode
			cluster2 := &Cluster{
				Name:           name,
				DeploymentMode: mode2,
			}
			err = repo.Create(ctx, cluster2)

			// Should return ErrClusterNameDuplicate
			return errors.Is(err, ErrClusterNameDuplicate)
		},
		genValidClusterName(),
		genDeploymentMode(),
		genDeploymentMode(),
	))

	properties.TestingRun(t)
}

// TestProperty_ClusterNameUniquenessOnUpdate tests that updating a cluster to a duplicate name is rejected
func TestProperty_ClusterNameUniquenessOnUpdate(t *testing.T) {
	// **Feature: stx-agent, Property 10: Cluster Name Uniqueness Validation**
	// **Validates: Requirements 7.1**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("updating to duplicate cluster name is rejected", prop.ForAll(
		func(name1 string, name2 string, mode DeploymentMode) bool {
			// Skip if names are the same (not a duplicate scenario)
			if name1 == name2 {
				return true
			}

			// Setup fresh database for each test
			db, cleanup := setupTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			ctx := context.Background()

			// Create first cluster
			cluster1 := &Cluster{
				Name:           name1,
				DeploymentMode: mode,
			}
			if err := repo.Create(ctx, cluster1); err != nil {
				t.Logf("Failed to create first cluster: %v", err)
				return false
			}

			// Create second cluster with different name
			cluster2 := &Cluster{
				Name:           name2,
				DeploymentMode: mode,
			}
			if err := repo.Create(ctx, cluster2); err != nil {
				t.Logf("Failed to create second cluster: %v", err)
				return false
			}

			// Attempt to update second cluster to have the same name as first
			cluster2.Name = name1
			err := repo.Update(ctx, cluster2)

			// Should return ErrClusterNameDuplicate
			return errors.Is(err, ErrClusterNameDuplicate)
		},
		genValidClusterName(),
		genValidClusterName(),
		genDeploymentMode(),
	))

	properties.TestingRun(t)
}

// TestProperty_EmptyClusterNameRejected tests that empty cluster names are rejected
func TestProperty_EmptyClusterNameRejected(t *testing.T) {
	// **Feature: stx-agent, Property 10: Cluster Name Uniqueness Validation**
	// **Validates: Requirements 7.1**

	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	ctx := context.Background()

	// Attempt to create cluster with empty name
	cluster := &Cluster{
		Name:           "",
		DeploymentMode: DeploymentModeHybrid,
	}
	err := repo.Create(ctx, cluster)

	// Should return ErrClusterNameEmpty
	if !errors.Is(err, ErrClusterNameEmpty) {
		t.Errorf("Expected ErrClusterNameEmpty, got: %v", err)
	}
}
