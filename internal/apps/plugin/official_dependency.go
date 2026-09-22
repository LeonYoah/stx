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
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/seatunnel"
)

const officialDocsRawBaseURL = "https://raw.githubusercontent.com/apache/seatunnel-website/main/versioned_docs/version-%s/connector-v2/%s.md"

type officialDependencyDocSpec struct {
	PluginName  string
	ArtifactID  string
	ProfileKey  string
	EngineScope string
	DocSlug     string
	TargetDir   string
	Confidence  string
	IsDefault   bool
	ParserKind  string
}

type mavenMetadata struct {
	Versioning struct {
		Latest   string   `xml:"latest"`
		Release  string   `xml:"release"`
		Versions []string `xml:"versions>version"`
	} `xml:"versioning"`
}

var officialDependencyDocSpecs = map[string][]officialDependencyDocSpec{
	"hive": {
		{
			PluginName:  "hive",
			ArtifactID:  "connector-hive",
			ProfileKey:  "default",
			EngineScope: "zeta",
			DocSlug:     "sink/Hive",
			TargetDir:   "plugins/connector-hive",
			Confidence:  "medium",
			IsDefault:   true,
			ParserKind:  "hive_sink",
		},
	},
	"cdc-oracle": {
		{
			PluginName:  "cdc-oracle",
			ArtifactID:  "connector-cdc-oracle",
			ProfileKey:  "default",
			EngineScope: "zeta",
			DocSlug:     "source/Oracle-CDC",
			TargetDir:   "plugins/connector-cdc-oracle",
			Confidence:  "high",
			IsDefault:   true,
			ParserKind:  "oracle_like",
		},
	},
	"jdbc": {
		{
			PluginName:  "jdbc",
			ArtifactID:  "connector-jdbc",
			ProfileKey:  "oracle",
			EngineScope: "zeta",
			DocSlug:     "sink/Oracle",
			TargetDir:   "plugins/connector-jdbc",
			Confidence:  "medium",
			IsDefault:   false,
			ParserKind:  "oracle_like",
		},
		{
			PluginName:  "jdbc",
			ArtifactID:  "connector-jdbc",
			ProfileKey:  "hive",
			EngineScope: "zeta",
			DocSlug:     "source/HiveJdbc",
			TargetDir:   "plugins/connector-jdbc",
			Confidence:  "medium",
			IsDefault:   false,
			ParserKind:  "hivejdbc",
		},
	},
}

// loadPluginsFromCatalog loads persisted plugins for a version.
// loadPluginsFromCatalog 从数据库加载已持久化的插件目录。
func (s *Service) loadPluginsFromCatalog(ctx context.Context, version string) ([]Plugin, MirrorSource, *time.Time) {
	if s.repo == nil {
		return nil, MirrorSourceApache, nil
	}
	entries, err := s.repo.ListCatalogEntriesByVersion(ctx, version)
	if err != nil || len(entries) == 0 {
		return nil, MirrorSourceApache, nil
	}
	hasSeedEntries := false
	for _, entry := range entries {
		if entry.Source == PluginCatalogSourceSeed {
			hasSeedEntries = true
			break
		}
	}
	if hasSeedEntries {
		return nil, MirrorSourceApache, nil
	}
	plugins := make([]Plugin, 0, len(entries))
	var sourceMirror MirrorSource = MirrorSourceApache
	var refreshedAt *time.Time
	for _, entry := range entries {
		if strings.TrimSpace(entry.SourceMirror) != "" {
			sourceMirror = MirrorSource(entry.SourceMirror)
		}
		if entry.RefreshedAt != nil {
			refreshedAt = entry.RefreshedAt
		}
		plugins = append(plugins, Plugin{
			Name:        entry.PluginName,
			DisplayName: entry.DisplayName,
			Category:    entry.Category,
			Version:     entry.SeatunnelVersion,
			Description: entry.Description,
			GroupID:     entry.GroupID,
			ArtifactID:  entry.ArtifactID,
			DocURL:      entry.DocURL,
		})
	}
	return s.filterHiddenPluginsForVersion(version, plugins), sourceMirror, refreshedAt
}

// persistPluginCatalog stores plugin catalog entries in DB.
// persistPluginCatalog 将插件目录持久化到数据库。
func (s *Service) persistPluginCatalog(ctx context.Context, version string, plugins []Plugin, source PluginCatalogSource, sourceMirror MirrorSource, refreshedAt time.Time) error {
	if s.repo == nil || len(plugins) == 0 {
		return nil
	}
	entries := make([]PluginCatalogEntry, 0, len(plugins))
	for _, plugin := range plugins {
		entries = append(entries, PluginCatalogEntry{
			SeatunnelVersion: version,
			PluginName:       plugin.Name,
			DisplayName:      plugin.DisplayName,
			ArtifactID:       plugin.ArtifactID,
			GroupID:          plugin.GroupID,
			Category:         plugin.Category,
			Description:      plugin.Description,
			DocURL:           plugin.DocURL,
			Source:           source,
			SourceMirror:     string(sourceMirror),
			RefreshedAt:      &refreshedAt,
		})
	}
	if source == PluginCatalogSourceRemote {
		return s.repo.ReplaceCatalogEntriesByVersion(ctx, version, entries)
	}
	return s.repo.UpsertCatalogEntries(ctx, entries)
}

func (s *Service) filterHiddenPluginsForVersion(version string, plugins []Plugin) []Plugin {
	seed, err := loadOfficialDependencySeed(version)
	if err != nil || seed == nil || len(seed.HiddenPlugins) == 0 {
		return plugins
	}
	hiddenSet := hiddenPluginSetFromSeed(seed)
	if len(hiddenSet) == 0 {
		return plugins
	}
	filtered := make([]Plugin, 0, len(plugins))
	for _, item := range plugins {
		if _, hidden := hiddenSet[item.Name]; hidden {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}

// GetOfficialDependencies returns stored official dependency profiles for one plugin.
// GetOfficialDependencies 返回指定插件的已存储官方依赖画像。
func (s *Service) GetOfficialDependencies(ctx context.Context, pluginName, version, profileKey string) (*OfficialDependenciesResponse, error) {
	if version == "" {
		version = seatunnel.DefaultVersion()
	}
	s.ensureBundledSeedLoaded(ctx, version)
	profiles, effective, status, mode, baseline, disabled := s.resolveStoredOfficialDependencies(ctx, pluginName, version, normalizeProfileKeys([]string{profileKey}))
	return &OfficialDependenciesResponse{
		PluginName:               pluginName,
		SeatunnelVersion:         version,
		DependencyStatus:         status,
		DependencyCount:          len(effective),
		BaselineVersionUsed:      baseline,
		DependencyResolutionMode: mode,
		Profiles:                 profiles,
		EffectiveDependencies:    effective,
		DisabledDependencies:     disabled,
	}, nil
}

// AnalyzeOfficialDependencies fetches official docs and stores analyzed dependency profiles.
// AnalyzeOfficialDependencies 抓取官方文档并保存分析后的依赖画像。
func (s *Service) AnalyzeOfficialDependencies(ctx context.Context, pluginName, version, profileKey string, forceRefresh bool) (*OfficialDependenciesResponse, error) {
	if version == "" {
		version = seatunnel.DefaultVersion()
	}
	if s.repo == nil {
		return nil, fmt.Errorf("repository not configured / 仓储未配置")
	}
	s.ensureBundledSeedLoaded(ctx, version)

	specs := filterOfficialDependencySpecs(pluginName, profileKey)
	if len(specs) == 0 {
		// 没有在线分析器时仍返回已经保存的官方依赖，避免分析接口与查询接口结果不一致。
		// When no online analyzer exists, return stored official dependencies so analyze and lookup stay consistent.
		return s.GetOfficialDependencies(ctx, pluginName, version, profileKey)
	}

	if !forceRefresh {
		profiles, effective, status, mode, baseline, disabled := s.resolveStoredOfficialDependencies(ctx, pluginName, version, normalizeProfileKeys([]string{profileKey}))
		if len(profiles) > 0 {
			return &OfficialDependenciesResponse{
				PluginName:               pluginName,
				SeatunnelVersion:         version,
				DependencyStatus:         status,
				DependencyCount:          len(effective),
				BaselineVersionUsed:      baseline,
				DependencyResolutionMode: mode,
				Profiles:                 profiles,
				EffectiveDependencies:    effective,
				DisabledDependencies:     disabled,
			}, nil
		}
	}

	for _, spec := range specs {
		profile, err := s.buildProfileFromOfficialDoc(ctx, version, spec)
		if err != nil {
			return nil, err
		}
		if err := s.repo.UpsertDependencyProfile(ctx, profile); err != nil {
			return nil, err
		}
	}

	return s.GetOfficialDependencies(ctx, pluginName, version, profileKey)
}

// GetEffectiveDependencies merges official stored dependencies and user-configured dependencies.
// GetEffectiveDependencies 合并官方依赖与用户补充依赖。
func (s *Service) GetEffectiveDependencies(ctx context.Context, pluginName, version string, profileKeys []string) ([]PluginDependency, error) {
	s.ensureBundledSeedLoaded(ctx, version)
	_, officialDeps, _, _, _, _ := s.resolveStoredOfficialDependencies(ctx, pluginName, version, profileKeys)
	userDeps, err := s.getUserConfiguredDependencies(ctx, pluginName, version)
	if err != nil {
		return nil, err
	}
	merged := mergeDependencies(officialDeps, userDeps)
	return merged, nil
}

// enrichPluginsWithDependencyState adds lightweight dependency status for plugin marketplace.
// enrichPluginsWithDependencyState 为插件市场补充轻量依赖状态。
func (s *Service) enrichPluginsWithDependencyState(ctx context.Context, version string, plugins []Plugin) []Plugin {
	if len(plugins) == 0 {
		return plugins
	}
	result := make([]Plugin, len(plugins))
	copy(result, plugins)
	for i := range result {
		profiles, effective, status, mode, baseline, _ := s.resolveStoredOfficialDependencies(ctx, result[i].Name, version, nil)
		result[i].DependencyStatus = status
		result[i].DependencyCount = len(effective)
		result[i].DependencyBaselineVersion = baseline
		result[i].DependencyResolutionMode = mode
		if len(profiles) == 1 {
			result[i].Dependencies = effective
		}
	}
	return result
}

func (s *Service) getUserConfiguredDependencies(ctx context.Context, pluginName, version string) ([]PluginDependency, error) {
	if s.repo == nil {
		return []PluginDependency{}, nil
	}
	configs, err := s.repo.ListDependencies(ctx, pluginName, version)
	if err != nil {
		return nil, err
	}
	deps := make([]PluginDependency, 0, len(configs))
	for _, cfg := range configs {
		deps = append(deps, PluginDependency{
			GroupID:          cfg.GroupID,
			ArtifactID:       cfg.ArtifactID,
			Version:          cfg.Version,
			TargetDir:        cfg.TargetDir,
			SourceType:       cfg.SourceType,
			OriginalFileName: cfg.OriginalFileName,
			StoredPath:       cfg.StoredPath,
		})
	}
	return deps, nil
}

func (s *Service) resolveStoredOfficialDependencies(ctx context.Context, pluginName, version string, profileKeys []string) ([]PluginDependencyProfile, []PluginDependency, PluginDependencyStatus, DependencyResolutionMode, string, []PluginDependencyDisable) {
	if s.repo == nil {
		return []PluginDependencyProfile{}, []PluginDependency{}, PluginDependencyStatusUnknown, DependencyResolutionModeNone, "", nil
	}
	profiles, err := s.repo.ListDependencyProfilesByPlugin(ctx, pluginName)
	if err != nil || len(profiles) == 0 {
		return []PluginDependencyProfile{}, []PluginDependency{}, PluginDependencyStatusUnknown, DependencyResolutionModeNone, "", nil
	}

	selectedProfiles := selectBestProfiles(profiles, version, normalizeProfileKeys(profileKeys))
	if len(selectedProfiles) == 0 {
		return []PluginDependencyProfile{}, []PluginDependency{}, PluginDependencyStatusUnknown, DependencyResolutionModeNone, "", nil
	}
	disabledItems, err := s.repo.ListDependencyDisables(ctx, pluginName, version)
	if err != nil {
		disabledItems = nil
	}
	disabledByKey := make(map[string]PluginDependencyDisable, len(disabledItems))
	for _, item := range disabledItems {
		disabledByKey[dependencyKey(item.GroupID, item.ArtifactID, item.Version, item.TargetDir)] = item
	}
	for i := range selectedProfiles {
		for j := range selectedProfiles[i].Items {
			item := &selectedProfiles[i].Items[j]
			resolvedVersion := resolveEffectiveDependencyVersion(version, *item)
			key := dependencyKey(item.GroupID, item.ArtifactID, resolvedVersion, item.TargetDir)
			if disabled, ok := disabledByKey[key]; ok {
				item.Disabled = true
				item.DisableID = &disabled.ID
			}
		}
	}

	var effective []PluginDependency
	if len(normalizeProfileKeys(profileKeys)) > 1 {
		merged := make([]PluginDependency, 0)
		for _, profile := range selectedProfiles {
			profileDeps := make([]PluginDependency, 0, len(profile.Items))
			for _, item := range profile.Items {
				if item.Disabled {
					continue
				}
				profileDeps = append(profileDeps, PluginDependency{
					GroupID:    item.GroupID,
					ArtifactID: item.ArtifactID,
					Version:    resolveEffectiveDependencyVersion(version, item),
					TargetDir:  item.TargetDir,
					SourceType: PluginDependencySourceOfficial,
				})
			}
			merged = mergeDependencies(merged, profileDeps)
		}
		effective = merged
		return selectedProfiles, effective, summarizeProfileStatus(selectedProfiles), summarizeResolutionMode(selectedProfiles), summarizeBaselineVersion(selectedProfiles), disabledItems
	}

	primary, ok := choosePrimaryProfile(selectedProfiles, normalizeProfileKeys(profileKeys))
	if !ok {
		return selectedProfiles, []PluginDependency{}, PluginDependencyStatusUnknown, DependencyResolutionModeNone, "", disabledItems
	}

	effective = make([]PluginDependency, 0, len(primary.Items))
	for _, item := range primary.Items {
		if item.Disabled {
			continue
		}
		effective = append(effective, PluginDependency{
			GroupID:    item.GroupID,
			ArtifactID: item.ArtifactID,
			Version:    resolveEffectiveDependencyVersion(version, item),
			TargetDir:  item.TargetDir,
			SourceType: PluginDependencySourceOfficial,
		})
	}
	status := mapProfileToStatus(primary)
	baseline := primary.BaselineVersionUsed
	if baseline == "" {
		baseline = primary.SeatunnelVersion
	}
	return selectedProfiles, effective, status, primary.ResolutionMode, baseline, disabledItems
}

func resolveEffectiveDependencyVersion(requestedVersion string, item PluginDependencyProfileItem) string {
	version := strings.TrimSpace(item.Version)
	if requestedVersion == "" {
		return version
	}
	if item.TargetDir == "connectors" && item.GroupID == "org.apache.seatunnel" {
		return requestedVersion
	}
	return version
}

func selectBestProfiles(profiles []PluginDependencyProfile, requestedVersion string, profileKeys []string) []PluginDependencyProfile {
	requested := make(map[string]struct{}, len(profileKeys))
	for _, key := range profileKeys {
		requested[canonicalProfileKey(key)] = struct{}{}
	}
	grouped := make(map[string][]PluginDependencyProfile)
	for _, profile := range profiles {
		profileKey := canonicalProfileKey(profile.ProfileKey)
		if len(requested) > 0 {
			if _, ok := requested[profileKey]; !ok {
				continue
			}
		}
		if !profileAppliesToVersion(profile, requestedVersion) {
			continue
		}
		grouped[profileKey] = append(grouped[profileKey], profile)
	}
	result := make([]PluginDependencyProfile, 0, len(grouped))
	for _, candidates := range grouped {
		if best, ok := chooseBestProfile(candidates, requestedVersion); ok {
			result = append(result, best)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsDefault != result[j].IsDefault {
			return result[i].IsDefault
		}
		return result[i].ProfileKey < result[j].ProfileKey
	})
	return result
}

func choosePrimaryProfile(profiles []PluginDependencyProfile, profileKeys []string) (PluginDependencyProfile, bool) {
	if len(profiles) == 0 {
		return PluginDependencyProfile{}, false
	}
	if len(profileKeys) == 1 {
		return profiles[0], true
	}
	if len(profiles) == 1 {
		return profiles[0], true
	}
	for _, profile := range profiles {
		if profile.IsDefault {
			return profile, true
		}
	}
	return PluginDependencyProfile{}, false
}

func chooseBestProfile(candidates []PluginDependencyProfile, requestedVersion string) (PluginDependencyProfile, bool) {
	if len(candidates) == 0 {
		return PluginDependencyProfile{}, false
	}
	exact := make([]PluginDependencyProfile, 0)
	fallbackMinor := make([]PluginDependencyProfile, 0)
	fallbackMajor := make([]PluginDependencyProfile, 0)
	for _, profile := range candidates {
		switch {
		case profile.SeatunnelVersion == requestedVersion:
			exact = append(exact, profile)
		case profile.AppliesTo == "*" && profile.BaselineVersionUsed != "":
			fallbackMajor = append(fallbackMajor, profile)
		case isSameMajorMinorVersion(profile.SeatunnelVersion, requestedVersion) && comparePluginVersions(profile.SeatunnelVersion, requestedVersion) < 0:
			fallbackMinor = append(fallbackMinor, profile)
		case isSameMajorVersion(profile.SeatunnelVersion, requestedVersion) && comparePluginVersions(profile.SeatunnelVersion, requestedVersion) < 0:
			fallbackMajor = append(fallbackMajor, profile)
		}
	}
	for _, group := range [][]PluginDependencyProfile{exact, fallbackMinor, fallbackMajor} {
		if len(group) == 0 {
			continue
		}
		sort.Slice(group, func(i, j int) bool {
			cmp := comparePluginVersions(group[i].SeatunnelVersion, group[j].SeatunnelVersion)
			if cmp != 0 {
				return cmp > 0
			}
			return preferProfileSource(group[i].SourceKind) < preferProfileSource(group[j].SourceKind)
		})
		best := group[0]
		if best.SeatunnelVersion != requestedVersion {
			best.BaselineVersionUsed = best.SeatunnelVersion
			if best.SourceKind == PluginDependencyProfileSourceRuntimeAnalyzed {
				best.ResolutionMode = DependencyResolutionModeRuntime
			} else {
				best.ResolutionMode = DependencyResolutionModeFallback
			}
		}
		return best, true
	}
	return PluginDependencyProfile{}, false
}

func normalizeProfileKeys(profileKeys []string) []string {
	if len(profileKeys) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(profileKeys))
	result := make([]string, 0, len(profileKeys))
	for _, key := range profileKeys {
		normalized := canonicalProfileKey(key)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result
}

func canonicalProfileKey(key string) string {
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "hivejdbc":
		return "hive"
	default:
		return normalized
	}
}

func profileAppliesToVersion(profile PluginDependencyProfile, version string) bool {
	if strings.TrimSpace(version) == "" {
		return true
	}
	if isVersionListed(profile.ExcludedVersions, version) {
		return false
	}
	if strings.TrimSpace(profile.AppliesTo) == "" || strings.TrimSpace(profile.AppliesTo) == "*" {
		if strings.TrimSpace(profile.IncludeVersions) == "" {
			return true
		}
		return isVersionListed(profile.IncludeVersions, version)
	}
	return isVersionListed(profile.AppliesTo, version)
}

func isVersionListed(raw, version string) bool {
	for _, item := range strings.Split(raw, ",") {
		if strings.TrimSpace(item) == strings.TrimSpace(version) {
			return true
		}
	}
	return false
}

func summarizeProfileStatus(profiles []PluginDependencyProfile) PluginDependencyStatus {
	allNoDeps := true
	for _, profile := range profiles {
		if !profile.NoAdditionalDependencies {
			allNoDeps = false
			break
		}
	}
	if allNoDeps {
		return PluginDependencyStatusNotRequired
	}
	for _, profile := range profiles {
		status := mapProfileToStatus(profile)
		if status == PluginDependencyStatusRuntimeAnalyzed {
			return status
		}
	}
	for _, profile := range profiles {
		status := mapProfileToStatus(profile)
		if status == PluginDependencyStatusReadyFallback {
			return status
		}
	}
	return PluginDependencyStatusReadyExact
}

func summarizeResolutionMode(profiles []PluginDependencyProfile) DependencyResolutionMode {
	for _, profile := range profiles {
		if profile.ResolutionMode == DependencyResolutionModeRuntime {
			return DependencyResolutionModeRuntime
		}
	}
	for _, profile := range profiles {
		if profile.ResolutionMode == DependencyResolutionModeFallback {
			return DependencyResolutionModeFallback
		}
	}
	if len(profiles) > 0 {
		return DependencyResolutionModeExact
	}
	return DependencyResolutionModeNone
}

func summarizeBaselineVersion(profiles []PluginDependencyProfile) string {
	for _, profile := range profiles {
		if strings.TrimSpace(profile.BaselineVersionUsed) != "" {
			return profile.BaselineVersionUsed
		}
		if strings.TrimSpace(profile.SeatunnelVersion) != "" {
			return profile.SeatunnelVersion
		}
	}
	return ""
}

func preferProfileSource(source PluginDependencyProfileSource) int {
	if source == PluginDependencyProfileSourceOfficialSeed {
		return 0
	}
	return 1
}

func mapProfileToStatus(profile PluginDependencyProfile) PluginDependencyStatus {
	switch {
	case profile.NoAdditionalDependencies:
		return PluginDependencyStatusNotRequired
	case profile.SourceKind == PluginDependencyProfileSourceRuntimeAnalyzed:
		return PluginDependencyStatusRuntimeAnalyzed
	case profile.ResolutionMode == DependencyResolutionModeFallback:
		return PluginDependencyStatusReadyFallback
	case profile.ResolutionMode == DependencyResolutionModeRuntime:
		return PluginDependencyStatusRuntimeAnalyzed
	default:
		return PluginDependencyStatusReadyExact
	}
}

func filterOfficialDependencySpecs(pluginName, profileKey string) []officialDependencyDocSpec {
	specs := officialDependencyDocSpecs[strings.ToLower(pluginName)]
	if profileKey == "" {
		return specs
	}
	profileKey = canonicalProfileKey(profileKey)
	filtered := make([]officialDependencyDocSpec, 0, len(specs))
	for _, spec := range specs {
		if canonicalProfileKey(spec.ProfileKey) == profileKey {
			filtered = append(filtered, spec)
		}
	}
	return filtered
}

func (s *Service) buildProfileFromOfficialDoc(ctx context.Context, version string, spec officialDependencyDocSpec) (*PluginDependencyProfile, error) {
	fetcher := s.officialDocFetcher
	if fetcher == nil {
		fetcher = s.fetchOfficialDocMarkdown
	}
	markdown, err := fetcher(ctx, version, spec.DocSlug)
	if err != nil {
		return nil, err
	}
	items, noDeps, err := s.parseOfficialDependencies(ctx, version, markdown, spec)
	if err != nil {
		return nil, err
	}
	contentHash := sha1.Sum([]byte(markdown))
	targetDir := firstNonEmpty(spec.TargetDir, defaultPluginDependencyTargetDir(version, spec.ArtifactID))
	profile := &PluginDependencyProfile{
		SeatunnelVersion:         version,
		PluginName:               spec.PluginName,
		ArtifactID:               spec.ArtifactID,
		ProfileKey:               spec.ProfileKey,
		EngineScope:              spec.EngineScope,
		SourceKind:               PluginDependencyProfileSourceRuntimeAnalyzed,
		BaselineVersionUsed:      version,
		ResolutionMode:           DependencyResolutionModeRuntime,
		TargetDir:                targetDir,
		DocSlug:                  spec.DocSlug,
		DocSourceURL:             fmt.Sprintf(officialDocsRawBaseURL, version, spec.DocSlug),
		Confidence:               spec.Confidence,
		IsDefault:                spec.IsDefault,
		NoAdditionalDependencies: noDeps,
		ContentHash:              hex.EncodeToString(contentHash[:]),
		Items:                    items,
	}
	return profile, nil
}

func (s *Service) fetchOfficialDocMarkdown(ctx context.Context, version, docSlug string) (string, error) {
	url := fmt.Sprintf(officialDocsRawBaseURL, version, docSlug)
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch official docs: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (s *Service) parseOfficialDependencies(ctx context.Context, version, markdown string, spec officialDependencyDocSpec) ([]PluginDependencyProfileItem, bool, error) {
	switch spec.ParserKind {
	case "oracle_like":
		return s.parseOracleLikeDependencies(ctx, markdown, spec)
	case "hivejdbc":
		return s.parseHiveJdbcDependencies(markdown, spec)
	case "hive_sink":
		return s.parseHiveSinkDependencies(version, markdown, spec)
	default:
		return nil, false, fmt.Errorf("unsupported parser kind: %s", spec.ParserKind)
	}
}

func (s *Service) parseOracleLikeDependencies(ctx context.Context, markdown string, spec officialDependencyDocSpec) ([]PluginDependencyProfileItem, bool, error) {
	coords := extractMavenCoordinates(markdown)
	items := make([]PluginDependencyProfileItem, 0, 2)
	seen := make(map[string]struct{})
	for _, coord := range coords {
		if coord.groupID == "" || coord.artifactID == "" {
			continue
		}
		version := coord.version
		if version == "" {
			resolver := s.mavenVersionLookup
			if resolver == nil {
				resolver = s.resolveLatestMavenVersion
			}
			resolvedVersion, err := resolver(ctx, coord.groupID, coord.artifactID)
			if err != nil {
				return nil, false, err
			}
			version = resolvedVersion
		}
		key := coord.groupID + ":" + coord.artifactID + ":" + version + ":" + spec.TargetDir
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, PluginDependencyProfileItem{
			GroupID:    coord.groupID,
			ArtifactID: coord.artifactID,
			Version:    version,
			TargetDir:  spec.TargetDir,
			Required:   true,
			SourceURL:  coord.sourceURL,
		})
	}
	resolver := s.mavenVersionLookup
	if resolver == nil {
		resolver = s.resolveLatestMavenVersion
	}
	orai18nVersion, err := resolver(ctx, "com.oracle.database.nls", "orai18n")
	if err != nil {
		return nil, false, err
	}
	items = append(items, PluginDependencyProfileItem{
		GroupID:    "com.oracle.database.nls",
		ArtifactID: "orai18n",
		Version:    orai18nVersion,
		TargetDir:  spec.TargetDir,
		Required:   true,
		Note:       "Added from official docs note for i18n support / 根据官方文档 i18n 说明补充",
	})
	return items, false, nil
}

func (s *Service) parseHiveJdbcDependencies(markdown string, spec officialDependencyDocSpec) ([]PluginDependencyProfileItem, bool, error) {
	coords := extractMavenCoordinates(markdown)
	for _, coord := range coords {
		if coord.groupID == "org.apache.hive" && coord.artifactID == "hive-jdbc" {
			return []PluginDependencyProfileItem{{
				GroupID:    coord.groupID,
				ArtifactID: coord.artifactID,
				Version:    firstNonEmpty(extractSupportedVersion(markdown), coord.version, "3.1.3"),
				TargetDir:  spec.TargetDir,
				Required:   true,
				SourceURL:  coord.sourceURL,
			}}, false, nil
		}
	}
	return []PluginDependencyProfileItem{{
		GroupID:    "org.apache.hive",
		ArtifactID: "hive-jdbc",
		Version:    firstNonEmpty(extractSupportedVersion(markdown), "3.1.3"),
		TargetDir:  spec.TargetDir,
		Required:   true,
		Note:       "Recovered from HiveJdbc official docs / 从 HiveJdbc 官方文档恢复",
	}}, false, nil
}

func (s *Service) parseHiveSinkDependencies(version, markdown string, spec officialDependencyDocSpec) ([]PluginDependencyProfileItem, bool, error) {
	if !strings.Contains(markdown, "seatunnel-hadoop3-3.1.4-uber.jar") || !strings.Contains(markdown, "hive-exec-3.1.3.jar") {
		return nil, false, fmt.Errorf("unexpected Hive docs format / Hive 文档格式不符合预期")
	}
	return []PluginDependencyProfileItem{
		{
			GroupID:    "org.apache.seatunnel",
			ArtifactID: "seatunnel-hadoop3-3.1.4-uber",
			Version:    version,
			TargetDir:  spec.TargetDir,
			Required:   true,
			Note:       "Recovered from official Hive sink docs / 从 Hive 官方文档恢复",
		},
		{
			GroupID:    "org.apache.hive",
			ArtifactID: "hive-exec",
			Version:    "3.1.3",
			TargetDir:  spec.TargetDir,
			Required:   true,
		},
		{
			GroupID:    "org.apache.thrift",
			ArtifactID: "libfb303",
			Version:    "0.9.3",
			TargetDir:  spec.TargetDir,
			Required:   true,
		},
	}, false, nil
}

type extractedCoordinate struct {
	groupID    string
	artifactID string
	version    string
	sourceURL  string
}

func extractMavenCoordinates(markdown string) []extractedCoordinate {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`https?://mvnrepository\.com/artifact/([^/\s]+)/([^/\s)\]]+)`),
		regexp.MustCompile(`<groupId>\s*([^<]+)\s*</groupId>.*?<artifactId>\s*([^<]+)\s*</artifactId>(?:.*?<version>\s*([^<]+)\s*</version>)?`),
	}
	seen := make(map[string]struct{})
	result := make([]extractedCoordinate, 0)
	for _, pattern := range patterns {
		matches := pattern.FindAllStringSubmatch(markdown, -1)
		for _, match := range matches {
			if len(match) < 3 {
				continue
			}
			coord := extractedCoordinate{groupID: strings.TrimSpace(match[1]), artifactID: strings.TrimSpace(match[2])}
			if len(match) > 3 {
				coord.version = strings.TrimSpace(match[3])
			}
			coord.sourceURL = match[0]
			key := coord.groupID + ":" + coord.artifactID + ":" + coord.version
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, coord)
		}
	}
	return result
}

func extractSupportedVersion(markdown string) string {
	re := regexp.MustCompile(`supports\s+([0-9]+\.[0-9]+\.[0-9]+)`)
	if match := re.FindStringSubmatch(strings.ToLower(markdown)); len(match) >= 2 {
		return match[1]
	}
	return ""
}

func (s *Service) resolveLatestMavenVersion(ctx context.Context, groupID, artifactID string) (string, error) {
	groupPath := strings.ReplaceAll(groupID, ".", "/")
	url := fmt.Sprintf("%s/%s/%s/maven-metadata.xml", MirrorURLs[MirrorSourceApache], groupPath, artifactID)
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	applyMavenRequestHeaders(req)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch maven metadata: %s", resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var metadata mavenMetadata
	if err := xml.Unmarshal(body, &metadata); err != nil {
		return "", err
	}
	if metadata.Versioning.Release != "" {
		return metadata.Versioning.Release, nil
	}
	if metadata.Versioning.Latest != "" {
		return metadata.Versioning.Latest, nil
	}
	if len(metadata.Versioning.Versions) == 0 {
		return "", fmt.Errorf("no maven versions found for %s:%s", groupID, artifactID)
	}
	sort.Slice(metadata.Versioning.Versions, func(i, j int) bool {
		return comparePluginVersions(metadata.Versioning.Versions[i], metadata.Versioning.Versions[j]) > 0
	})
	return metadata.Versioning.Versions[0], nil
}

func mergeDependencies(groups ...[]PluginDependency) []PluginDependency {
	merged := make([]PluginDependency, 0)
	seen := make(map[string]struct{})
	for _, deps := range groups {
		for _, dep := range deps {
			key := dependencyKey(dep.GroupID, dep.ArtifactID, dep.Version, dep.TargetDir)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, dep)
		}
	}
	return merged
}

func isSameMajorMinorVersion(v1, v2 string) bool {
	p1 := strings.Split(v1, ".")
	p2 := strings.Split(v2, ".")
	if len(p1) < 2 || len(p2) < 2 {
		return false
	}
	return p1[0] == p2[0] && p1[1] == p2[1]
}

func isSameMajorVersion(v1, v2 string) bool {
	p1 := strings.Split(v1, ".")
	p2 := strings.Split(v2, ".")
	if len(p1) == 0 || len(p2) == 0 {
		return false
	}
	return p1[0] == p2[0]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dependencyKey(groupID, artifactID, version, targetDir string) string {
	return strings.TrimSpace(groupID) + ":" + strings.TrimSpace(artifactID) + ":" + strings.TrimSpace(version) + ":" + strings.TrimSpace(targetDir)
}
