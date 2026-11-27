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

package plugin

import (
	"context"
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

//go:embed seed/*.json
var officialDependencySeedFS embed.FS

type officialDependencySeedFile struct {
	RequestedVersion   string                              `json:"-"`
	TemplateName       string                              `json:"template_name"`
	TemplateVersion    string                              `json:"template_version"`
	ReviewedVersions   []string                            `json:"reviewed_versions"`
	Notes              []string                            `json:"notes"`
	HiddenPlugins      []string                            `json:"hidden_plugins"`
	Catalog            []officialDependencySeedCatalog     `json:"catalog"`
	DefaultNotRequired []string                            `json:"default_not_required"`
	Profiles           []officialDependencySeedProfileSpec `json:"profiles"`
}

type officialDependencySeedCatalog struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	ArtifactID  string `json:"artifact_id"`
	GroupID     string `json:"group_id"`
	DocURL      string `json:"doc_url"`
}

type officialDependencySeedProfileSpec struct {
	PluginName               string                                  `json:"plugin_name"`
	ArtifactID               string                                  `json:"artifact_id"`
	ProfileKey               string                                  `json:"profile_key"`
	ProfileName              string                                  `json:"profile_name"`
	EngineScope              string                                  `json:"engine_scope"`
	TargetDir                string                                  `json:"target_dir"`
	AppliesTo                string                                  `json:"applies_to"`
	IncludeVersions          []string                                `json:"include_versions"`
	ExcludeVersions          []string                                `json:"exclude_versions"`
	DocSlug                  string                                  `json:"doc_slug"`
	DocSourceURL             string                                  `json:"doc_source_url"`
	Confidence               string                                  `json:"confidence"`
	IsDefault                bool                                    `json:"is_default"`
	NoAdditionalDependencies bool                                    `json:"no_additional_dependencies"`
	ResolutionMode           DependencyResolutionMode                `json:"resolution_mode"`
	BaselineVersionUsed      string                                  `json:"baseline_version_used"`
	Items                    []officialDependencySeedProfileItemSpec `json:"items"`
}

type officialDependencySeedProfileItemSpec struct {
	GroupID    string `json:"group_id"`
	ArtifactID string `json:"artifact_id"`
	Version    string `json:"version"`
	TargetDir  string `json:"target_dir"`
	Required   bool   `json:"required"`
	SourceURL  string `json:"source_url"`
	Note       string `json:"note"`
}

func (s *Service) ensureBundledSeedLoaded(ctx context.Context, version string) {
	if version == "" || s.repo == nil {
		return
	}
	s.seedLoadedMu.RLock()
	if s.seedLoadedVersions[version] {
		s.seedLoadedMu.RUnlock()
		return
	}
	s.seedLoadedMu.RUnlock()

	seed, err := loadOfficialDependencySeed(version)
	if err != nil || seed == nil {
		return
	}

	if err := s.loadOfficialDependencySeedIntoDB(ctx, seed); err != nil {
		return
	}

	s.seedLoadedMu.Lock()
	s.seedLoadedVersions[version] = true
	s.seedLoadedMu.Unlock()
}

func loadOfficialDependencySeed(version string) (*officialDependencySeedFile, error) {
	content, err := officialDependencySeedFS.ReadFile("seed/seatunnel-plugins.json")
	if err != nil {
		return nil, err
	}
	var seed officialDependencySeedFile
	if err := json.Unmarshal(content, &seed); err != nil {
		return nil, err
	}
	normalizeOfficialDependencySeed(&seed)
	seed.RequestedVersion = version
	return &seed, nil
}

func (s *Service) loadOfficialDependencySeedIntoDB(ctx context.Context, seed *officialDependencySeedFile) error {
	catalogByName := make(map[string]officialDependencySeedCatalog, len(seed.Catalog))
	keepKeys := make(map[string]struct{})
	for _, item := range seed.Catalog {
		catalogByName[item.Name] = item
	}

	for _, item := range seed.DefaultNotRequired {
		catalog, ok := catalogByName[item]
		if !ok {
			continue
		}
		profile := PluginDependencyProfile{
			SeatunnelVersion:         seed.RequestedVersion,
			PluginName:               item,
			ArtifactID:               catalog.ArtifactID,
			ProfileKey:               "default",
			ProfileName:              "Default",
			EngineScope:              "zeta",
			SourceKind:               PluginDependencyProfileSourceOfficialSeed,
			BaselineVersionUsed:      resolvedSeedBaselineVersion(seed, seed.RequestedVersion),
			ResolutionMode:           defaultSeedResolutionMode(seed, seed.RequestedVersion),
			TargetDir:                defaultPluginDependencyTargetDir(seed.RequestedVersion, catalog.ArtifactID),
			AppliesTo:                "*",
			DocSourceURL:             catalog.DocURL,
			Confidence:               "manual",
			IsDefault:                true,
			NoAdditionalDependencies: true,
			ContentHash:              hashOfficialSeedValue(seed.RequestedVersion, item, "default", "not_required"),
			Items:                    []PluginDependencyProfileItem{},
		}
		keepKeys[profile.PluginName+":"+profile.ProfileKey+":"+profile.EngineScope] = struct{}{}
		if err := s.repo.UpsertDependencyProfile(ctx, &profile); err != nil {
			return err
		}
	}

	for _, spec := range seed.Profiles {
		items := make([]PluginDependencyProfileItem, 0, len(spec.Items))
		for _, item := range spec.Items {
			targetDir := strings.TrimSpace(item.TargetDir)
			if targetDir == "" {
				targetDir = firstNonEmpty(spec.TargetDir, defaultPluginDependencyTargetDir(seed.RequestedVersion, spec.ArtifactID))
			}
			targetDir = adjustSeedTargetDirForVersion(targetDir, seed.RequestedVersion)
			items = append(items, PluginDependencyProfileItem{
				GroupID:    item.GroupID,
				ArtifactID: item.ArtifactID,
				Version:    item.Version,
				TargetDir:  targetDir,
				Required:   item.Required,
				SourceURL:  item.SourceURL,
				Note:       item.Note,
			})
		}
		if !seedProfileAppliesToRequestedVersion(spec, seed.RequestedVersion) {
			continue
		}
		profile := PluginDependencyProfile{
			SeatunnelVersion:         seed.RequestedVersion,
			PluginName:               spec.PluginName,
			ArtifactID:               spec.ArtifactID,
			ProfileKey:               spec.ProfileKey,
			ProfileName:              firstNonEmpty(spec.ProfileName, spec.ProfileKey),
			EngineScope:              spec.EngineScope,
			SourceKind:               PluginDependencyProfileSourceOfficialSeed,
			BaselineVersionUsed:      resolvedSeedProfileBaselineVersion(spec, seed, seed.RequestedVersion),
			ResolutionMode:           seedProfileResolutionMode(spec, seed, seed.RequestedVersion),
			TargetDir:                adjustSeedTargetDirForVersion(firstNonEmpty(spec.TargetDir, defaultPluginDependencyTargetDir(seed.RequestedVersion, spec.ArtifactID)), seed.RequestedVersion),
			AppliesTo:                firstNonEmpty(spec.AppliesTo, "*"),
			IncludeVersions:          strings.Join(spec.IncludeVersions, ","),
			ExcludedVersions:         strings.Join(spec.ExcludeVersions, ","),
			DocSlug:                  spec.DocSlug,
			DocSourceURL:             spec.DocSourceURL,
			Confidence:               spec.Confidence,
			IsDefault:                spec.IsDefault,
			NoAdditionalDependencies: spec.NoAdditionalDependencies,
			ContentHash:              hashOfficialSeedValue(seed.RequestedVersion, spec.PluginName, spec.ProfileKey, spec.DocSourceURL),
			Items:                    items,
		}
		keepKeys[profile.PluginName+":"+profile.ProfileKey+":"+profile.EngineScope] = struct{}{}
		if err := s.repo.UpsertDependencyProfile(ctx, &profile); err != nil {
			return err
		}
	}

	return s.repo.DeleteStaleDependencyProfiles(ctx, seed.RequestedVersion, PluginDependencyProfileSourceOfficialSeed, keepKeys)
}

func normalizeOfficialDependencySeed(seed *officialDependencySeedFile) {
	if seed == nil {
		return
	}
	if strings.TrimSpace(seed.TemplateName) == "" {
		seed.TemplateName = "seatunnel-plugins"
	}
	seed.ReviewedVersions = normalizeSeedReviewedVersions(seed.ReviewedVersions)
	if strings.TrimSpace(seed.TemplateVersion) == "" {
		if len(seed.ReviewedVersions) > 0 {
			seed.TemplateVersion = seed.ReviewedVersions[0]
		}
	}
	if strings.TrimSpace(seed.TemplateVersion) != "" {
		found := false
		for _, item := range seed.ReviewedVersions {
			if strings.TrimSpace(item) == strings.TrimSpace(seed.TemplateVersion) {
				found = true
				break
			}
		}
		if !found {
			seed.ReviewedVersions = append(seed.ReviewedVersions, strings.TrimSpace(seed.TemplateVersion))
			seed.ReviewedVersions = normalizeSeedReviewedVersions(seed.ReviewedVersions)
		}
	}
}

func normalizeSeedReviewedVersions(versions []string) []string {
	unique := make(map[string]struct{}, len(versions))
	result := make([]string, 0, len(versions))
	for _, item := range versions {
		version := strings.TrimSpace(item)
		if version == "" {
			continue
		}
		if _, exists := unique[version]; exists {
			continue
		}
		unique[version] = struct{}{}
		result = append(result, version)
	}
	sort.Slice(result, func(i, j int) bool {
		return comparePluginVersions(result[i], result[j]) < 0
	})
	return result
}

func seedHasExactVersion(seed *officialDependencySeedFile, requestedVersion string) bool {
	requestedVersion = strings.TrimSpace(requestedVersion)
	if seed == nil || requestedVersion == "" {
		return false
	}
	for _, item := range seed.ReviewedVersions {
		if strings.TrimSpace(item) == requestedVersion {
			return true
		}
	}
	return false
}

func resolvedSeedBaselineVersion(seed *officialDependencySeedFile, requestedVersion string) string {
	requestedVersion = strings.TrimSpace(requestedVersion)
	if seed == nil {
		return requestedVersion
	}
	if seedHasExactVersion(seed, requestedVersion) {
		return requestedVersion
	}
	best := ""
	for _, item := range seed.ReviewedVersions {
		version := strings.TrimSpace(item)
		if version == "" {
			continue
		}
		if comparePluginVersions(version, requestedVersion) <= 0 && (best == "" || comparePluginVersions(version, best) > 0) {
			best = version
		}
	}
	if best != "" {
		return best
	}
	return strings.TrimSpace(seed.TemplateVersion)
}

func defaultSeedResolutionMode(seed *officialDependencySeedFile, requestedVersion string) DependencyResolutionMode {
	if seedHasExactVersion(seed, requestedVersion) {
		return DependencyResolutionModeExact
	}
	return DependencyResolutionModeFallback
}

func resolvedSeedProfileBaselineVersion(spec officialDependencySeedProfileSpec, seed *officialDependencySeedFile, requestedVersion string) string {
	if value := strings.TrimSpace(spec.BaselineVersionUsed); value != "" && value != strings.TrimSpace(seed.TemplateVersion) {
		return value
	}
	return resolvedSeedBaselineVersion(seed, requestedVersion)
}

func seedProfileResolutionMode(spec officialDependencySeedProfileSpec, seed *officialDependencySeedFile, requestedVersion string) DependencyResolutionMode {
	if spec.ResolutionMode != "" && seedHasExactVersion(seed, requestedVersion) {
		return spec.ResolutionMode
	}
	return defaultSeedResolutionMode(seed, requestedVersion)
}

func seedProfileAppliesToRequestedVersion(spec officialDependencySeedProfileSpec, requestedVersion string) bool {
	if len(spec.ExcludeVersions) > 0 {
		for _, item := range spec.ExcludeVersions {
			if strings.TrimSpace(item) == strings.TrimSpace(requestedVersion) {
				return false
			}
		}
	}
	if len(spec.IncludeVersions) > 0 {
		for _, item := range spec.IncludeVersions {
			if strings.TrimSpace(item) == strings.TrimSpace(requestedVersion) {
				return true
			}
		}
		return false
	}
	appliesTo := strings.TrimSpace(spec.AppliesTo)
	return appliesTo == "" || appliesTo == "*" || appliesTo == requestedVersion
}

func adjustSeedTargetDirForVersion(targetDir, requestedVersion string) string {
	normalized := strings.TrimSpace(targetDir)
	if normalized == "" {
		return normalized
	}
	if !supportsConnectorIsolatedDependency(requestedVersion) && strings.HasPrefix(normalized, "plugins/") {
		return "lib"
	}
	return normalized
}

func hiddenPluginSetFromSeed(seed *officialDependencySeedFile) map[string]struct{} {
	result := make(map[string]struct{}, len(seed.HiddenPlugins))
	for _, item := range seed.HiddenPlugins {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		result[name] = struct{}{}
	}
	return result
}

func hashOfficialSeedValue(parts ...string) string {
	h := sha1.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
