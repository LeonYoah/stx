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

package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// A：通用 Webhook 改成飞书且密钥留空时，不能沿用 Webhook 的旧密钥去签飞书请求（复现抓包 #010 / 19021）。
// A: switching a generic webhook to Feishu with the secret left empty must not reuse the webhook's old secret to sign
// Feishu requests (reproduces capture #010 / 19021).
func TestUpdateNotificationChannel_TypeChangeDropsOldSecret(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)
	enabled := true
	capture := newIMCapture(t, `{"code":0,"msg":"success"}`)
	created, err := service.CreateNotificationChannel(ctx, &UpsertNotificationChannelRequest{
		Name: "ch1", Type: NotificationChannelTypeWebhook, Enabled: &enabled,
		Endpoint: "https://hooks.example.com/stx?token=OLD", Secret: "old-webhook-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"", notificationSecretMask} {
		if _, err := service.UpdateNotificationChannel(ctx, created.ID, &UpsertNotificationChannelRequest{
			Name: "ch1", Type: NotificationChannelTypeFeishu, Enabled: &enabled,
			Endpoint: capture.server.URL + "/open-apis/bot/v2/hook/NEW-FEISHU-HOOK", Secret: secret,
		}); err != nil {
			t.Fatal(err)
		}
		stored, _ := repo.GetNotificationChannelByID(ctx, created.ID)
		if stored.Type != NotificationChannelTypeFeishu || stored.Secret != "" {
			t.Fatalf("type change with secret %q must clear the old secret: %+v", secret, stored)
		}
		// 下次再改回 webhook 也不会凭空冒出旧密钥。
		// Changing back later must not resurrect the old secret either.
		if _, err := sendTestNotification(ctx, stored, nil); err != nil {
			t.Fatal(err)
		}
		if body := string(capture.lastBody(t)); strings.Contains(body, `"sign"`) || strings.Contains(body, `"timestamp"`) {
			t.Fatalf("an unsigned feishu channel must not sign the body: %s", body)
		}
		// 恢复成带旧密钥的 webhook，进入下一轮。
		// Restore the webhook with the old secret for the next round.
		stored.Type, stored.Secret, stored.Endpoint = NotificationChannelTypeWebhook, "old-webhook-secret", "https://hooks.example.com/stx?token=OLD"
		if err := repo.SaveNotificationChannel(ctx, stored); err != nil {
			t.Fatal(err)
		}
	}

	// 类型没变时，空 / 占位符仍表示保持不变。
	// Without a type change, empty / placeholder still keeps the stored secret.
	if _, err := service.UpdateNotificationChannel(ctx, created.ID, &UpsertNotificationChannelRequest{
		Name: "ch1", Type: NotificationChannelTypeWebhook, Enabled: &enabled, Endpoint: "https://hooks.example.com/stx?token=******", Secret: notificationSecretMask,
	}); err != nil {
		t.Fatal(err)
	}
	if stored, _ := repo.GetNotificationChannelByID(ctx, created.ID); stored.Secret != "old-webhook-secret" || !strings.HasSuffix(stored.Endpoint, "token=OLD") {
		t.Fatalf("same-type masked update must keep the stored values: %+v", stored)
	}

	// 草稿测试也一样：改了类型就不借用已保存的密钥。
	// Draft tests behave the same: a changed type does not borrow the saved secret.
	draft, err := restoreMaskedNotificationChannelFields(&UpsertNotificationChannelRequest{
		Name: "ch1", Type: NotificationChannelTypeDingTalk, Endpoint: "https://oapi.dingtalk.com/robot/send?access_token=x", Secret: notificationSecretMask,
	}, &NotificationChannel{Type: NotificationChannelTypeWebhook, Secret: "old-webhook-secret"})
	if err != nil || draft.Secret != "" {
		t.Fatalf("draft with a changed type must not reuse the secret: %+v %v", draft, err)
	}
}

// A：clear_secret 显式清空已保存的密钥；同时给了新密钥时以新密钥为准。
// A: clear_secret explicitly clears the stored secret; a new secret in the same request wins.
func TestUpdateNotificationChannel_ClearSecret(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)
	enabled := true
	created, err := service.CreateNotificationChannel(ctx, &UpsertNotificationChannelRequest{
		Name: "ding", Type: NotificationChannelTypeDingTalk, Enabled: &enabled,
		Endpoint: "https://oapi.dingtalk.com/robot/send?access_token=T", Secret: "SECold",
	})
	if err != nil {
		t.Fatal(err)
	}
	update := func(secret string, clear bool) *NotificationChannel {
		t.Helper()
		if _, err := service.UpdateNotificationChannel(ctx, created.ID, &UpsertNotificationChannelRequest{
			Name: "ding", Type: NotificationChannelTypeDingTalk, Enabled: &enabled,
			Endpoint: "https://oapi.dingtalk.com/robot/send?access_token=******", Secret: secret, ClearSecret: clear,
		}); err != nil {
			t.Fatal(err)
		}
		stored, _ := repo.GetNotificationChannelByID(ctx, created.ID)
		return stored
	}
	if stored := update(notificationSecretMask, true); stored.Secret != "" || !strings.HasSuffix(stored.Endpoint, "access_token=T") {
		t.Fatalf("clear_secret with the placeholder must clear only the secret: %+v", stored)
	}
	if stored := update("SECnew", true); stored.Secret != "SECnew" {
		t.Fatalf("a new secret must win over clear_secret: %+v", stored)
	}
	if stored := update("", true); stored.Secret != "" {
		t.Fatalf("clear_secret with an empty secret must clear it: %+v", stored)
	}
}

func newNotificationChannelTestRouter(service *Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(service)
	router.POST("/notification-channels", handler.CreateNotificationChannel)
	router.PUT("/notification-channels/:id", handler.UpdateNotificationChannel)
	return router
}

func doJSON(t *testing.T, router http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// C2：更新时 endpoint 仍含 ****** 且被改过，是请求错误（400），不是服务端错误（500）；clear_secret 经 HTTP 接口生效。
// C2: an edited endpoint that still holds ****** is a client error (400), not a server error (500); clear_secret
// works through the HTTP API.
func TestNotificationChannelHandler_MaskedEndpointIs400AndClearSecretWired(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)
	router := newNotificationChannelTestRouter(service)

	rec := doJSON(t, router, http.MethodPost, "/notification-channels", map[string]interface{}{
		"name": "ding", "type": "dingtalk", "endpoint": "https://oapi.dingtalk.com/robot/send?access_token=T", "secret": "SECold",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Data NotificationChannelDTO `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/notification-channels/%d", created.Data.ID)

	rec = doJSON(t, router, http.MethodPut, path, map[string]interface{}{
		"name": "ding", "type": "dingtalk", "endpoint": "https://other.example.com/robot/send?access_token=******", "secret": "******",
	})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "masked value") {
		t.Fatalf("masked endpoint update must be 400, got %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, router, http.MethodPost, "/notification-channels", map[string]interface{}{
		"name": "ding2", "type": "dingtalk", "endpoint": "https://oapi.dingtalk.com/robot/send?access_token=******",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("masked endpoint create must be 400, got %d %s", rec.Code, rec.Body.String())
	}
	if rec = doJSON(t, router, http.MethodPut, "/notification-channels/9999", map[string]interface{}{
		"name": "x", "type": "dingtalk", "endpoint": "https://oapi.dingtalk.com/robot/send?access_token=T",
	}); rec.Code != http.StatusNotFound {
		t.Fatalf("missing channel must be 404, got %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, router, http.MethodPut, path, map[string]interface{}{
		"name": "ding", "type": "dingtalk", "endpoint": created.Data.Endpoint, "secret": "******", "clear_secret": true,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("clear secret: %d %s", rec.Code, rec.Body.String())
	}
	stored, _ := repo.GetNotificationChannelByID(ctx, created.Data.ID)
	if stored.Secret != "" || !strings.HasSuffix(stored.Endpoint, "access_token=T") {
		t.Fatalf("clear_secret via API must clear only the secret: %+v", stored)
	}
	if err := (&invalidNotificationChannelError{message: "x"}); !errors.Is(err, ErrInvalidNotificationChannel) {
		t.Fatalf("validation errors must match ErrInvalidNotificationChannel")
	}
}

// C1：错误信息里的 URL 脱敏要保留 #fragment（钉钉帮助链接），fragment 里的 token 仍要脱敏。
// C1: URL redaction in error messages keeps the #fragment (DingTalk help links) but still masks tokens in it.
func TestRedactEndpointURL_KeepsFragment(t *testing.T) {
	cases := map[string]string{
		"https://ding-doc.dingtalk.com/doc#/serverapi2/qf2nxq]":                                         "https://ding-doc.dingtalk.com/doc#/serverapi2/qf2nxq]",
		"https://open.dingtalk.com/document/robots/customize-robot-security-settings#title-7fs-kgs-36x": "https://open.dingtalk.com/document/robots/customize-robot-security-settings#title-7fs-kgs-36x",
		"https://example.com/cb?access_token=abc#access_token=abc&state=1":                              "https://example.com/cb?access_token=******#access_token=******&state=******",
		"https://example.com/p#abcdefghijklmnop1234567890":                                              "https://example.com/p#******",
	}
	for input, want := range cases {
		if got := redactEndpointURL(input); got != want {
			t.Fatalf("redact %q: got %q want %q", input, got, want)
		}
	}

	capture := newIMCapture(t, `{"errcode":310000,"errmsg":"sign not match, more: [https://ding-doc.dingtalk.com/doc#/serverapi2/qf2nxq]"}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeDingTalk, Endpoint: capture.server.URL + "/robot/send?access_token=FAKE-DING-TOKEN", Secret: "SECx"}
	_, err := sendNotification(context.Background(), channel, textPayload(channel.Type, "hi"))
	if err == nil || !strings.Contains(err.Error(), "more: [https://ding-doc.dingtalk.com/doc#/serverapi2/qf2nxq]") {
		t.Fatalf("dingtalk help link must survive redaction: %v", err)
	}
	if strings.Contains(err.Error(), "FAKE-DING-TOKEN") || strings.Contains(err.Error(), "SECx") {
		t.Fatalf("error leaks credentials: %v", err)
	}
}

// buildLongSyncAlertText 模拟抓包 #034：同步告警 + 超长中文错误摘要（含堆栈续行），策略和链接在最后。
// buildLongSyncAlertText mimics capture #034: a sync alert with a very long Chinese error summary (stack continuation
// lines included), policy and link at the end.
func buildLongSyncAlertText(summaryRunes int) (string, []string) {
	title := "[STX][告警][严重] 任务 mysql-cdc-schema-demo · 作业运行失败"
	status := "状态：告警中"
	policy := "策略：订单库 CDC 作业告警 (12)"
	link := "链接：http://stx-console.example:17880/sync?job=1088&task=16"
	lines := []string{
		title,
		status,
		"规则：作业运行失败",
		"集群：prod-seatunnel-3x (1)",
		"任务：mysql-cdc-schema-demo (#16)",
		"错误摘要：" + strings.Repeat("连", summaryRunes) + "\n\tat com.mysql.cj.jdbc.exceptions.SQLError.createSQLException(SQLError.java:129)\n\tat <init>",
		"详情：" + strings.Repeat("详细<信息>\n", summaryRunes/10),
		policy,
		link,
	}
	return strings.Join(lines, "\n"), []string{title, status, "规则：作业运行失败", policy, link}
}

// D：裁剪先截短错误摘要 / 详情，标题、状态、策略和控制台链接保持完整（企业微信 2048 字节、飞书 / 钉钉请求体上限）。
// D: fitting shortens the error summary / details first and keeps title, status, policy and the console link intact
// (WeCom 2048 bytes, Feishu / DingTalk body limits).
func TestFitNotificationPayload_KeepsTitleStatusPolicyAndLink(t *testing.T) {
	cases := []struct {
		channelType NotificationChannelType
		runes       int
		limit       int
	}{
		{NotificationChannelTypeWeCom, 500, weComTextContentMaxBytes},
		{NotificationChannelTypeWeCom, 5000, weComTextContentMaxBytes},
		{NotificationChannelTypeFeishu, 12000, feishuRequestBodyMaxBytes - imSignatureReserveBytes},
		{NotificationChannelTypeDingTalk, 12000, dingTalkRequestBodyMaxBytes},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s-%d", tc.channelType, tc.runes), func(t *testing.T) {
			text, kept := buildLongSyncAlertText(tc.runes)
			fitted := fitNotificationPayload(tc.channelType, textPayload(tc.channelType, text))
			got, _, ok := imTextPayload(tc.channelType, fitted)
			if !ok {
				t.Fatalf("fitted payload is not a text message")
			}
			size := len(got)
			if tc.channelType != NotificationChannelTypeWeCom {
				raw, _ := json.Marshal(fitted)
				size = len(raw)
			}
			if size > tc.limit || !utf8.ValidString(got) {
				t.Fatalf("fitted size %d exceeds %d (or invalid UTF-8)", size, tc.limit)
			}
			for _, line := range kept {
				if !strings.Contains(got, line) {
					t.Fatalf("fitted text lost %q:\n%s", line, got)
				}
			}
			if !strings.HasSuffix(got, kept[len(kept)-1]) {
				t.Fatalf("console link must stay the last line:\n%s", got[len(got)-200:])
			}
			if !strings.Contains(got, "错误摘要：连") || !strings.Contains(got, notificationFieldTruncatedSuffix) {
				t.Fatalf("the long fields should be shortened, not dropped")
			}
		})
	}

	// 端到端：企业微信渠道实际发出的正文也保留链接。
	// End to end: the text actually sent to WeCom keeps the link too.
	capture := newIMCapture(t, `{"errcode":0,"errmsg":"ok"}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeWeCom, Endpoint: capture.server.URL + "/cgi-bin/webhook/send?key=k"}
	text, kept := buildLongSyncAlertText(500)
	if _, err := sendNotification(context.Background(), channel, textPayload(channel.Type, text)); err != nil {
		t.Fatal(err)
	}
	var body struct {
		Text struct {
			Content string `json:"content"`
		} `json:"text"`
	}
	if err := json.Unmarshal(capture.lastBody(t), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Text.Content) > weComTextContentMaxBytes || !strings.HasSuffix(body.Text.Content, kept[len(kept)-1]) {
		t.Fatalf("wecom content must fit 2048 bytes and end with the link, got %d bytes", len(body.Text.Content))
	}
}

// D：没有可裁剪字段时（整段都是标题 / 受保护字段）退回整体截断，但链接仍接在最后。
// D: without shrinkable fields the text falls back to a plain cut, still re-appending the link.
func TestShrinkNotificationText_FallbackKeepsLink(t *testing.T) {
	link := "链接：http://stx-console.example:17880/monitoring?tab=alerts"
	text := strings.Repeat("超长标题", 1000) + "\n" + link
	got := shrinkNotificationText(text, weComTextContentMaxBytes, func(v string) int { return len(v) })
	if len(got) > weComTextContentMaxBytes || !strings.HasSuffix(got, link) || !utf8.ValidString(got) {
		t.Fatalf("fallback must fit and keep the link, got %d bytes", len(got))
	}
}

// B：通用 Webhook 配置了签名密钥时带 X-STX-Timestamp / X-STX-Signature（HMAC-SHA256），没有密钥时不带。
// B: a generic webhook with a signing secret sends X-STX-Timestamp / X-STX-Signature (HMAC-SHA256); none without.
func TestSendNotification_GenericWebhookSignatureHeaders(t *testing.T) {
	// 期望值：printf '1793245483.{"a":1}' | openssl dgst -sha256 -hmac 'whsec-fake-123'
	// Expected: printf '1793245483.{"a":1}' | openssl dgst -sha256 -hmac 'whsec-fake-123'
	if got := signWebhookBody("whsec-fake-123", 1793245483, []byte(`{"a":1}`)); got != "bbb192279a8388f7990bc13010529b02beb8e99f36a22931c9860bd847438d39" {
		t.Fatalf("webhook signature mismatch: %s", got)
	}

	type captured struct {
		header http.Header
		body   []byte
	}
	requests := make(chan captured, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requests <- captured{header: r.Header.Clone(), body: raw}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	channel := &NotificationChannel{Type: NotificationChannelTypeWebhook, Endpoint: server.URL + "/hook", Secret: "whsec-fake-123"}
	if _, err := sendNotification(context.Background(), channel, map[string]interface{}{"a": 1}); err != nil {
		t.Fatal(err)
	}
	got := <-requests
	timestamp := got.header.Get(webhookTimestampHeader)
	signature := got.header.Get(webhookSignatureHeader)
	var ts int64
	if _, err := fmt.Sscan(timestamp, &ts); err != nil || time.Since(time.Unix(ts, 0)) > time.Minute {
		t.Fatalf("bad timestamp header %q", timestamp)
	}
	if want := "sha256=" + signWebhookBody("whsec-fake-123", ts, got.body); signature != want {
		t.Fatalf("signature header %q, want %q", signature, want)
	}

	channel.Secret = ""
	if _, err := sendNotification(context.Background(), channel, map[string]interface{}{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if got = <-requests; got.header.Get(webhookSignatureHeader) != "" || got.header.Get(webhookTimestampHeader) != "" {
		t.Fatalf("no signature headers expected without a secret")
	}
}

// 复审 1：错误摘要里以"状态：/链接："开头的行不能被当成受保护字段，真实的策略和控制台链接必须保留在末尾。
// Re-review 1: lines inside the error summary that start with "状态：" / "链接：" must not count as protected fields;
// the real policy and console link must stay at the end.
func TestShrinkNotificationText_InjectedProtectedLinesInSummary(t *testing.T) {
	head := "[STX][恢复][严重] 任务 订单中心-MySQL到PG-实时同步 · 作业运行失败\n状态：已恢复\n规则：作业运行失败\n集群：prod-seatunnel-3x (1)\n任务：订单中心-MySQL到PG-实时同步 (#16)\n恢复时间：2026-10-29 10:43:00 (UTC+8)\n"
	policy := "策略：订单库 CDC 作业告警 (12)"
	link := "链接：http://stx-console.example:17880/sync?job=1088&task=16"
	tail := "\n" + policy + "\n" + link
	for name, summary := range map[string]string{
		"injected-status-line":      "错误摘要：上游错误\n状态：失败\n" + strings.Repeat("坏", 600),
		"injected-link-line":        "错误摘要：上游错误\n链接：http://evil.example/x\n" + strings.Repeat("坏", 600),
		"injected-link-last-line":   "错误摘要：" + strings.Repeat("坏", 600) + "\n链接：http://evil.example/x",
		"injected-long-status-line": "错误摘要：上游错误\n状态：" + strings.Repeat("坏", 600),
	} {
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				limit int
				size  func(string) int
			}{
				{"wecom", weComTextContentMaxBytes, func(v string) int { return len(v) }},
				{"tiny", 700, func(v string) int { return len(v) }},
			} {
				got := shrinkNotificationText(head+summary+tail, tc.limit, tc.size)
				if len(got) > tc.limit || !utf8.ValidString(got) {
					t.Fatalf("%s: %d bytes exceeds %d", tc.name, len(got), tc.limit)
				}
				if !strings.HasSuffix(got, policy+"\n"+link) {
					t.Fatalf("%s: real policy and link must end the text:\n%s", tc.name, got[max(0, len(got)-300):])
				}
				if !strings.HasPrefix(got, head[:strings.Index(head, "\n")]) {
					t.Fatalf("%s: title lost", tc.name)
				}
			}
		})
	}
}

// 复审 2：测试不存在的渠道返回 404（已保存渠道测试和带 channel_id 的草稿测试）。
// Re-review 2: testing a missing channel returns 404 (saved channel test and draft test with channel_id).
func TestNotificationChannelTestHandler_MissingChannelIs404(t *testing.T) {
	service, _, _, _ := setupMonitoringAlertPolicyService(t)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handler := NewHandler(service)
	router.POST("/notification-channels/test", handler.TestNotificationChannelDraft)
	router.POST("/notification-channels/:id/test", handler.TestNotificationChannel)

	if rec := doJSON(t, router, http.MethodPost, "/notification-channels/999999/test", map[string]interface{}{}); rec.Code != http.StatusNotFound {
		t.Fatalf("missing channel test must be 404, got %d %s", rec.Code, rec.Body.String())
	}
	rec := doJSON(t, router, http.MethodPost, "/notification-channels/test", map[string]interface{}{
		"channel_id": 999999,
		"channel":    map[string]interface{}{"name": "x", "type": "webhook", "endpoint": "https://hooks.example.com/x"},
	})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("draft test for a missing channel must be 404, got %d %s", rec.Code, rec.Body.String())
	}
}
