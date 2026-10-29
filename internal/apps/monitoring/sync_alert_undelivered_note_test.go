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
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

const alertCardOpenTag = `<div style="max-width:760px;margin:0 auto;background:#ffffff;border:1px solid #e2e8f0;border-radius:16px;overflow:hidden;">`

// assertNoteInsideCard 断言 HTML 以 <!DOCTYPE 开头，且转义后的说明出现在卡片元素内（卡片开标签之后、卡片闭合之前）。
// assertNoteInsideCard asserts the HTML starts with <!DOCTYPE and the escaped note sits inside the card element (after
// the card's opening tag and before it closes).
func assertNoteInsideCard(t *testing.T, body string, note string) {
	t.Helper()
	if !strings.HasPrefix(body, "<!DOCTYPE") {
		t.Fatalf("email HTML must start with <!DOCTYPE (no quirks mode), got prefix %q", body[:min(len(body), 80)])
	}
	escaped := html.EscapeString(note)
	cardStart := strings.Index(body, alertCardOpenTag)
	noteAt := strings.Index(body, escaped)
	cardEnd := strings.LastIndex(body, "</div></div></body></html>")
	if cardStart < 0 || noteAt < 0 || cardEnd < 0 || !(cardStart < noteAt && noteAt < cardEnd) {
		t.Fatalf("note must be inside the card (card=%d note=%d end=%d):\n%s", cardStart, noteAt, cardEnd, body)
	}
	if strings.Count(body, escaped) != 1 {
		t.Fatalf("note must appear exactly once:\n%s", body)
	}
	block := body[strings.LastIndex(body[:noteAt], "<div"):noteAt]
	if !strings.Contains(block, `data-stx-alert-note="true"`) || !strings.Contains(block, "background:#fef9c3") {
		t.Fatalf("note must render as the yellow notice block, got %q", block)
	}
}

// #85 复审：说明块渲染在卡片内并做 HTML 转义；不带说明时不渲染提示块。
// #85 review: the note renders inside the card, HTML-escaped; without a note no notice block is rendered.
func TestRenderAlertMessageHTML_NoteInsideCard(t *testing.T) {
	fields := []localAlertMessageField{{Label: "状态", Value: "已恢复"}, {Label: "任务", Value: "orders"}}
	visual := resolveLocalAlertMessageVisual(NotificationDeliveryEventTypeResolved)
	note := `说明：<script>alert("x")</script> & 渠道不可用`
	body := renderAlertMessageHTML("[STX] 已恢复", "CRITICAL", visual, fields, note)
	assertNoteInsideCard(t, body, note)
	if strings.Contains(body, "<script>") {
		t.Fatalf("note must be HTML-escaped:\n%s", body)
	}
	plain := renderAlertMessageHTML("[STX] 已恢复", "CRITICAL", visual, fields, "")
	if !strings.HasPrefix(plain, "<!DOCTYPE") || strings.Contains(plain, "data-stx-alert-note") {
		t.Fatalf("without a note there must be no notice block:\n%s", plain)
	}
}

// #85 复审：首次告警未送达的恢复邮件，说明在卡片内，文档仍以 <!DOCTYPE 开头；纯文本正文说明紧跟标题。
// #85 review: the recovery email for an undelivered first alert keeps <!DOCTYPE first and the note inside the card;
// the plain text has the note right after the title.
func TestWithUndeliveredFirstAlertNote_EmailNoteInsideCard(t *testing.T) {
	episode := &SyncAlertEpisode{ID: 1, Scope: SyncAlertEpisodeScopeTask, TaskID: 7, TaskName: "orders", RuleKey: syncalert.RuleJobFailed, Severity: AlertSeverityCritical, OccurrenceCount: 1}
	message := withUndeliveredFirstAlertNote(buildSyncAlertMessage(episode, &AlertPolicy{Name: "p"}, NotificationDeliveryEventTypeResolved, nil, time.Now(), false))
	payload, err := message.Build(&NotificationChannel{Type: NotificationChannelTypeEmail}, []string{"ops@example.com"})
	if err != nil {
		t.Fatalf("build email: %v", err)
	}
	mail := payload.(*emailNotificationPayload)
	assertNoteInsideCard(t, mail.HTML, syncAlertUndeliveredFirstAlertNote)
	if lines := strings.Split(mail.Text, "\n"); len(lines) < 2 || lines[1] != syncAlertUndeliveredFirstAlertNote {
		t.Fatalf("plain text must carry the note right after the title:\n%s", mail.Text)
	}
}

// #85 复审：说明行的标签受保护，IM 裁剪不会截掉说明行（即使其余字段都要压到最短）。
// #85 review: the note line's label is protected, so IM fitting never cuts it (even when every other field must be
// shrunk to its minimum).
func TestShrinkNotificationText_KeepsUndeliveredFirstAlertNote(t *testing.T) {
	if !notificationProtectedFieldLabels[syncAlertUndeliveredFirstAlertNoteLabel] {
		t.Fatalf("note label %q must be protected", syncAlertUndeliveredFirstAlertNoteLabel)
	}
	if !strings.HasPrefix(syncAlertUndeliveredFirstAlertNote, syncAlertUndeliveredFirstAlertNoteLabel+"：") {
		t.Fatalf("note must start with its label: %q", syncAlertUndeliveredFirstAlertNote)
	}

	// 长错误摘要 / 详情（复用 D 的夹具）。
	// Long error summary / details (reusing fixture D).
	long, kept := buildLongSyncAlertText(5000)
	// 许多比说明行短的可裁剪字段：说明行是最长的段，未受保护时会第一个被截断。
	// Many shrinkable fields shorter than the note: the note is the longest segment, so unprotected it would be cut first.
	medium := []string{"[STX][恢复] 任务 orders · 作业运行失败"}
	for i := 0; i < 40; i++ {
		medium = append(medium, fmt.Sprintf("字段%d：%s", i, strings.Repeat("值", 25)))
	}
	medium = append(medium, "链接：http://stx-console.example:17880/monitoring?tab=alerts")

	for name, text := range map[string]string{"long-summary": long, "many-fields": strings.Join(medium, "\n")} {
		noted := insertNoteAfterTitle(text)
		for _, tc := range []struct {
			channelType NotificationChannelType
			limit       int
		}{
			{NotificationChannelTypeWeCom, weComTextContentMaxBytes},
			{NotificationChannelTypeFeishu, feishuRequestBodyMaxBytes - imSignatureReserveBytes},
			{NotificationChannelTypeDingTalk, dingTalkRequestBodyMaxBytes},
		} {
			t.Run(fmt.Sprintf("%s-%s", name, tc.channelType), func(t *testing.T) {
				fitted := fitNotificationPayload(tc.channelType, textPayload(tc.channelType, noted))
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
				if got == noted && tc.channelType == NotificationChannelTypeWeCom {
					t.Fatalf("fixture must actually need truncation for WeCom")
				}
				if lines := strings.Split(got, "\n"); len(lines) < 2 || lines[1] != syncAlertUndeliveredFirstAlertNote {
					t.Fatalf("note line must survive fitting verbatim right after the title:\n%s", got)
				}
				if name == "long-summary" {
					for _, line := range kept {
						if !strings.Contains(got, line) {
							t.Fatalf("fitted text lost %q", line)
						}
					}
				}
			})
		}
	}
}
