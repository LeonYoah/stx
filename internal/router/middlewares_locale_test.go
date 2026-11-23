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

package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/LeonYoah/stx/internal/pkg/locale"
	"github.com/gin-gonic/gin"
)

func runLocaleMiddleware(t *testing.T, target, acceptLanguage string) locale.Locale {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(localeMiddleware())
	var got locale.Locale
	router.GET("/probe", func(c *gin.Context) {
		got = locale.FromContext(c.Request.Context())
		c.Status(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if acceptLanguage != "" {
		req.Header.Set("Accept-Language", acceptLanguage)
	}
	router.ServeHTTP(httptest.NewRecorder(), req)
	return got
}

func TestLocaleMiddlewarePriority(t *testing.T) {
	cases := []struct {
		name           string
		target         string
		acceptLanguage string
		want           locale.Locale
	}{
		{"lang wins over header", "/probe?lang=en", "zh-CN", locale.EN},
		{"language wins over header", "/probe?language=en", "zh-CN", locale.EN},
		{"locale wins over header", "/probe?locale=en", "zh-CN", locale.EN},
		{"lang wins over language", "/probe?lang=en&language=zh", "", locale.EN},
		{"language wins over locale", "/probe?language=en&locale=zh", "", locale.EN},
		{"header fallback", "/probe", "en-US,en;q=0.9", locale.EN},
		{"header weighted zh", "/probe", "zh-CN,zh;q=0.9,en;q=0.8", locale.ZH},
		{"invalid lang falls to header", "/probe?lang=ja", "en", locale.EN},
		{"default when nothing", "/probe", "", locale.ZH},
		{"unrecognised header default", "/probe", "fr-FR", locale.ZH},
		{"zh-cn region tag", "/probe?lang=zh-cn", "", locale.ZH},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := runLocaleMiddleware(t, c.target, c.acceptLanguage); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
