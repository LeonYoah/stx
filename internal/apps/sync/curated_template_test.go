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
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newCuratedTestService(t *testing.T) *Service {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.AutoMigrate(&CuratedTemplate{}); err != nil {
		t.Fatalf("automigrate curated: %v", err)
	}
	return NewService(NewRepository(database))
}

func TestLoadCuratedSeedTemplates(t *testing.T) {
	items, err := loadCuratedSeedTemplates()
	if err != nil {
		t.Fatalf("load seeds: %v", err)
	}
	if len(items) < 15 {
		t.Fatalf("expected curated seeds, got %d", len(items))
	}
	foundHive := false
	foundTransform := false
	for _, item := range items {
		if item.ID == "sink-hive-kerberos" {
			foundHive = true
		}
		if item.Section == CuratedSectionTransform {
			foundTransform = true
		}
		if item.Section == "" || item.Content == "" {
			t.Fatalf("invalid seed %+v", item)
		}
	}
	if !foundHive || !foundTransform {
		t.Fatalf("missing hive/transform seeds hive=%v transform=%v", foundHive, foundTransform)
	}
}

func TestParseFourSectionsIncludesAllPresent(t *testing.T) {
	content := `
env {
  job.mode = "BATCH"
}

source {
  Jdbc {
    plugin_output = "src"
  }
}

transform {
  Copy {
    plugin_input = "src"
    plugin_output = "staged"
  }
}

sink {
  Console {
    plugin_input = ["staged"]
  }
}
`
	parsed := parseFourSections(content)
	if len(parsed.Present) != 4 {
		t.Fatalf("present=%v", parsed.Present)
	}
	if !strings.Contains(parsed.Combined, "transform") || !strings.Contains(parsed.Combined, "env") {
		t.Fatalf("combined missing sections: %s", parsed.Combined)
	}
}

func TestCuratedTemplateCRUDAndFork(t *testing.T) {
	service := newCuratedTestService(t)
	ctx := context.Background()
	userID := uint(9)

	list, err := service.ListCuratedTemplates(ctx, &ListCuratedTemplatesRequest{Section: CuratedSectionSource}, userID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if list.Total == 0 {
		t.Fatal("expected builtin sources")
	}

	created, err := service.CreateCuratedTemplate(ctx, &CreateCuratedTemplateRequest{
		Name:    "my-jdbc",
		Section: CuratedSectionSource,
		Content: `Jdbc { plugin_output = "src" password = "{{jdbc_password}}" }`,
	}, userID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Origin != CuratedOriginUser || created.ID == 0 {
		t.Fatalf("unexpected create view %+v", created)
	}

	forked, err := service.ForkCuratedTemplate(ctx, &ForkCuratedTemplateRequest{BuiltinID: "source-fake"}, userID)
	if err != nil {
		t.Fatalf("fork: %v", err)
	}
	if forked.BuiltinID != "source-fake" || forked.Origin != CuratedOriginOverride {
		t.Fatalf("unexpected fork %+v", forked)
	}

	name := "fake-copy"
	updated, err := service.UpdateCuratedTemplate(ctx, forked.ID, &UpdateCuratedTemplateRequest{Name: &name}, userID)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != name {
		t.Fatalf("name=%s", updated.Name)
	}

	if err := service.DeleteCuratedTemplate(ctx, created.ID, userID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestRewritePluginIOKeysForLegacyOnCuratedInsert(t *testing.T) {
	in := `FakeSource { plugin_output = "fake" }`
	out := rewritePluginIOKeysForLegacy(in, true)
	if !strings.Contains(out, "result_table_name") || strings.Contains(out, "plugin_output") {
		t.Fatalf("rewrite failed: %s", out)
	}
}

func TestCuratedSeedCredentialVarsAndCoverage(t *testing.T) {
	items, err := loadCuratedSeedTemplates()
	if err != nil {
		t.Fatalf("load seeds: %v", err)
	}
	requiredIDs := []string{
		"transform-copy-field",
		"sink-hive-basic",
		"sink-hive-kerberos",
		"sink-hive-s3",
		"source-localfile-json",
		"source-s3file-json",
		"source-hdfsfile-json",
		"source-ftpfile-json",
	}
	byID := map[string]curatedSeedTemplate{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range requiredIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("missing required seed %s", id)
		}
	}

	expectedVars := []string{
		"jdbc_password",
		"cdc_password",
		"ftp_password",
		"s3_access_key",
		"s3_secret_key",
	}
	joined := strings.Builder{}
	for _, item := range items {
		joined.WriteString(item.Content)
		joined.WriteByte('\n')
	}
	all := joined.String()
	for _, name := range expectedVars {
		needle := "{{" + name + "}}"
		if !strings.Contains(all, needle) {
			t.Fatalf("credential var %s not found in curated seeds", needle)
		}
	}
}

func TestCreateCuratedComboKeepsFourSections(t *testing.T) {
	service := newCuratedTestService(t)
	ctx := context.Background()
	content := `
env {
  job.mode = "BATCH"
}

source {
  Jdbc {
    plugin_output = "src"
    password = "{{jdbc_password}}"
  }
}

transform {
  Copy {
    plugin_input = "src"
    plugin_output = "staged"
  }
}

sink {
  Hive {
    plugin_input = ["staged"]
  }
}
`
	parsed, err := service.ParseCuratedCombo(ctx, &ParseCuratedComboRequest{Content: content})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Present) != 4 {
		t.Fatalf("present=%v", parsed.Present)
	}
	created, err := service.CreateCuratedTemplate(ctx, &CreateCuratedTemplateRequest{
		Name:    "combo-with-transform",
		Section: CuratedSectionCombo,
		Content: parsed.Combined,
	}, 1)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Section != CuratedSectionCombo {
		t.Fatalf("section=%s", created.Section)
	}
	for _, part := range []string{"env", "source", "transform", "sink", "{{jdbc_password}}"} {
		if !strings.Contains(created.Content, part) {
			t.Fatalf("combo content missing %q: %s", part, created.Content)
		}
	}
}
