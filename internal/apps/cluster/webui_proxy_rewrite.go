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
	"fmt"
	"regexp"
	"strings"
)

// vueHistoryBaseRe 匹配 createWebHistory("/") / createWebHashHistory("/") 的压缩调用。
// vueHistoryBaseRe matches minified createWebHistory("/") / createWebHashHistory("/") calls.
var vueHistoryBaseRe = regexp.MustCompile(`history:([A-Za-z_$][\w$]*)\("/"\)`)

// vueHistoryBaseSingleRe 同上，单引号形式。
// vueHistoryBaseSingleRe same as above with single quotes.
var vueHistoryBaseSingleRe = regexp.MustCompile(`history:([A-Za-z_$][\w$]*)\('/'\)`)

// rewriteSeaTunnelWebUIContent 把 SeaTunnel Web UI 在子路径代理下的绝对资源 / API 路径改写到 prefix。
// SeaTunnel 3.0 UI 使用 axios baseURL:"" + "/overview"，且 Vue Router / Vite 预加载会把路径钉在站点根上。
// rewriteSeaTunnelWebUIContent rewrites absolute asset/API paths for SeaTunnel Web UI under a subpath proxy.
// SeaTunnel 3.0 UI uses axios baseURL:"" with "/overview", and Vue Router / Vite preload pin paths at site root.
func rewriteSeaTunnelWebUIContent(content, contentType, prefix string) string {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	if prefix == "" || content == "" {
		return content
	}
	contentType = strings.ToLower(contentType)

	// 先改写正文中的根路径，再注入 <base> / patch，避免 base href 被二次加前缀。
	// Rewrite root-absolute paths first, then inject <base>/patch so the base href is not double-prefixed.
	content = rewriteRootAbsoluteAttr(content, "href", prefix)
	content = rewriteRootAbsoluteAttr(content, "src", prefix)
	content = rewriteRootAbsoluteAttr(content, "action", prefix)
	content = strings.ReplaceAll(content, "url(/", "url("+prefix+"/")
	content = strings.ReplaceAll(content, "fetch('/api/", "fetch('"+prefix+"/api/")
	content = strings.ReplaceAll(content, "fetch(\"/api/", "fetch(\""+prefix+"/api/")
	content = strings.ReplaceAll(content, "baseURL:\"/api\"", "baseURL:\""+prefix+"/api\"")
	content = strings.ReplaceAll(content, "baseURL: '/api'", "baseURL: '"+prefix+"/api'")
	content = strings.ReplaceAll(content, "baseURL:'/api'", "baseURL:'"+prefix+"/api'")
	content = strings.ReplaceAll(content, "axios.defaults.baseURL=\"/api\"", "axios.defaults.baseURL=\""+prefix+"/api\"")
	// SeaTunnel 3.0：axios.create({timeout:6000,baseURL:""})，空 baseURL 需指到代理前缀。
	// SeaTunnel 3.0: axios.create({timeout:6000,baseURL:""}) needs the empty baseURL pointed at the proxy prefix.
	content = strings.ReplaceAll(content, "baseURL:\"\"", "baseURL:\""+prefix+"\"")
	content = strings.ReplaceAll(content, "baseURL:''", "baseURL:'"+prefix+"'")

	// Vite 预加载：aE=function(e){return"/"+e} 会请求 /assets/...，必须挂到代理前缀。
	// Vite preload: aE=function(e){return"/"+e} requests /assets/... and must use the proxy prefix.
	// RE2 无反向引用，按常见压缩参数名逐个替换。 / RE2 has no backrefs; replace common minified param names.
	for _, arg := range []string{"e", "t", "n", "r"} {
		from := `=function(` + arg + `){return"/"+` + arg + `}`
		to := `=function(` + arg + `){return"` + prefix + `/"+` + arg + `}`
		content = strings.ReplaceAll(content, from, to)
	}

	// Vue Router history base 默认 "/" 会把地址栏改到 /jobs，脱离 /webui 前缀并打到 Next.js。
	// Vue Router history base "/" moves the address bar to /jobs, leaving /webui and hitting Next.js.
	content = vueHistoryBaseRe.ReplaceAllString(content, `history:$1("`+prefix+`/")`)
	content = vueHistoryBaseSingleRe.ReplaceAllString(content, `history:$1('`+prefix+`/')`)

	if strings.Contains(contentType, "text/html") {
		headInject := ""
		if !strings.Contains(content, "data-stx-webui-prefix-patch") {
			headInject += seaTunnelWebUIPrefixPatchScript(prefix)
		}
		if !strings.Contains(strings.ToLower(content), "<base ") {
			headInject += `<base href="` + prefix + `/">`
		}
		if headInject != "" {
			content = strings.Replace(content, "<head>", "<head>"+headInject, 1)
		}
	}

	return content
}

// rewriteRootAbsoluteAttr 将 attr="/..." 改写为 attr="prefix/..."，已带前缀的不再重复改写。
// rewriteRootAbsoluteAttr rewrites attr="/..." to attr="prefix/...", skipping values already under prefix.
func rewriteRootAbsoluteAttr(content, attr, prefix string) string {
	needle := attr + "=\"/"
	already := attr + "=\"" + prefix + "/"
	if !strings.Contains(content, needle) {
		return content
	}
	var b strings.Builder
	b.Grow(len(content) + len(prefix)*4)
	remaining := content
	for {
		idx := strings.Index(remaining, needle)
		if idx < 0 {
			b.WriteString(remaining)
			break
		}
		b.WriteString(remaining[:idx])
		if strings.HasPrefix(remaining[idx:], already) {
			b.WriteString(already)
			remaining = remaining[idx+len(already):]
			continue
		}
		b.WriteString(attr + "=\"" + prefix + "/")
		remaining = remaining[idx+len(needle):]
	}
	return b.String()
}

// seaTunnelWebUIPrefixPatchScript 注入 fetch/XHR 补丁，把根路径或同站绝对 URL 挂到代理前缀下。
// seaTunnelWebUIPrefixPatchScript injects a fetch/XHR patch for root paths and same-origin absolute URLs.
func seaTunnelWebUIPrefixPatchScript(prefix string) string {
	prefix = strings.TrimRight(strings.TrimSpace(prefix), "/")
	// 使用 JSON 字符串字面量，避免前缀中的引号破坏脚本。
	// Use a JSON string literal so quotes inside the prefix cannot break the script.
	prefixJS := fmt.Sprintf("%q", prefix)
	return `<script data-stx-webui-prefix-patch="1">(function(){var p=` + prefixJS + `;if(!p)return;function fix(u){if(typeof u!=="string"||!u)return u;try{if(/^https?:\/\//i.test(u)||u.indexOf("//")===0){var a=document.createElement("a");a.href=u;if(a.origin===location.origin&&a.pathname!==p&&a.pathname.indexOf(p+"/")!==0){return p+a.pathname+a.search+a.hash;}return u;}}catch(e){return u;}if(u===p||u.indexOf(p+"/")===0)return u;if(u.charAt(0)==="/")return p+u;return u;}try{var of=window.fetch;if(typeof of==="function"){window.fetch=function(input,init){if(typeof input==="string"){input=fix(input);}else if(input&&typeof Request!=="undefined"&&input instanceof Request){try{input=new Request(fix(input.url),input);}catch(e){}}return of.call(this,input,init);};}var xo=XMLHttpRequest.prototype.open;XMLHttpRequest.prototype.open=function(method,url){if(typeof url==="string"){arguments[1]=fix(url);}return xo.apply(this,arguments);};}catch(e){}})();</script>`
}
