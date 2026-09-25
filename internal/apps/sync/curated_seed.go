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
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed seed/curated-templates/*
var curatedSeedFS embed.FS

type curatedSeedManifest struct {
	Version               int                   `json:"version"`
	MinSeatunnelPluginIO  string                `json:"min_seatunnel_plugin_io"`
	Templates             []curatedSeedMeta     `json:"templates"`
}

type curatedSeedMeta struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Section     string   `json:"section"`
	Mode        string   `json:"mode"`
	Pattern     string   `json:"pattern"`
	Connectors  []string `json:"connectors"`
	File        string   `json:"file"`
	Enabled     bool     `json:"enabled"`
}

// curatedSeedTemplate is one loaded built-in curated fragment.
// curatedSeedTemplate 是一条已加载的内置精选片段。
type curatedSeedTemplate struct {
	ID          string
	Name        string
	Description string
	Section     string
	Mode        string
	Pattern     string
	Connectors  []string
	Content     string
	Enabled     bool
}

var (
	curatedSeedOnce sync.Once
	curatedSeedCache []curatedSeedTemplate
	curatedSeedErr   error
)

func loadCuratedSeedTemplates() ([]curatedSeedTemplate, error) {
	curatedSeedOnce.Do(func() {
		raw, err := curatedSeedFS.ReadFile("seed/curated-templates/manifest.json")
		if err != nil {
			curatedSeedErr = fmt.Errorf("sync: read curated manifest: %w", err)
			return
		}
		var manifest curatedSeedManifest
		if err := json.Unmarshal(raw, &manifest); err != nil {
			curatedSeedErr = fmt.Errorf("sync: parse curated manifest: %w", err)
			return
		}
		out := make([]curatedSeedTemplate, 0, len(manifest.Templates))
		for _, meta := range manifest.Templates {
			if !meta.Enabled {
				continue
			}
			file := strings.TrimSpace(meta.File)
			if file == "" {
				continue
			}
			body, err := curatedSeedFS.ReadFile("seed/curated-templates/" + file)
			if err != nil {
				curatedSeedErr = fmt.Errorf("sync: read curated seed %s: %w", file, err)
				return
			}
			out = append(out, curatedSeedTemplate{
				ID:          strings.TrimSpace(meta.ID),
				Name:        strings.TrimSpace(meta.Name),
				Description: strings.TrimSpace(meta.Description),
				Section:     normalizeCuratedSection(meta.Section),
				Mode:        normalizeCuratedMode(meta.Mode),
				Pattern:     normalizeCuratedPattern(meta.Pattern),
				Connectors:  append([]string{}, meta.Connectors...),
				Content:     string(body),
				Enabled:     true,
			})
		}
		curatedSeedCache = out
	})
	if curatedSeedErr != nil {
		return nil, curatedSeedErr
	}
	cp := make([]curatedSeedTemplate, len(curatedSeedCache))
	copy(cp, curatedSeedCache)
	return cp, nil
}

func findCuratedSeedByID(id string) (curatedSeedTemplate, bool) {
	items, err := loadCuratedSeedTemplates()
	if err != nil {
		return curatedSeedTemplate{}, false
	}
	for _, item := range items {
		if item.ID == id {
			return item, true
		}
	}
	return curatedSeedTemplate{}, false
}
