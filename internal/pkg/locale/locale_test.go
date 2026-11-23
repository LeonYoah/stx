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

package locale

import (
	"context"
	"testing"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		raw     string
		want    Locale
		wantOK  bool
	}{
		{"zh", ZH, true},
		{"zh-CN", ZH, true},
		{"ZH-hans", ZH, true},
		{"zh_cn", ZH, true},
		{" zh ", ZH, true},
		{"en", EN, true},
		{"en-US", EN, true},
		{"EN_gb", EN, true},
		{"ja", "", false},
		{"", "", false},
		{"  ", "", false},
		{"fr-FR", "", false},
	}
	for _, c := range cases {
		got, ok := Normalize(c.raw)
		if got != c.want || ok != c.wantOK {
			t.Errorf("Normalize(%q) = (%q, %v), want (%q, %v)", c.raw, got, ok, c.want, c.wantOK)
		}
	}
}

func TestParseAcceptLanguage(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		want    Locale
		wantOK  bool
	}{
		{"empty", "", "", false},
		{"wildcard", "*", "", false},
		{"single zh", "zh-CN", ZH, true},
		{"single en", "en-US", EN, true},
		{"weighted zh first", "zh-CN,zh;q=0.9,en;q=0.8", ZH, true},
		{"weighted en first", "en-US,en;q=0.9,zh;q=0.7", EN, true},
		{"en wins by weight", "zh;q=0.5,en;q=0.9", EN, true},
		{"order wins on equal weight", "en,zh", EN, true},
		{"order wins on equal weight zh", "zh,en", ZH, true},
		{"skips unrecognised", "fr-FR,en;q=0.8", EN, true},
		{"all unrecognised", "fr-FR,ja;q=0.8", "", false},
		{"malformed q keeps default weight", "zh;q=abc,en", ZH, true},
		{"spaces tolerated", " zh-CN ; q=0.9 , en ; q=0.1 ", ZH, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseAcceptLanguage(c.header)
			if got != c.want || ok != c.wantOK {
				t.Errorf("ParseAcceptLanguage(%q) = (%q, %v), want (%q, %v)", c.header, got, ok, c.want, c.wantOK)
			}
		})
	}
}

func TestContextRoundTrip(t *testing.T) {
	if got := FromContext(context.Background()); got != Default {
		t.Fatalf("empty context = %q, want %q", got, Default)
	}
	if got := FromContext(nil); got != Default {
		t.Fatalf("nil context = %q, want %q", got, Default)
	}
	ctx := WithLocale(context.Background(), EN)
	if got := FromContext(ctx); got != EN {
		t.Fatalf("FromContext = %q, want %q", got, EN)
	}
}

func TestPick(t *testing.T) {
	zhCtx := WithLocale(context.Background(), ZH)
	enCtx := WithLocale(context.Background(), EN)
	if got := Pick(zhCtx, "中文", "English"); got != "中文" {
		t.Fatalf("zh Pick = %q", got)
	}
	if got := Pick(enCtx, "中文", "English"); got != "English" {
		t.Fatalf("en Pick = %q", got)
	}
	if got := Pick(context.Background(), "中文", "English"); got != "中文" {
		t.Fatalf("default Pick = %q, want zh", got)
	}
	if got := Pick(enCtx, "", "English"); got != "English" {
		t.Fatalf("en Pick fallback = %q", got)
	}
	if got := Pick(enCtx, "中文", ""); got != "中文" {
		t.Fatalf("en Pick zh fallback = %q", got)
	}
}
