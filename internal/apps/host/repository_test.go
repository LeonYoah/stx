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

package host

import (
	"context"
	"errors"
	"fmt"
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
	tempDir, err := os.MkdirTemp("", "host_test_*")
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

	// Auto-migrate the Host model
	if err := db.AutoMigrate(&Host{}); err != nil {
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

// genValidHostName generates valid host names (alphanumeric, 1-100 chars)
func genValidHostName() gopter.Gen {
	return gen.RegexMatch("[a-zA-Z][a-zA-Z0-9_-]{0,99}").SuchThat(func(s string) bool {
		return len(s) > 0 && len(s) <= 100
	})
}

// genValidIPv4 generates valid IPv4 addresses
func genValidIPv4() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 255),
		gen.IntRange(0, 255),
		gen.IntRange(0, 255),
		gen.IntRange(1, 254),
	).Map(func(vals []interface{}) string {
		return fmt.Sprintf("%d.%d.%d.%d",
			vals[0].(int), vals[1].(int), vals[2].(int), vals[3].(int))
	})
}

// genInvalidIP generates invalid IP addresses
func genInvalidIP() gopter.Gen {
	return gen.OneGenOf(
		gen.Const(""),                // empty string
		gen.Const("invalid"),         // non-IP string
		gen.Const("256.1.1.1"),       // out of range octet
		gen.Const("1.2.3"),           // incomplete IPv4
		gen.Const("1.2.3.4.5"),       // too many octets
		gen.Const("abc.def.ghi.jkl"), // non-numeric
		gen.Const("192.168.1"),       // missing octet
		gen.AlphaString().SuchThat(func(s string) bool { // random alpha strings
			return len(s) > 0 && len(s) < 50
		}),
	)
}

// **Feature: stx-agent, Property 1: Host Name Uniqueness Validation**
// **Validates: Requirements 3.1**
// For any host creation request, if a host with the same name already exists,
// the system SHALL reject the creation and return an error indicating the name conflict.

func TestProperty_HostNameUniqueness(t *testing.T) {
	// **Feature: stx-agent, Property 1: Host Name Uniqueness Validation**
	// **Validates: Requirements 3.1**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	properties.Property("duplicate host names are rejected", prop.ForAll(
		func(name string, ip1 string, ip2 string) bool {
			// Setup fresh database for each test
			db, cleanup := setupTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			ctx := context.Background()

			// Create first host
			host1 := &Host{
				Name:      name,
				IPAddress: ip1,
			}
			err := repo.Create(ctx, host1)
			if err != nil {
				t.Logf("Failed to create first host: %v", err)
				return false
			}

			// Attempt to create second host with same name but different IP
			host2 := &Host{
				Name:      name,
				IPAddress: ip2,
			}
			err = repo.Create(ctx, host2)

			// Should return ErrHostNameDuplicate
			return errors.Is(err, ErrHostNameDuplicate)
		},
		genValidHostName(),
		genValidIPv4(),
		genValidIPv4(),
	))

	properties.TestingRun(t)
}

// TestCreateRejectsDuplicateIP tests that creating hosts with duplicate IP is rejected.
func TestCreateRejectsDuplicateIP(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	ctx := context.Background()

	host1 := &Host{
		Name:      "host-1",
		HostType:  HostTypeBareMetal,
		IPAddress: "10.10.10.10",
	}
	if err := repo.Create(ctx, host1); err != nil {
		t.Fatalf("create first host failed: %v", err)
	}

	host2 := &Host{
		Name:      "host-2",
		HostType:  HostTypeBareMetal,
		IPAddress: "10.10.10.10",
	}
	err := repo.Create(ctx, host2)
	if !errors.Is(err, ErrHostIPDuplicate) {
		t.Fatalf("expected ErrHostIPDuplicate, got: %v", err)
	}
}

// TestUpdateRejectsDuplicateIP tests that updating host IP to an existing one is rejected.
func TestUpdateRejectsDuplicateIP(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewRepository(db)
	ctx := context.Background()

	host1 := &Host{
		Name:      "host-1",
		HostType:  HostTypeBareMetal,
		IPAddress: "10.10.10.11",
	}
	if err := repo.Create(ctx, host1); err != nil {
		t.Fatalf("create first host failed: %v", err)
	}

	host2 := &Host{
		Name:      "host-2",
		HostType:  HostTypeBareMetal,
		IPAddress: "10.10.10.12",
	}
	if err := repo.Create(ctx, host2); err != nil {
		t.Fatalf("create second host failed: %v", err)
	}

	host2.IPAddress = host1.IPAddress
	err := repo.Update(ctx, host2)
	if !errors.Is(err, ErrHostIPDuplicate) {
		t.Fatalf("expected ErrHostIPDuplicate, got: %v", err)
	}
}

// **Feature: stx-agent, Property 2: IP Address Format Validation**
// **Validates: Requirements 3.1**
// For any host creation request with an IP address, the system SHALL validate
// the IP address format (IPv4 or IPv6) and reject invalid formats with a descriptive error.

func TestProperty_IPAddressFormatValidation(t *testing.T) {
	// **Feature: stx-agent, Property 2: IP Address Format Validation**
	// **Validates: Requirements 3.1**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	// Property: Valid IP addresses are accepted
	properties.Property("valid IP addresses are accepted", prop.ForAll(
		func(name string, ip string) bool {
			db, cleanup := setupTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			ctx := context.Background()

			host := &Host{
				Name:      name,
				IPAddress: ip,
			}
			err := repo.Create(ctx, host)

			// Should succeed (no IP validation error)
			return err == nil
		},
		genValidHostName(),
		genValidIPv4(),
	))

	// Property: Invalid IP addresses are rejected
	properties.Property("invalid IP addresses are rejected", prop.ForAll(
		func(name string, invalidIP string) bool {
			db, cleanup := setupTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			ctx := context.Background()

			host := &Host{
				Name:      name,
				IPAddress: invalidIP,
			}
			err := repo.Create(ctx, host)

			// Should return ErrHostIPInvalid
			return errors.Is(err, ErrHostIPInvalid)
		},
		genValidHostName(),
		genInvalidIP(),
	))

	properties.TestingRun(t)
}

// TestProperty_ValidIPv6Addresses tests that valid IPv6 addresses are accepted
func TestProperty_ValidIPv6Addresses(t *testing.T) {
	// **Feature: stx-agent, Property 2: IP Address Format Validation**
	// **Validates: Requirements 3.1**

	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100
	parameters.Rng.Seed(42)

	properties := gopter.NewProperties(parameters)

	// Generate valid IPv6 addresses
	genValidIPv6 := gen.OneConstOf(
		"::1",
		"2001:db8::1",
		"fe80::1",
		"::ffff:192.168.1.1",
		"2001:0db8:85a3:0000:0000:8a2e:0370:7334",
	)

	properties.Property("valid IPv6 addresses are accepted", prop.ForAll(
		func(name string, ip string) bool {
			db, cleanup := setupTestDB(t)
			defer cleanup()

			repo := NewRepository(db)
			ctx := context.Background()

			host := &Host{
				Name:      name,
				IPAddress: ip,
			}
			err := repo.Create(ctx, host)

			return err == nil
		},
		genValidHostName(),
		genValidIPv6,
	))

	properties.TestingRun(t)
}

// TestValidateIPAddress tests the ValidateIPAddress function directly
func TestValidateIPAddress(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// Valid IPv4
		{"valid IPv4 - localhost", "127.0.0.1", true},
		{"valid IPv4 - private", "192.168.1.1", true},
		{"valid IPv4 - public", "8.8.8.8", true},
		{"valid IPv4 - zeros", "0.0.0.0", true},
		{"valid IPv4 - max", "255.255.255.255", true},

		// Valid IPv6
		{"valid IPv6 - loopback", "::1", true},
		{"valid IPv6 - full", "2001:0db8:85a3:0000:0000:8a2e:0370:7334", true},
		{"valid IPv6 - compressed", "2001:db8::1", true},
		{"valid IPv6 - link-local", "fe80::1", true},

		// Invalid
		{"invalid - empty", "", false},
		{"invalid - text", "invalid", false},
		{"invalid - out of range", "256.1.1.1", false},
		{"invalid - incomplete", "192.168.1", false},
		{"invalid - too many octets", "1.2.3.4.5", false},
		{"invalid - alpha", "abc.def.ghi.jkl", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateIPAddress(tt.ip)
			if result != tt.expected {
				t.Errorf("ValidateIPAddress(%q) = %v, want %v", tt.ip, result, tt.expected)
			}
		})
	}
}
