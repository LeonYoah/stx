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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/LeonYoah/stx/internal/config"
)

// IM 机器人（钉钉 / 企业微信 / 飞书）相关的签名、平台错误码解析、长度裁剪和敏感信息脱敏。
// Signing, platform error-code parsing, size fitting and secret redaction for IM bots (DingTalk / WeCom / Feishu).

const (
	// notificationSecretMask 是 GET 接口里替换密钥、SMTP 密码和 URL token 的占位符；更新时原样传回表示"保持不变"。
	// notificationSecretMask replaces secrets, SMTP passwords and URL tokens in GET responses; sending it back on
	// update means "keep the stored value".
	notificationSecretMask = "******"

	// weComTextContentMaxBytes 企业微信机器人 text.content 最长 2048 字节（UTF-8）。
	// weComTextContentMaxBytes is the WeCom bot text.content limit: 2048 bytes (UTF-8).
	weComTextContentMaxBytes = 2048
	// feishuRequestBodyMaxBytes 飞书自定义机器人请求体最大 20KB。
	// feishuRequestBodyMaxBytes is the Feishu custom bot request body limit: 20KB.
	feishuRequestBodyMaxBytes = 20 * 1024
	// dingTalkRequestBodyMaxBytes 钉钉自定义机器人消息体上限按 20000 字节控制。
	// dingTalkRequestBodyMaxBytes caps the DingTalk custom bot body at 20000 bytes.
	dingTalkRequestBodyMaxBytes = 20000
	// imSignatureReserveBytes 给飞书签名字段（timestamp + sign）预留的字节数。
	// imSignatureReserveBytes is reserved for the Feishu signature fields (timestamp + sign).
	imSignatureReserveBytes = 128

	notificationTruncatedSuffix = "\n…（内容过长，已截断）"

	// alertFieldTextMaxBytes 是单个长文本字段（远程告警说明、本地进程详情、策略说明）在通知里的上限。
	// alertFieldTextMaxBytes caps one long text field (remote alert description, local process details, policy
	// description) inside a notification.
	alertFieldTextMaxBytes = 2000
)

// monitoringAlertsConsoleLink 返回告警中心深链 {console_url}/monitoring?tab=alerts[&cluster_id=]；未配置控制台地址时为空。
// monitoringAlertsConsoleLink returns the alert center deep link {console_url}/monitoring?tab=alerts[&cluster_id=];
// empty without a console URL.
func monitoringAlertsConsoleLink(clusterID string) string {
	base := config.GetConsoleURL()
	if base == "" {
		return ""
	}
	query := url.Values{}
	query.Set("tab", "alerts")
	if clusterID = strings.TrimSpace(clusterID); clusterID != "" && clusterID != "0" {
		query.Set("cluster_id", clusterID)
	}
	return base + "/monitoring?" + query.Encode()
}

// truncateAlertFieldText 按字节裁剪通知里的长文本字段，多字节安全。
// truncateAlertFieldText trims a long notification text field by bytes, multi-byte safe.
func truncateAlertFieldText(value string) string {
	return truncateUTF8Bytes(strings.TrimSpace(value), alertFieldTextMaxBytes, "…（已截断）")
}

// signDingTalk 计算钉钉加签：Base64(HmacSHA256(key=secret, msg="<timestamp_ms>\n<secret>"))，调用方再做 URL 编码。
// signDingTalk computes the DingTalk signature: Base64(HmacSHA256(key=secret, msg="<timestamp_ms>\n<secret>")); the
// caller URL-encodes it.
func signDingTalk(secret string, timestampMillis int64) string {
	stringToSign := strconv.FormatInt(timestampMillis, 10) + "\n" + secret
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// signFeishu 计算飞书加签：Base64(HmacSHA256(key="<timestamp_s>\n<secret>", msg=空))。
// signFeishu computes the Feishu signature: Base64(HmacSHA256(key="<timestamp_s>\n<secret>", msg=empty)).
func signFeishu(secret string, timestampSeconds int64) string {
	stringToSign := strconv.FormatInt(timestampSeconds, 10) + "\n" + secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// signDingTalkEndpoint 在钉钉 webhook URL 上追加 timestamp 和 sign 查询参数。
// signDingTalkEndpoint appends the timestamp and sign query parameters to a DingTalk webhook URL.
func signDingTalkEndpoint(endpoint, secret string, now time.Time) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid dingtalk endpoint")
	}
	timestamp := now.UnixMilli()
	query := parsed.Query()
	query.Set("timestamp", strconv.FormatInt(timestamp, 10))
	query.Set("sign", signDingTalk(secret, timestamp))
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// signNotificationRequest 按渠道类型给 IM 请求加签：钉钉改 URL，飞书改请求体；没有配置密钥时原样返回。
// signNotificationRequest signs an IM request per channel type: DingTalk changes the URL and Feishu the body; without a
// secret everything is returned unchanged.
func signNotificationRequest(channel *NotificationChannel, endpoint string, payload interface{}, now time.Time) (string, interface{}, error) {
	secret := strings.TrimSpace(channel.Secret)
	if secret == "" {
		return endpoint, payload, nil
	}
	switch channel.Type {
	case NotificationChannelTypeDingTalk:
		signed, err := signDingTalkEndpoint(endpoint, secret, now)
		if err != nil {
			return "", nil, err
		}
		return signed, payload, nil
	case NotificationChannelTypeFeishu:
		body, ok := payload.(map[string]interface{})
		if !ok {
			return endpoint, payload, nil
		}
		cloned := make(map[string]interface{}, len(body)+2)
		for key, value := range body {
			cloned[key] = value
		}
		timestamp := now.Unix()
		cloned["timestamp"] = strconv.FormatInt(timestamp, 10)
		cloned["sign"] = signFeishu(secret, timestamp)
		return endpoint, cloned, nil
	default:
		return endpoint, payload, nil
	}
}

const (
	// webhookSignatureHeader / webhookTimestampHeader 是通用 Webhook 配置了签名密钥时附带的请求头。
	// webhookSignatureHeader / webhookTimestampHeader are sent by generic webhooks that have a signing secret.
	webhookSignatureHeader = "X-STX-Signature"
	webhookTimestampHeader = "X-STX-Timestamp"
)

// signWebhookBody 计算通用 Webhook 签名：hex(HmacSHA256(key=secret, msg="<timestamp_s>.<body>"))。
// signWebhookBody computes the generic webhook signature: hex(HmacSHA256(key=secret, msg="<timestamp_s>.<body>")).
func signWebhookBody(secret string, timestampSeconds int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(strconv.FormatInt(timestampSeconds, 10) + "."))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// applyWebhookSignatureHeaders 给配置了密钥的通用 Webhook 请求加上 X-STX-Timestamp 和
// X-STX-Signature: sha256=<hex>；接收方用同一密钥对 "<timestamp>.<原始请求体>" 计算 HMAC-SHA256 校验。
// applyWebhookSignatureHeaders adds X-STX-Timestamp and X-STX-Signature: sha256=<hex> to generic webhook requests
// with a secret; receivers verify with HMAC-SHA256 over "<timestamp>.<raw body>" using the same secret.
func applyWebhookSignatureHeaders(header http.Header, channel *NotificationChannel, body []byte, now time.Time) {
	if channel == nil || channel.Type != NotificationChannelTypeWebhook {
		return
	}
	secret := strings.TrimSpace(channel.Secret)
	if secret == "" {
		return
	}
	timestamp := now.Unix()
	header.Set(webhookTimestampHeader, strconv.FormatInt(timestamp, 10))
	header.Set(webhookSignatureHeader, "sha256="+signWebhookBody(secret, timestamp, body))
}

// checkPlatformResponse 解析 IM 平台在 HTTP 200 里返回的业务错误码：钉钉 / 企业微信看 errcode，飞书看 code（旧版为
// StatusCode）；非 0 或无法解析都算失败。
// checkPlatformResponse parses the business error code IM platforms return inside HTTP 200: DingTalk / WeCom use
// errcode, Feishu uses code (StatusCode on the legacy format); non-zero or unparseable counts as a failure.
func checkPlatformResponse(channelType NotificationChannelType, body []byte) error {
	switch channelType {
	case NotificationChannelTypeDingTalk, NotificationChannelTypeWeCom, NotificationChannelTypeFeishu:
	default:
		return nil
	}
	var parsed struct {
		ErrCode       *int64 `json:"errcode"`
		ErrMsg        string `json:"errmsg"`
		Code          *int64 `json:"code"`
		Msg           string `json:"msg"`
		StatusCode    *int64 `json:"StatusCode"`
		StatusMessage string `json:"StatusMessage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return fmt.Errorf("%s returned an unrecognized response: %s", channelType, truncateUTF8Bytes(strings.TrimSpace(string(body)), 200, "…"))
	}
	switch channelType {
	case NotificationChannelTypeFeishu:
		switch {
		case parsed.Code != nil:
			if *parsed.Code != 0 {
				return fmt.Errorf("feishu error: code=%d, msg=%s", *parsed.Code, parsed.Msg)
			}
			return nil
		case parsed.StatusCode != nil:
			if *parsed.StatusCode != 0 {
				return fmt.Errorf("feishu error: code=%d, msg=%s", *parsed.StatusCode, parsed.StatusMessage)
			}
			return nil
		}
		return fmt.Errorf("feishu response has no code field")
	default:
		if parsed.ErrCode == nil {
			return fmt.Errorf("%s response has no errcode field", channelType)
		}
		if *parsed.ErrCode != 0 {
			return fmt.Errorf("%s error: errcode=%d, errmsg=%s", channelType, *parsed.ErrCode, parsed.ErrMsg)
		}
		return nil
	}
}

// truncateUTF8Bytes 把字符串裁剪到 maxBytes 字节以内（含后缀），不会切断多字节字符。
// truncateUTF8Bytes trims the string to at most maxBytes bytes (suffix included) without splitting a multi-byte rune.
func truncateUTF8Bytes(value string, maxBytes int, suffix string) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	if len(suffix) >= maxBytes {
		suffix = ""
	}
	cut := maxBytes - len(suffix)
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut] + suffix
}

// imTextPayload 定位 IM 文本消息里的正文，返回正文和"写回正文后的新消息体"函数；不是文本消息时 ok 为 false。
// imTextPayload locates the text of an IM text message and returns it with a function that builds a new body holding
// replacement text; ok is false when the payload is not a text message.
func imTextPayload(channelType NotificationChannelType, payload interface{}) (string, func(string) map[string]interface{}, bool) {
	body, ok := payload.(map[string]interface{})
	if !ok {
		return "", nil, false
	}
	outerKey, innerKey := "text", "content"
	if channelType == NotificationChannelTypeFeishu {
		outerKey, innerKey = "content", "text"
	}
	var text string
	switch inner := body[outerKey].(type) {
	case map[string]string:
		text = inner[innerKey]
	case map[string]interface{}:
		text, _ = inner[innerKey].(string)
	default:
		return "", nil, false
	}
	rebuild := func(next string) map[string]interface{} {
		cloned := make(map[string]interface{}, len(body))
		for key, value := range body {
			cloned[key] = value
		}
		cloned[outerKey] = map[string]string{innerKey: next}
		return cloned
	}
	return text, rebuild, true
}

// notificationProtectedFieldLabels 是裁剪时必须原样保留的字段（标题总是保留）。
// notificationProtectedFieldLabels are the fields kept verbatim when fitting (the title is always kept).
var notificationProtectedFieldLabels = map[string]bool{
	"状态":   true,
	"策略":   true,
	"规则":   true,
	"告警名称": true,
	"级别":   true,
	"链接":   true,
}

// notificationFieldLinePattern 匹配 "标签：值" 形式的字段首行；不匹配的行是上一字段的续行（比如堆栈）。
// notificationFieldLinePattern matches the first line of a "label：value" field; other lines continue the previous
// field (a stack trace, for example).
var notificationFieldLinePattern = regexp.MustCompile(`^([\p{Han}A-Za-z0-9 _\-]{1,16})：`)

const (
	// notificationFieldMinBytes 是可裁剪字段至少保留的字节数。
	// notificationFieldMinBytes is the minimum a shrinkable field keeps.
	notificationFieldMinBytes        = 48
	notificationFieldTruncatedSuffix = "…（已截断）"
	// notificationProtectedFieldMaxBytes 是受保护字段（单行）的最大长度，超过就按普通长字段裁剪。
	// notificationProtectedFieldMaxBytes caps a protected (single-line) field; longer ones are shrunk like any field.
	notificationProtectedFieldMaxBytes = 1024
)

type notificationTextSegment struct {
	text      string
	label     string
	protected bool
}

// splitNotificationText 把通知正文拆成段：第一行是标题，之后每个 "标签：值" 字段一段（续行并入当前字段）。
// splitNotificationText splits a notification text into segments: the title line, then one segment per
// "label：value" field (continuation lines join the current field).
func splitNotificationText(text string) []notificationTextSegment {
	lines := strings.Split(text, "\n")
	segments := []notificationTextSegment{{text: lines[0], protected: true}}
	for _, line := range lines[1:] {
		if match := notificationFieldLinePattern.FindStringSubmatch(line); match != nil {
			segments = append(segments, notificationTextSegment{text: line, label: match[1], protected: notificationProtectedFieldLabels[match[1]]})
			continue
		}
		last := &segments[len(segments)-1]
		last.text += "\n" + line
	}
	// 模板自己的受保护字段都是单行短值；带续行或超长的"受保护"段其实是长字段内容里恰好以"状态：/链接："等开头的行
	// （比如错误摘要里的一行），不能保护，否则会把后面的整段内容一起锁住。
	// The template's own protected fields are short single-line values; a "protected" segment with continuation lines
	// or an oversized value is really a line inside a long field that happens to start with "状态：" / "链接：" (e.g. a
	// line of the error summary) and must not be protected, or it would lock the rest of that content with it.
	for i := 1; i < len(segments); i++ {
		if segments[i].protected && (strings.Contains(segments[i].text, "\n") || len(segments[i].text) > notificationProtectedFieldMaxBytes) {
			segments[i].protected = false
		}
	}
	return segments
}

func joinNotificationSegments(segments []notificationTextSegment) string {
	parts := make([]string, len(segments))
	for i, segment := range segments {
		parts[i] = segment.text
	}
	return strings.Join(parts, "\n")
}

// shrinkNotificationText 把正文压到 size(text) ≤ limit：先依次截短最长的可裁剪字段（错误摘要、详情等），标题、
// 状态、策略、规则和链接保持原样；仍然放不下时才截断其余内容，并把链接行接回末尾。
// shrinkNotificationText fits the text so size(text) ≤ limit: it first shortens the longest shrinkable fields (error
// summary, details, ...) while the title, status, policy, rule and link stay intact; only when that is not enough does
// it cut the rest and re-append the link line.
func shrinkNotificationText(text string, limit int, size func(string) int) string {
	if size(text) <= limit {
		return text
	}
	segments := splitNotificationText(text)
	for guard := 0; guard < 8*len(segments)+8; guard++ {
		current := joinNotificationSegments(segments)
		excess := size(current) - limit
		if excess <= 0 {
			return current
		}
		longest := -1
		for i, segment := range segments {
			if segment.protected || len(segment.text) <= notificationFieldMinBytes {
				continue
			}
			if longest < 0 || len(segment.text) > len(segments[longest].text) {
				longest = i
			}
		}
		if longest < 0 {
			break
		}
		target := len(segments[longest].text) - rawBytesToDrop(excess, size(current), len(current)) - len(notificationFieldTruncatedSuffix)
		if target < notificationFieldMinBytes {
			target = notificationFieldMinBytes
		}
		segments[longest].text = truncateUTF8Bytes(segments[longest].text, target, notificationFieldTruncatedSuffix)
	}

	// 可裁剪字段都已压到最小仍超限：截断其余内容，把最后一个受保护的"策略"和"链接"行（模板总把它们放在末尾）接回去。
	// Every shrinkable field is at its minimum and it still does not fit: cut the rest and re-append the last protected
	// policy and link lines (the templates always put them last).
	keep := map[int]bool{}
	for _, label := range []string{"策略", "链接"} {
		for i := len(segments) - 1; i > 0; i-- {
			if segments[i].protected && segments[i].label == label {
				keep[i] = true
				break
			}
		}
	}
	rest := make([]notificationTextSegment, 0, len(segments))
	var tail string
	for i, segment := range segments {
		if keep[i] {
			tail += "\n" + segment.text
			continue
		}
		rest = append(rest, segment)
	}
	body := joinNotificationSegments(rest)
	budget := len(body)
	for guard := 0; guard < 16; guard++ {
		candidate := truncateUTF8Bytes(body, budget, notificationTruncatedSuffix) + tail
		candidateSize := size(candidate)
		excess := candidateSize - limit
		if excess <= 0 {
			return candidate
		}
		budget -= rawBytesToDrop(excess, candidateSize, len(candidate))
		if budget <= len(notificationTruncatedSuffix) {
			break
		}
	}
	// 理论上到不了这里；保守地按最坏转义放大（\u00xx 为 6 倍）截断。
	// Should be unreachable; conservatively cut assuming the worst escaping blow-up (\u00xx is 6x).
	return truncateUTF8Bytes(text, limit/6, notificationTruncatedSuffix)
}

// rawBytesToDrop 估算要去掉多少原始字节才能让序列化后的体积少 excess 字节：JSON 转义会放大（\n、\u003c 等），
// 按当前"序列化字节 / 原始字节"的比例折算，至少去掉 1 字节，循环里会再次测量。
// rawBytesToDrop estimates how many raw bytes to remove so the serialized size drops by excess: JSON escaping inflates
// (\n, \u003c, ...), so scale by the current serialized/raw ratio, drop at least one byte, and re-measure in the loop.
func rawBytesToDrop(excess, serializedSize, rawSize int) int {
	if rawSize <= 0 || serializedSize <= rawSize {
		return excess
	}
	ratio := float64(serializedSize) / float64(rawSize)
	drop := int(float64(excess)/ratio) + 1
	if drop < 1 {
		drop = 1
	}
	return drop
}

// fitNotificationPayload 按平台限制裁剪 IM 文本消息：企业微信正文 ≤2048 字节，飞书 / 钉钉请求体 ≤20KB。
// 先截短长字段（错误摘要、详情），标题、状态、策略名称和链接保持完整。
// fitNotificationPayload trims IM text messages to platform limits: WeCom text ≤2048 bytes, Feishu / DingTalk request
// body ≤20KB. Long fields (error summary, details) shrink first; title, status, policy name and link stay intact.
func fitNotificationPayload(channelType NotificationChannelType, payload interface{}) interface{} {
	text, rebuild, ok := imTextPayload(channelType, payload)
	if !ok {
		return payload
	}
	var limit int
	var size func(string) int
	switch channelType {
	case NotificationChannelTypeWeCom:
		limit = weComTextContentMaxBytes
		size = func(value string) int { return len(value) }
	case NotificationChannelTypeFeishu, NotificationChannelTypeDingTalk:
		limit = dingTalkRequestBodyMaxBytes
		if channelType == NotificationChannelTypeFeishu {
			limit = feishuRequestBodyMaxBytes - imSignatureReserveBytes
		}
		size = func(value string) int {
			raw, err := json.Marshal(rebuild(value))
			if err != nil {
				return len(value)
			}
			return len(raw)
		}
	default:
		return payload
	}
	if size(text) <= limit {
		return payload
	}
	return rebuild(shrinkNotificationText(text, limit, size))
}

var (
	notificationURLPattern     = regexp.MustCompile(`https?://[^\s"'<>]+`)
	notificationTokenSegmentRe = regexp.MustCompile(`^[A-Za-z0-9_\-]{20,}$`)
	notificationDigitRe        = regexp.MustCompile(`[0-9]`)
)

// redactEndpointURL 隐藏 URL 里的凭据：所有查询参数值、userinfo 密码、hook 之后的路径段以及形似 token 的长路径段。
// redactEndpointURL hides credentials in a URL: every query value, the userinfo password, the path segment after
// "hook", and long token-like path segments.
func redactEndpointURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		if idx := strings.IndexAny(raw, "?#"); idx >= 0 {
			return raw[:idx] + "?" + notificationSecretMask
		}
		return raw
	}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(parsed.User.Username(), notificationSecretMask)
		}
	}
	segments := strings.Split(parsed.Path, "/")
	maskedPath := false
	for i, segment := range segments {
		if segment == "" {
			continue
		}
		previous := ""
		if i > 0 {
			previous = strings.ToLower(segments[i-1])
		}
		if previous == "hook" || previous == "hooks" ||
			(notificationTokenSegmentRe.MatchString(segment) && notificationDigitRe.MatchString(segment)) {
			segments[i] = notificationSecretMask
			maskedPath = true
		}
	}
	if maskedPath {
		parsed.Path = strings.Join(segments, "/")
		parsed.RawPath = ""
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		keys := make([]string, 0, len(query))
		for key := range query {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, url.QueryEscape(key)+"="+notificationSecretMask)
		}
		parsed.RawQuery = strings.Join(parts, "&")
	}
	// 保留 #fragment（钉钉 errmsg 里的帮助链接靠它定位），但 fragment 里形如 key=value 的值同样脱敏。
	// Keep the #fragment (DingTalk errmsg help links rely on it) but redact key=value pairs inside it as well.
	// fragment 按原文拼回，避免 url.String 把 ] 等字符转义。
	// The fragment is re-attached verbatim so url.String does not escape characters such as ].
	fragment := ""
	if idx := strings.Index(raw, "#"); idx >= 0 {
		fragment = raw[idx+1:]
	}
	parsed.Fragment = ""
	parsed.RawFragment = ""
	result := parsed.String()
	// url.String 会把 userinfo 里的 * 转义，这里换回占位符便于阅读。
	// url.String escapes * inside userinfo; restore the placeholder for readability.
	result = strings.ReplaceAll(result, url.PathEscape(notificationSecretMask), notificationSecretMask)
	if fragment != "" {
		result += "#" + redactURLFragment(fragment)
	}
	return result
}

// redactURLFragment 脱敏 fragment 中 key=value 形式的值和形似 token 的片段，其他内容保持原样。
// redactURLFragment redacts key=value values and token-like parts of a fragment and keeps everything else.
func redactURLFragment(fragment string) string {
	if strings.Contains(fragment, "=") {
		pairs := strings.Split(fragment, "&")
		for i, pair := range pairs {
			if key, _, found := strings.Cut(pair, "="); found {
				pairs[i] = key + "=" + notificationSecretMask
			}
		}
		return strings.Join(pairs, "&")
	}
	if notificationTokenSegmentRe.MatchString(fragment) && notificationDigitRe.MatchString(fragment) {
		return notificationSecretMask
	}
	return fragment
}

// redactURLsInText 把文本里出现的每个 URL 都替换成脱敏后的形式。
// redactURLsInText replaces every URL found in the text with its redacted form.
func redactURLsInText(text string) string {
	if text == "" || !strings.Contains(text, "://") {
		return text
	}
	return notificationURLPattern.ReplaceAllStringFunc(text, redactEndpointURL)
}

// channelSensitiveValues 收集渠道上不能出现在错误信息、日志和投递记录里的值。
// channelSensitiveValues collects the channel values that must never appear in errors, logs or delivery records.
func channelSensitiveValues(channel *NotificationChannel) []string {
	if channel == nil {
		return nil
	}
	values := []string{strings.TrimSpace(channel.Secret)}
	if config := unmarshalNotificationChannelConfig(channel.Type, channel.ConfigJSON); config != nil && config.Email != nil {
		values = append(values, config.Email.Password, strings.TrimSpace(config.Email.Password))
	}
	if parsed, err := url.Parse(strings.TrimSpace(channel.Endpoint)); err == nil {
		if parsed.User != nil {
			if password, ok := parsed.User.Password(); ok {
				values = append(values, password)
			}
		}
		for _, list := range parsed.Query() {
			for _, value := range list {
				values = append(values, value, url.QueryEscape(value))
			}
		}
		segments := strings.Split(parsed.Path, "/")
		for i, segment := range segments {
			if i > 0 && (strings.EqualFold(segments[i-1], "hook") || strings.EqualFold(segments[i-1], "hooks")) {
				values = append(values, segment)
			}
		}
	}
	return values
}

// redactNotificationText 去掉文本中的渠道密钥、token 和 URL 凭据。
// redactNotificationText strips channel secrets, tokens and URL credentials from text.
func redactNotificationText(text string, channel *NotificationChannel) string {
	if text == "" {
		return text
	}
	return redactChannelValues(redactURLsInText(text), channel)
}

// redactChannelValues 只替换渠道自身的密钥、token 等值，不改动文本里的其他 URL（用于请求体 / 响应体留档）。
// redactChannelValues only replaces the channel's own secrets and tokens and leaves other URLs in the text intact
// (used for archived request / response bodies).
func redactChannelValues(text string, channel *NotificationChannel) string {
	if text == "" {
		return text
	}
	values := channelSensitiveValues(channel)
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	for _, value := range values {
		// 太短的值（比如 1、true）替换会误伤正常文字，跳过。
		// Very short values (like 1 or true) would mangle normal text, so skip them.
		if len(value) < 4 || value == notificationSecretMask {
			continue
		}
		text = strings.ReplaceAll(text, value, notificationSecretMask)
	}
	return text
}

// redactedNotificationError 是脱敏后的发送错误，保留原始错误用于 errors.Is / errors.As。
// redactedNotificationError is a redacted send error that keeps the original for errors.Is / errors.As.
type redactedNotificationError struct {
	message string
	cause   error
}

func (e *redactedNotificationError) Error() string { return e.message }
func (e *redactedNotificationError) Unwrap() error { return e.cause }

// redactNotificationError 返回不含渠道凭据的错误。
// redactNotificationError returns an error free of channel credentials.
func redactNotificationError(err error, channel *NotificationChannel) error {
	if err == nil {
		return nil
	}
	var already *redactedNotificationError
	if errors.As(err, &already) && already == err {
		return err
	}
	message := redactNotificationText(err.Error(), channel)
	return &redactedNotificationError{message: message, cause: err}
}

// maskNotificationSecret 有值时返回占位符。
// maskNotificationSecret returns the placeholder when a value is set.
func maskNotificationSecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return notificationSecretMask
}

// isMaskedOrEmptySecret 判断前端传回的是占位符或空值（即"保持不变"）。
// isMaskedOrEmptySecret reports whether the client sent the placeholder or nothing (meaning "keep unchanged").
func isMaskedOrEmptySecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || trimmed == notificationSecretMask
}

// ErrInvalidNotificationChannel 标记通知渠道请求本身不合法（HTTP 400），而不是服务端故障。
// ErrInvalidNotificationChannel marks an invalid notification channel request (HTTP 400) rather than a server failure.
var ErrInvalidNotificationChannel = errors.New("invalid notification channel")

// ErrNotificationChannelNotFound 表示请求的通知渠道不存在（HTTP 404）。
// ErrNotificationChannelNotFound means the requested notification channel does not exist (HTTP 404).
var ErrNotificationChannelNotFound = errors.New("notification channel not found")

// invalidNotificationChannelError 用 ErrInvalidNotificationChannel 包装一条校验错误，错误文本保持原样。
// invalidNotificationChannelError wraps a validation error with ErrInvalidNotificationChannel, keeping its text.
type invalidNotificationChannelError struct{ message string }

func (e *invalidNotificationChannelError) Error() string { return e.message }
func (e *invalidNotificationChannelError) Is(target error) bool {
	return target == ErrInvalidNotificationChannel
}

func newInvalidNotificationChannelError(format string, args ...interface{}) error {
	return &invalidNotificationChannelError{message: fmt.Sprintf(format, args...)}
}

// restoreMaskedNotificationChannelFields 把请求里的占位符 / 空密钥换回已保存的真实值：
//   - secret 为空或为占位符时沿用已有值；但渠道类型变了（比如通用 Webhook 改成飞书）或 clear_secret=true 时清空，
//     避免拿旧渠道的密钥去签新平台的请求；
//   - SMTP 密码为空或为占位符时沿用已有值；
//   - endpoint 与已保存地址的脱敏形式相同时沿用已有地址；仍含占位符但被改过时报错，避免把 ****** 存进去。
//
// restoreMaskedNotificationChannelFields swaps placeholders / empty secrets in the request for the stored values:
//   - an empty or masked secret keeps the stored one, except when the channel type changed (e.g. generic webhook to
//     Feishu) or clear_secret=true, which clear it so an old secret never signs requests for another platform;
//   - an empty or masked SMTP password keeps the stored one;
//   - an endpoint equal to the stored address's redacted form keeps the stored address; one that still holds the
//     placeholder but was edited is rejected so ****** is never saved.
func restoreMaskedNotificationChannelFields(req *UpsertNotificationChannelRequest, existing *NotificationChannel) (*UpsertNotificationChannelRequest, error) {
	if req == nil {
		return nil, nil
	}
	cloned := *req
	if existing == nil {
		if strings.TrimSpace(cloned.Secret) == notificationSecretMask {
			cloned.Secret = ""
		}
		if strings.Contains(cloned.Endpoint, notificationSecretMask) {
			return nil, newInvalidNotificationChannelError("endpoint contains a masked value, please enter the full webhook URL")
		}
		return &cloned, nil
	}

	if cloned.Type != NotificationChannelTypeEmail {
		if isMaskedOrEmptySecret(cloned.Secret) {
			switch {
			case cloned.ClearSecret, cloned.Type != existing.Type:
				cloned.Secret = ""
			default:
				cloned.Secret = existing.Secret
			}
		}
		endpoint := strings.TrimSpace(cloned.Endpoint)
		if endpoint != "" && endpoint == redactEndpointURL(existing.Endpoint) {
			cloned.Endpoint = existing.Endpoint
		} else if strings.Contains(endpoint, notificationSecretMask) {
			return nil, newInvalidNotificationChannelError("endpoint contains a masked value, please enter the full webhook URL")
		}
	}

	if cloned.Config != nil && cloned.Config.Email != nil {
		existingConfig := unmarshalNotificationChannelConfig(existing.Type, existing.ConfigJSON)
		email := *cloned.Config.Email
		if isMaskedOrEmptySecret(email.Password) && strings.TrimSpace(email.Username) != "" &&
			existingConfig != nil && existingConfig.Email != nil {
			email.Password = existingConfig.Email.Password
		}
		if strings.TrimSpace(email.Password) == notificationSecretMask {
			email.Password = ""
		}
		config := *cloned.Config
		config.Email = &email
		cloned.Config = &config
	}
	return &cloned, nil
}
