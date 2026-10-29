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
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	gosync "sync"
	"testing"
	"time"
	"unicode/utf8"
)

// 期望值由独立实现计算后硬编码（python hmac/base64 与 openssl dgst -hmac 结果一致）：
//
//	python3 -c "import hmac,hashlib,base64;s='SECfakeDingSecret123';ts=1793245483000;\
//	  print(base64.b64encode(hmac.new(s.encode(),f'{ts}\n{s}'.encode(),hashlib.sha256).digest()).decode())"
//	printf '1793245483000\nSECfakeDingSecret123' | openssl dgst -sha256 -hmac 'SECfakeDingSecret123' -binary | base64
//	printf '' | openssl dgst -sha256 -hmac "$(printf '1793245483\nfakeFeishuSecret')" -binary | base64
//
// Expected values were computed with independent implementations (python hmac/base64 and openssl dgst -hmac agree).
const (
	dingTalkVectorSecret    = "SECfakeDingSecret123"
	dingTalkVectorTimestamp = int64(1793245483000)
	dingTalkVectorSign      = "oSJsCf1Rm750/+TfPMzxThN39Loy6Bu4DXiDhu1x2go="
	dingTalkVectorSignURL   = "oSJsCf1Rm750%2F%2BTfPMzxThN39Loy6Bu4DXiDhu1x2go%3D"
	feishuVectorSecret      = "fakeFeishuSecret"
	feishuVectorTimestamp   = int64(1793245483)
	feishuVectorSign        = "kLqg9DWWSFA+zL9pwtS5Hesg1t5xr5v33RIZprI94yA="
)

func TestSignDingTalk_KnownVector(t *testing.T) {
	if got := signDingTalk(dingTalkVectorSecret, dingTalkVectorTimestamp); got != dingTalkVectorSign {
		t.Fatalf("dingtalk sign mismatch: got %s want %s", got, dingTalkVectorSign)
	}
	signed, err := signDingTalkEndpoint("https://oapi.dingtalk.com/robot/send?access_token=abc", dingTalkVectorSecret, time.UnixMilli(dingTalkVectorTimestamp))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signed, "sign="+dingTalkVectorSignURL) || !strings.Contains(signed, "timestamp=1793245483000") || !strings.Contains(signed, "access_token=abc") {
		t.Fatalf("signed url missing url-encoded sign/timestamp/token: %s", signed)
	}
}

func TestSignFeishu_KnownVector(t *testing.T) {
	if got := signFeishu(feishuVectorSecret, feishuVectorTimestamp); got != feishuVectorSign {
		t.Fatalf("feishu sign mismatch: got %s want %s", got, feishuVectorSign)
	}
}

// imCapture 是一个按路径返回固定响应的假 IM 平台，记录收到的请求。
// imCapture is a fake IM platform that replies with a fixed body and records requests.
type imCapture struct {
	mu       gosync.Mutex
	queries  []url.Values
	bodies   [][]byte
	response string
	status   int
	server   *httptest.Server
}

func newIMCapture(t *testing.T, response string) *imCapture {
	t.Helper()
	capture := &imCapture{response: response, status: http.StatusOK}
	capture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		capture.mu.Lock()
		capture.queries = append(capture.queries, r.URL.Query())
		capture.bodies = append(capture.bodies, raw)
		status, response := capture.status, capture.response
		capture.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(capture.server.Close)
	return capture
}

func (c *imCapture) set(status int, response string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status, c.response = status, response
}

func (c *imCapture) lastBody(t *testing.T) []byte {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.bodies) == 0 {
		t.Fatalf("no request captured")
	}
	return c.bodies[len(c.bodies)-1]
}

func (c *imCapture) lastQuery(t *testing.T) url.Values {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.queries) == 0 {
		t.Fatalf("no request captured")
	}
	return c.queries[len(c.queries)-1]
}

func textPayload(channelType NotificationChannelType, text string) map[string]interface{} {
	if channelType == NotificationChannelTypeFeishu {
		return map[string]interface{}{"msg_type": "text", "content": map[string]string{"text": text}}
	}
	return map[string]interface{}{"msgtype": "text", "text": map[string]string{"content": text}}
}

func TestSendNotification_DingTalkSignsURL(t *testing.T) {
	capture := newIMCapture(t, `{"errcode":0,"errmsg":"ok"}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeDingTalk, Endpoint: capture.server.URL + "/robot/send?access_token=FAKE-DING-TOKEN", Secret: dingTalkVectorSecret}
	if _, err := sendNotification(context.Background(), channel, textPayload(channel.Type, "hello")); err != nil {
		t.Fatalf("send: %v", err)
	}
	query := capture.lastQuery(t)
	ts, err := strconv.ParseInt(query.Get("timestamp"), 10, 64)
	if err != nil || ts < time.Now().Add(-time.Minute).UnixMilli() {
		t.Fatalf("timestamp must be current milliseconds, got %q", query.Get("timestamp"))
	}
	if query.Get("sign") != signDingTalk(dingTalkVectorSecret, ts) || query.Get("access_token") != "FAKE-DING-TOKEN" {
		t.Fatalf("unexpected signed query: %v", query)
	}
}

func TestSendNotification_FeishuSignsBody(t *testing.T) {
	capture := newIMCapture(t, `{"code":0,"msg":"success","data":{}}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeFeishu, Endpoint: capture.server.URL + "/open-apis/bot/v2/hook/FAKE", Secret: feishuVectorSecret}
	if _, err := sendNotification(context.Background(), channel, textPayload(channel.Type, "hello")); err != nil {
		t.Fatalf("send: %v", err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(capture.lastBody(t), &body); err != nil {
		t.Fatal(err)
	}
	tsText, _ := body["timestamp"].(string)
	ts, err := strconv.ParseInt(tsText, 10, 64)
	if err != nil || ts < time.Now().Add(-time.Minute).Unix() || ts > time.Now().Add(time.Minute).Unix() {
		t.Fatalf("timestamp must be current seconds as a string, got %#v", body["timestamp"])
	}
	if body["sign"] != signFeishu(feishuVectorSecret, ts) || body["msg_type"] != "text" {
		t.Fatalf("unexpected signed body: %#v", body)
	}
}

func TestSendNotification_WithoutSecretDoesNotSign(t *testing.T) {
	capture := newIMCapture(t, `{"errcode":0,"errmsg":"ok"}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeDingTalk, Endpoint: capture.server.URL + "/robot/send?access_token=x"}
	if _, err := sendNotification(context.Background(), channel, textPayload(channel.Type, "hello")); err != nil {
		t.Fatalf("send: %v", err)
	}
	if query := capture.lastQuery(t); query.Get("sign") != "" || query.Get("timestamp") != "" {
		t.Fatalf("no secret means no signature, got %v", query)
	}
}

func TestSendNotification_PlatformErrorCodes(t *testing.T) {
	cases := []struct {
		name        string
		channelType NotificationChannelType
		response    string
		wantErr     string
	}{
		{"dingtalk ok", NotificationChannelTypeDingTalk, `{"errcode":0,"errmsg":"ok"}`, ""},
		{"dingtalk sign mismatch", NotificationChannelTypeDingTalk, `{"errcode": 310000, "errmsg": "sign not match, more: [https://ding-doc.dingtalk.com/doc#/serverapi2/qf2nxq]"}`, "errcode=310000"},
		{"wecom ok", NotificationChannelTypeWeCom, `{"errcode":0,"errmsg":"ok"}`, ""},
		{"wecom invalid url", NotificationChannelTypeWeCom, `{"errcode": 93000, "errmsg": "invalid webhook url"}`, "errcode=93000"},
		{"wecom missing errcode", NotificationChannelTypeWeCom, `{"ok":true}`, "no errcode"},
		{"feishu ok", NotificationChannelTypeFeishu, `{"code":0,"msg":"success","data":{}}`, ""},
		{"feishu legacy ok", NotificationChannelTypeFeishu, `{"StatusCode":0,"StatusMessage":"success"}`, ""},
		{"feishu sign fail", NotificationChannelTypeFeishu, `{"code": 19021, "msg": "sign match fail or timestamp is not within one hour from current time", "data": {}}`, "code=19021"},
		{"feishu non json", NotificationChannelTypeFeishu, `<html>bad gateway</html>`, "unrecognized response"},
		{"webhook ignores body", NotificationChannelTypeWebhook, `{"errcode":1}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			capture := newIMCapture(t, tc.response)
			channel := &NotificationChannel{Type: tc.channelType, Endpoint: capture.server.URL + "/hook"}
			attempt, err := sendNotification(context.Background(), channel, textPayload(tc.channelType, "hello"))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected success, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if attempt == nil || attempt.StatusCode != http.StatusOK || attempt.ResponseBody != tc.response {
				t.Fatalf("attempt must keep the platform response for display: %+v", attempt)
			}
		})
	}
}

func TestService_TestNotificationChannel_PlatformErrorIsFailure(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)
	capture := newIMCapture(t, `{"errcode": 310000, "errmsg": "sign not match"}`)
	channel := &NotificationChannel{Name: "ding", Type: NotificationChannelTypeDingTalk, Enabled: true, Endpoint: capture.server.URL + "/robot/send?access_token=FAKE-DING-TOKEN2", Secret: "SECwrong"}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatal(err)
	}
	result, err := service.TestNotificationChannel(ctx, channel.ID, &NotificationChannelTestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(NotificationDeliveryStatusFailed) || !strings.Contains(result.LastError, "310000") ||
		!strings.Contains(result.ResponseBody, "sign not match") || result.StatusCode != http.StatusOK {
		t.Fatalf("HTTP 200 + errcode must be a failure with the platform body: %+v", result)
	}
}

func TestNotificationChannelDTO_RedactsSecretsAndUpdateKeepsThem(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)
	enabled := true
	ding, err := service.CreateNotificationChannel(ctx, &UpsertNotificationChannelRequest{
		Name: "ding", Type: NotificationChannelTypeDingTalk, Enabled: &enabled,
		Endpoint: "https://oapi.dingtalk.com/robot/send?access_token=FAKE-DING-TOKEN", Secret: "SECfakeDingSecret123",
	})
	if err != nil {
		t.Fatal(err)
	}
	feishu, err := service.CreateNotificationChannel(ctx, &UpsertNotificationChannelRequest{
		Name: "feishu", Type: NotificationChannelTypeFeishu, Enabled: &enabled,
		Endpoint: "https://open.feishu.cn/open-apis/bot/v2/hook/FAKE-FEISHU-HOOK", Secret: "fakeFeishuSecret",
	})
	if err != nil {
		t.Fatal(err)
	}
	email, err := service.CreateNotificationChannel(ctx, &UpsertNotificationChannelRequest{
		Name: "mail", Type: NotificationChannelTypeEmail, Enabled: &enabled,
		Config: &NotificationChannelConfig{Email: &NotificationChannelEmailConfig{
			Security: NotificationEmailSecurityStartTLS, Host: "smtp.example.com", Port: 587,
			Username: "u@example.com", Password: "FAKE-SMTP-PASSWORD", From: "u@example.com", Recipients: []string{"ops@example.com"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := service.ListNotificationChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(list)
	for _, secret := range []string{"FAKE-DING-TOKEN", "SECfakeDingSecret123", "FAKE-FEISHU-HOOK", "fakeFeishuSecret", "FAKE-SMTP-PASSWORD"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("GET /notification-channels leaks %q: %s", secret, raw)
		}
	}
	var listedDing, listedMail *NotificationChannelDTO
	for _, item := range list.Channels {
		switch item.ID {
		case ding.ID:
			listedDing = item
		case email.ID:
			listedMail = item
		}
	}
	if listedDing.Secret != notificationSecretMask || listedDing.Endpoint != "https://oapi.dingtalk.com/robot/send?access_token=******" {
		t.Fatalf("unexpected masked dingtalk channel: %+v", listedDing)
	}
	if listedMail.Config.Email.Password != notificationSecretMask || listedMail.Config.Email.Username != "u@example.com" {
		t.Fatalf("unexpected masked email config: %+v", listedMail.Config.Email)
	}

	// 原样传回脱敏值：真实值保持不变。
	// Sending the masked values back keeps the real values.
	if _, err := service.UpdateNotificationChannel(ctx, ding.ID, &UpsertNotificationChannelRequest{
		Name: "ding-renamed", Type: NotificationChannelTypeDingTalk, Enabled: &enabled, Endpoint: listedDing.Endpoint, Secret: listedDing.Secret,
	}); err != nil {
		t.Fatal(err)
	}
	// 空密钥同样表示保持不变。
	// An empty secret also means keep unchanged.
	if _, err := service.UpdateNotificationChannel(ctx, feishu.ID, &UpsertNotificationChannelRequest{
		Name: "feishu", Type: NotificationChannelTypeFeishu, Enabled: &enabled, Endpoint: "https://open.feishu.cn/open-apis/bot/v2/hook/******", Secret: "",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.UpdateNotificationChannel(ctx, email.ID, &UpsertNotificationChannelRequest{
		Name: "mail", Type: NotificationChannelTypeEmail, Enabled: &enabled, Config: listedMail.Config,
	}); err != nil {
		t.Fatal(err)
	}
	storedDing, _ := repo.GetNotificationChannelByID(ctx, ding.ID)
	storedFeishu, _ := repo.GetNotificationChannelByID(ctx, feishu.ID)
	storedMail, _ := repo.GetNotificationChannelByID(ctx, email.ID)
	if storedDing.Name != "ding-renamed" || storedDing.Secret != "SECfakeDingSecret123" || storedDing.Endpoint != "https://oapi.dingtalk.com/robot/send?access_token=FAKE-DING-TOKEN" {
		t.Fatalf("masked update must keep stored dingtalk values: %+v", storedDing)
	}
	if storedFeishu.Secret != "fakeFeishuSecret" || storedFeishu.Endpoint != "https://open.feishu.cn/open-apis/bot/v2/hook/FAKE-FEISHU-HOOK" {
		t.Fatalf("masked update must keep stored feishu values: %+v", storedFeishu)
	}
	if cfg := unmarshalNotificationChannelConfig(storedMail.Type, storedMail.ConfigJSON); cfg.Email.Password != "FAKE-SMTP-PASSWORD" {
		t.Fatalf("masked update must keep the SMTP password, got %q", cfg.Email.Password)
	}

	// 新值照常覆盖；改了地址却还带着占位符要报错。
	// New values overwrite as usual; an edited address still holding the placeholder is rejected.
	if _, err := service.UpdateNotificationChannel(ctx, ding.ID, &UpsertNotificationChannelRequest{
		Name: "ding", Type: NotificationChannelTypeDingTalk, Enabled: &enabled, Endpoint: "https://oapi.dingtalk.com/robot/send?access_token=NEW", Secret: "SECnew",
	}); err != nil {
		t.Fatal(err)
	}
	storedDing, _ = repo.GetNotificationChannelByID(ctx, ding.ID)
	if storedDing.Secret != "SECnew" || !strings.HasSuffix(storedDing.Endpoint, "access_token=NEW") {
		t.Fatalf("new values must be saved: %+v", storedDing)
	}
	if _, err := service.UpdateNotificationChannel(ctx, ding.ID, &UpsertNotificationChannelRequest{
		Name: "ding", Type: NotificationChannelTypeDingTalk, Enabled: &enabled, Endpoint: "https://other.example.com/robot/send?access_token=******",
	}); err == nil {
		t.Fatalf("an edited endpoint with the placeholder must be rejected")
	}
}

func TestService_TestNotificationChannelDraft_ReusesSavedSecret(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)
	capture := newIMCapture(t, `{"errcode":0,"errmsg":"ok"}`)
	channel := &NotificationChannel{Name: "ding", Type: NotificationChannelTypeDingTalk, Enabled: true, Endpoint: capture.server.URL + "/robot/send?access_token=REAL-TOKEN", Secret: dingTalkVectorSecret}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatal(err)
	}
	dto := toNotificationChannelDTO(channel)
	enabled := true
	result, err := service.TestNotificationChannelDraft(ctx, &NotificationChannelDraftTestRequest{
		ChannelID: channel.ID,
		Channel:   &UpsertNotificationChannelRequest{Name: "ding (edited)", Type: channel.Type, Enabled: &enabled, Endpoint: dto.Endpoint, Secret: dto.Secret},
	})
	if err != nil || result.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("draft test with masked values should use the saved ones: %+v err=%v", result, err)
	}
	query := capture.lastQuery(t)
	ts, _ := strconv.ParseInt(query.Get("timestamp"), 10, 64)
	if query.Get("access_token") != "REAL-TOKEN" || query.Get("sign") != signDingTalk(dingTalkVectorSecret, ts) {
		t.Fatalf("draft test must use the real token and secret: %v", query)
	}
}

func TestSendNotification_RedactsTokenInErrors(t *testing.T) {
	service, repo, _, ctx := setupMonitoringAlertPolicyService(t)
	channel := &NotificationChannel{Name: "leaky", Type: NotificationChannelTypeWebhook, Enabled: true, Endpoint: "http://127.0.0.1:1/hook?token=LEAKY-TOKEN-SHOULD-BE-MASKED"}
	if err := repo.CreateNotificationChannel(ctx, channel); err != nil {
		t.Fatal(err)
	}
	_, sendErr := sendNotification(ctx, channel, map[string]interface{}{"title": "x"})
	if sendErr == nil || strings.Contains(sendErr.Error(), "LEAKY-TOKEN") || !strings.Contains(sendErr.Error(), "token=******") {
		t.Fatalf("send error must be redacted, got %v", sendErr)
	}
	var netErr net.Error
	if !errors.As(sendErr, &netErr) {
		t.Fatalf("redacted error should still unwrap to the network error: %T", sendErr)
	}

	result, err := service.TestNotificationChannel(ctx, channel.ID, &NotificationChannelTestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != string(NotificationDeliveryStatusFailed) || strings.Contains(result.LastError, "LEAKY-TOKEN") {
		t.Fatalf("test result leaks token: %+v", result)
	}
	deliveries, err := service.ListNotificationDeliveries(ctx, &NotificationDeliveryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	stored, _, _ := repo.ListNotificationDeliveries(ctx, &NotificationDeliveryFilter{})
	raw, _ := json.Marshal(deliveries)
	storedRaw, _ := json.Marshal(stored)
	if strings.Contains(string(raw), "LEAKY-TOKEN") || strings.Contains(string(storedRaw), "LEAKY-TOKEN") {
		t.Fatalf("delivery records leak token: %s", storedRaw)
	}

	// 历史记录（修复前写入的明文 URL）在返回时也会脱敏。
	// Historical records (plain URLs written before the fix) are redacted on read as well.
	legacy := toNotificationDeliveryDTO(&NotificationDelivery{LastError: `Post "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=FAKE-WECOM-KEY": EOF`})
	if strings.Contains(legacy.LastError, "FAKE-WECOM-KEY") {
		t.Fatalf("legacy last_error must be redacted on read: %s", legacy.LastError)
	}
}

func TestRedactEndpointURL(t *testing.T) {
	cases := map[string]string{
		"https://oapi.dingtalk.com/robot/send?access_token=abc":                 "https://oapi.dingtalk.com/robot/send?access_token=******",
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=FAKE-WECOM-KEY":   "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=******",
		"https://open.feishu.cn/open-apis/bot/v2/hook/FAKE-FEISHU-HOOK":         "https://open.feishu.cn/open-apis/bot/v2/hook/******",
		"https://hooks.slack.com/services/T000/B000/abcdefghijklmnop1234567890": "https://hooks.slack.com/services/T000/B000/******",
		"https://user:pass@example.com/notify":                                  "https://user:******@example.com/notify",
		"https://example.com/notify":                                            "https://example.com/notify",
	}
	for input, want := range cases {
		if got := redactEndpointURL(input); got != want {
			t.Fatalf("redact %q: got %q want %q", input, got, want)
		}
	}
}

func TestTruncateUTF8Bytes_MultiByteSafe(t *testing.T) {
	value := strings.Repeat("告警", 10) // 60 bytes
	for limit := 1; limit <= 61; limit++ {
		got := truncateUTF8Bytes(value, limit, "…")
		if len(got) > limit || !utf8.ValidString(got) {
			t.Fatalf("limit %d: got %d bytes valid=%v", limit, len(got), utf8.ValidString(got))
		}
	}
	if got := truncateUTF8Bytes("短文本", 100, "…"); got != "短文本" {
		t.Fatalf("short text must be unchanged, got %q", got)
	}
}

func TestSendNotification_WeComTruncatesTo2048Bytes(t *testing.T) {
	capture := newIMCapture(t, `{"errcode":0,"errmsg":"ok"}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeWeCom, Endpoint: capture.server.URL + "/cgi-bin/webhook/send?key=k"}
	// 复现实测：同步告警 + 500 字中文错误摘要，正文 2194 字节。
	// Reproduce the measurement: a sync alert with a 500-character Chinese error summary, 2194 bytes of text.
	text := "[STX][告警][严重] 任务 mysql-cdc-schema-demo · 作业运行失败\n错误摘要：" + strings.Repeat("连接数据库失败。", 90)
	if len(text) <= weComTextContentMaxBytes {
		t.Fatalf("fixture must exceed the limit, got %d bytes", len(text))
	}
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
	if len(body.Text.Content) > weComTextContentMaxBytes || !utf8.ValidString(body.Text.Content) ||
		!strings.HasSuffix(body.Text.Content, notificationFieldTruncatedSuffix) || !strings.HasPrefix(body.Text.Content, "[STX][告警][严重]") {
		t.Fatalf("wecom content must be <=2048 valid UTF-8 bytes with a marker, got %d bytes", len(body.Text.Content))
	}
}

func TestSendNotification_FeishuBodyFits20KB(t *testing.T) {
	capture := newIMCapture(t, `{"code":0,"msg":"success"}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeFeishu, Endpoint: capture.server.URL + "/open-apis/bot/v2/hook/x", Secret: feishuVectorSecret}
	// 换行和 <> 会被 JSON 转义放大，中文是 3 字节。
	// Newlines and <> are inflated by JSON escaping and Chinese is 3 bytes.
	text := strings.Repeat("超长描述<ABC>\n", 3000)
	if _, err := sendNotification(context.Background(), channel, textPayload(channel.Type, text)); err != nil {
		t.Fatal(err)
	}
	raw := capture.lastBody(t)
	if len(raw) > feishuRequestBodyMaxBytes {
		t.Fatalf("feishu body must be <= 20KB including signature, got %d bytes", len(raw))
	}
	var body struct {
		Content struct {
			Text string `json:"text"`
		} `json:"content"`
		Sign string `json:"sign"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(body.Content.Text) || !strings.HasSuffix(body.Content.Text, notificationTruncatedSuffix) || body.Sign == "" {
		t.Fatalf("feishu text must stay valid UTF-8, carry a marker and be signed")
	}
	if len(raw) < feishuRequestBodyMaxBytes-2048 {
		t.Fatalf("truncation should keep as much as fits, got only %d bytes", len(raw))
	}
}

func TestSendNotification_DingTalkBodyFits(t *testing.T) {
	capture := newIMCapture(t, `{"errcode":0,"errmsg":"ok"}`)
	channel := &NotificationChannel{Type: NotificationChannelTypeDingTalk, Endpoint: capture.server.URL + "/robot/send?access_token=x"}
	if _, err := sendNotification(context.Background(), channel, textPayload(channel.Type, strings.Repeat("钉钉", 10000))); err != nil {
		t.Fatal(err)
	}
	if raw := capture.lastBody(t); len(raw) > dingTalkRequestBodyMaxBytes || !utf8.Valid(raw) {
		t.Fatalf("dingtalk body must fit %d bytes, got %d", dingTalkRequestBodyMaxBytes, len(raw))
	}
}

func TestRemoteAndLocalAlertLongFieldsAreTruncated(t *testing.T) {
	record := &RemoteAlertRecord{AlertName: "HeapHigh", Severity: "warning", ClusterID: "1", Description: strings.Repeat("超长描述ABC", 1000)}
	text := buildRemoteAlertMessageText(record, NotificationDeliveryEventTypeFiring)
	if len(text) > 4000 || !utf8.ValidString(text) || !strings.Contains(text, "已截断") {
		t.Fatalf("remote description must be truncated, got %d bytes", len(text))
	}
}

// silentSMTPServer 只接受 TCP 连接，从不发送 greeting。
// silentSMTPServer accepts TCP connections and never sends a greeting.
func silentSMTPServer(t *testing.T) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu gosync.Mutex
	var conns []net.Conn
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()
		}
	}()
	return listener.Addr().String(), func() {
		_ = listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range conns {
			_ = conn.Close()
		}
	}
}

func TestSendEmailNotification_SilentServerTimesOut(t *testing.T) {
	previousDial, previousIO := smtpDialTimeout, smtpIOTimeout
	smtpDialTimeout, smtpIOTimeout = 200*time.Millisecond, 300*time.Millisecond
	t.Cleanup(func() { smtpDialTimeout, smtpIOTimeout = previousDial, previousIO })

	addr, stop := silentSMTPServer(t)
	defer stop()
	host, portText, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portText)
	configJSON, _ := json.Marshal(&NotificationChannelConfig{Email: &NotificationChannelEmailConfig{
		Protocol: "smtp", Security: NotificationEmailSecurityNone, Host: host, Port: port,
		From: "alerts@example.com", Recipients: []string{"ops@example.com"},
	}})
	channel := &NotificationChannel{Type: NotificationChannelTypeEmail, ConfigJSON: string(configJSON)}

	started := time.Now()
	_, err := sendNotification(context.Background(), channel, &emailNotificationPayload{Subject: "s", Text: "t"})
	elapsed := time.Since(started)
	if err == nil {
		t.Fatalf("a silent SMTP server must fail")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("send must return within the timeout, took %s", elapsed)
	}

	// ctx 先到期时以 ctx 为准。
	// A ctx deadline that comes first wins.
	smtpIOTimeout = 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started = time.Now()
	if _, err := sendNotification(ctx, channel, &emailNotificationPayload{Subject: "s", Text: "t"}); err == nil {
		t.Fatalf("expected ctx timeout error")
	}
	if elapsed := time.Since(started); elapsed > 3*time.Second {
		t.Fatalf("ctx deadline must interrupt the SMTP session, took %s", elapsed)
	}
}

func TestBuildSMTPMessage_DateAndMessageIDHeaders(t *testing.T) {
	message, err := buildSMTPMessage(&NotificationChannelEmailConfig{From: "alerts@example.com"}, &emailNotificationPayload{Subject: "s", Text: "t", To: []string{"ops@example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(string(message)))
	if err != nil {
		t.Fatal(err)
	}
	if date, err := parsed.Header.Date(); err != nil || time.Since(date) > time.Minute {
		t.Fatalf("Date header missing or invalid: %q err=%v", parsed.Header.Get("Date"), err)
	}
	id := parsed.Header.Get("Message-ID")
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, "@example.com>") {
		t.Fatalf("Message-ID must be <...@sender-domain>, got %q", id)
	}
	other, _ := buildSMTPMessage(&NotificationChannelEmailConfig{From: "alerts@example.com"}, &emailNotificationPayload{Subject: "s", Text: "t", To: []string{"ops@example.com"}})
	otherParsed, _ := mail.ReadMessage(strings.NewReader(string(other)))
	if otherParsed.Header.Get("Message-ID") == id {
		t.Fatalf("Message-ID must be unique per message")
	}
}

func TestSendEmailNotification_HeloUsesHostname(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()
	host, portText, _ := net.SplitHostPort(server.addr())
	port, _ := strconv.Atoi(portText)
	configJSON, _ := json.Marshal(&NotificationChannelConfig{Email: &NotificationChannelEmailConfig{
		Protocol: "smtp", Security: NotificationEmailSecurityNone, Host: host, Port: port,
		From: "alerts@example.com", Recipients: []string{"ops@example.com"},
	}})
	channel := &NotificationChannel{Type: NotificationChannelTypeEmail, ConfigJSON: string(configJSON)}
	if _, err := sendNotification(context.Background(), channel, &emailNotificationPayload{Subject: "s", Text: "t"}); err != nil {
		t.Fatal(err)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if server.hello != smtpHelloName() || server.hello == "" {
		t.Fatalf("HELO must use the real hostname %q, got %q", smtpHelloName(), server.hello)
	}
	if !strings.Contains(server.data, "Message-ID: <") || !strings.Contains(server.data, "Date: ") {
		t.Fatalf("delivered message must carry Date and Message-ID headers")
	}
}
