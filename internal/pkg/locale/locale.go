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
	"sort"
	"strconv"
	"strings"
)

type Locale string

const (
	ZH Locale = "zh"
	EN Locale = "en"

	Default = ZH
)

type contextKey struct{}

func Normalize(raw string) (Locale, bool) {
	tag := strings.ToLower(strings.TrimSpace(raw))
	tag = strings.ReplaceAll(tag, "_", "-")
	switch {
	case strings.HasPrefix(tag, "zh"):
		return ZH, true
	case strings.HasPrefix(tag, "en"):
		return EN, true
	default:
		return "", false
	}
}

func ParseAcceptLanguage(header string) (Locale, bool) {
	header = strings.TrimSpace(header)
	if header == "" || header == "*" {
		return "", false
	}
	type candidate struct {
		loc Locale
		q   float64
		idx int
	}
	candidates := make([]candidate, 0, 4)
	for i, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		tag := part
		q := 1.0
		if semi := strings.Index(part, ";"); semi >= 0 {
			tag = strings.TrimSpace(part[:semi])
			for _, param := range strings.Split(part[semi+1:], ";") {
				param = strings.TrimSpace(param)
				if v, ok := strings.CutPrefix(param, "q="); ok {
					if parsed, err := strconv.ParseFloat(v, 64); err == nil {
						q = parsed
					}
				}
			}
		}
		loc, ok := Normalize(tag)
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{loc: loc, q: q, idx: i})
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].q == candidates[j].q {
			return candidates[i].idx < candidates[j].idx
		}
		return candidates[i].q > candidates[j].q
	})
	return candidates[0].loc, true
}

func WithLocale(ctx context.Context, loc Locale) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, loc)
}

func FromContext(ctx context.Context) Locale {
	if ctx == nil {
		return Default
	}
	if loc, ok := ctx.Value(contextKey{}).(Locale); ok && loc != "" {
		return loc
	}
	return Default
}

func Pick(ctx context.Context, zh, en string) string {
	if FromContext(ctx) == EN {
		return firstNonEmpty(en, zh)
	}
	return firstNonEmpty(zh, en)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
