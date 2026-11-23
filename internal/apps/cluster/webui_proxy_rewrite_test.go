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
	"strings"
	"testing"
)

func TestRewriteSeaTunnelWebUIContent_rewritesV3RootAPIs(t *testing.T) {
	t.Parallel()

	prefix := "/api/v1/clusters/11/webui"
	html := `<head></head><body><img src="/assets/logo.png"/><script>const eu={timeout:6e3,baseURL:""};tn.get("/overview");fetch("/api/v1/x")</script></body>`
	got := rewriteSeaTunnelWebUIContent(html, "text/html; charset=utf-8", prefix)

	if !strings.Contains(got, `<base href="`+prefix+`/">`) {
		t.Fatalf("expected base href rewrite, got: %s", got)
	}
	if strings.Count(got, prefix+prefix) > 0 {
		t.Fatalf("base/asset paths must not be double-prefixed: %s", got)
	}
	if !strings.Contains(got, `data-stx-webui-prefix-patch="1"`) {
		t.Fatalf("expected fetch/XHR prefix patch script, got: %s", got)
	}
	if !strings.Contains(got, `baseURL:"`+prefix+`"`) {
		t.Fatalf("expected empty axios baseURL rewritten to proxy prefix, got: %s", got)
	}
	if strings.Contains(got, `baseURL:""`) {
		t.Fatalf("empty axios baseURL should not remain: %s", got)
	}
	if !strings.Contains(got, `src="`+prefix+`/assets/logo.png"`) {
		t.Fatalf("expected asset src rewrite, got: %s", got)
	}
}

func TestRewriteSeaTunnelWebUIContent_rewritesLegacyAPIBase(t *testing.T) {
	t.Parallel()

	prefix := "/api/v1/clusters/11/webui"
	js := `axios.defaults.baseURL="/api";fetch("/api/jobs");href="/assets/a.js"`
	got := rewriteSeaTunnelWebUIContent(js, "application/javascript", prefix)
	if !strings.Contains(got, `axios.defaults.baseURL="`+prefix+`/api"`) {
		t.Fatalf("expected legacy /api baseURL rewrite, got: %s", got)
	}
	if !strings.Contains(got, `fetch('`+prefix+`/api/jobs')`) && !strings.Contains(got, `fetch("`+prefix+`/api/jobs")`) {
		t.Fatalf("expected fetch /api rewrite, got: %s", got)
	}
	if !strings.Contains(got, `href="`+prefix+`/assets/a.js"`) {
		t.Fatalf("expected asset href rewrite, got: %s", got)
	}
}

func TestRewriteSeaTunnelWebUIContent_rewritesVitePreloadAndVueHistory(t *testing.T) {
	t.Parallel()

	prefix := "/api/v1/clusters/11/webui"
	// 对齐 SeaTunnel 3.0 压缩产物：Vite aE 预加载 + Vue Router createWebHistory("/")。
	// Mirrors SeaTunnel 3.0 minified output: Vite aE preload + Vue Router createWebHistory("/").
	js := `aE=function(e){return"/"+e},Mu={};const Jh=oE({history:Aw("/"),routes:lE});const eu={timeout:6e3,baseURL:""};`
	got := rewriteSeaTunnelWebUIContent(js, "application/javascript", prefix)

	if !strings.Contains(got, `aE=function(e){return"`+prefix+`/"+e}`) {
		t.Fatalf("expected Vite assetsURL helper rewrite, got: %s", got)
	}
	if strings.Contains(got, `aE=function(e){return"/"+e}`) {
		t.Fatalf("root Vite assetsURL helper should not remain: %s", got)
	}
	if !strings.Contains(got, `history:Aw("`+prefix+`/")`) {
		t.Fatalf("expected Vue history base rewrite, got: %s", got)
	}
	if strings.Contains(got, `history:Aw("/")`) {
		t.Fatalf("root Vue history base should not remain: %s", got)
	}
	if !strings.Contains(got, `baseURL:"`+prefix+`"`) {
		t.Fatalf("expected empty axios baseURL rewrite, got: %s", got)
	}
}

func TestRewriteSeaTunnelWebUIContent_rewritesViteImportedAssetLiteral(t *testing.T) {
	t.Parallel()

	prefix := "/api/v1/clusters/11/webui"
	// SeaTunnel 3.0.0 产物：import logo → Go="/assets/logo-DnrCYlPZ.png"（见 seatunnel-engine-server ui jar）。
	// SeaTunnel 3.0.0 build: import logo → Go="/assets/logo-DnrCYlPZ.png" (from seatunnel-engine-server ui jar).
	js := `const Go="/assets/logo-DnrCYlPZ.png";z("img",{src:Go,class:"h-12"});const other='/assets/chunk.js';`
	got := rewriteSeaTunnelWebUIContent(js, "application/javascript", prefix)

	wantLogo := `"` + prefix + `/assets/logo-DnrCYlPZ.png"`
	if !strings.Contains(got, wantLogo) {
		t.Fatalf("expected imported logo path under proxy prefix, want substring %s, got: %s", wantLogo, got)
	}
	if strings.Contains(got, `"/assets/logo-DnrCYlPZ.png"`) {
		t.Fatalf("root-absolute logo literal must not remain: %s", got)
	}
	wantChunk := `'` + prefix + `/assets/chunk.js'`
	if !strings.Contains(got, wantChunk) {
		t.Fatalf("expected single-quoted asset literal rewrite, got: %s", got)
	}
	if strings.Count(got, prefix+prefix) > 0 {
		t.Fatalf("asset paths must not be double-prefixed: %s", got)
	}
}

func TestRewriteSeaTunnelWebUIContent_doesNotBroadenNonAssetRoots(t *testing.T) {
	t.Parallel()

	prefix := "/api/v1/clusters/11/webui"
	// 反例（AC3）：只改写 /assets（与 favicon），不要把任意站点根路径并进 /webui 前缀。
	// Counterexample (AC3): rewrite only /assets (and favicon); do not fold arbitrary roots into /webui.
	js := strings.Join([]string{
		`const logo="/assets/logo-DnrCYlPZ.png";`,
		`const login="/api/v1/auth/login";`,
		`const clusters="/api/v1/clusters/11/nodes";`,
		`const page="/clusters/11?tab=runtime";`,
		`const cdn="https://cdn.example/assets/logo.png";`,
	}, "")
	got := rewriteSeaTunnelWebUIContent(js, "application/javascript", prefix)

	if !strings.Contains(got, `"`+prefix+`/assets/logo-DnrCYlPZ.png"`) {
		t.Fatalf("expected /assets logo rewrite, got: %s", got)
	}
	for _, keep := range []string{
		`"/api/v1/auth/login"`,
		`"/api/v1/clusters/11/nodes"`,
		`"/clusters/11?tab=runtime"`,
		`"https://cdn.example/assets/logo.png"`,
	} {
		if !strings.Contains(got, keep) {
			t.Fatalf("non-asset root must stay unchanged (%s), got: %s", keep, got)
		}
	}
	if strings.Contains(got, `"`+prefix+`/api/v1/auth/login"`) {
		t.Fatalf("must not prefix STX API paths with webui prefix, got: %s", got)
	}
	if strings.Contains(got, `"`+prefix+`/clusters/11`) {
		t.Fatalf("must not prefix STX page routes with webui prefix, got: %s", got)
	}
}
