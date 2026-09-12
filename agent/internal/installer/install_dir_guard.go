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
	"os"
	"path/filepath"
	"strings"
)

const managedInstallMarkerFileName = ".stx-managed"

// RemoveManagedInstallDir removes a SeaTunnel installation directory after validating
// that the path is a STX-managed installation directory.
func RemoveManagedInstallDir(installDir string) (string, error) {
	clean, err := validateManagedInstallDir(installDir)
	if err != nil {
		return "", err
	}

	if err := os.RemoveAll(clean); err != nil {
		return "", fmt.Errorf("failed to remove installation directory: %w", err)
	}
	return clean, nil
}

// WriteManagedInstallMarker writes a marker file so future cleanup can distinguish
// SeaTunnel-managed directories from arbitrary host paths.
func WriteManagedInstallMarker(installDir string) error {
	clean, err := normalizeInstallDirPath(installDir)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(clean, managedInstallMarkerFileName), []byte("managed_by=stx-agent\n"), 0644)
}

func validateManagedInstallDir(installDir string) (string, error) {
	clean, err := normalizeInstallDirPath(installDir)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return clean, nil
		}
		return "", fmt.Errorf("failed to access install_dir %s: %w", clean, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("install_dir must be a directory: %s", clean)
	}
	if hasManagedInstallMarker(clean) {
		return clean, nil
	}
	return "", fmt.Errorf("install_dir is not a managed SeaTunnel installation; for safety, please remove it manually: %s", clean)
}

func normalizeInstallDirPath(installDir string) (string, error) {
	trimmed := strings.TrimSpace(installDir)
	if trimmed == "" {
		return "", fmt.Errorf("install_dir is required")
	}

	clean := filepath.Clean(trimmed)
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("install_dir must be an absolute path: %s", clean)
	}
	if filepath.Dir(clean) == clean {
		return "", fmt.Errorf("install_dir is not allowed: %s", clean)
	}
	return clean, nil
}

func hasManagedInstallMarker(installDir string) bool {
	info, err := os.Stat(filepath.Join(installDir, managedInstallMarkerFileName))
	return err == nil && !info.IsDir()
}
