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
	"strings"
)

// syncAlertUndeliveredFirstAlertNoteLabel 是说明行的标签，列在 notificationProtectedFieldLabels 里，IM 裁剪不会截掉这一行。
// 不用通用的"说明"：远程告警的长描述字段也叫"说明"，那个字段必须可裁剪。
// syncAlertUndeliveredFirstAlertNoteLabel labels the note line; it is in notificationProtectedFieldLabels so IM
// fitting never cuts the line. It is not the generic "说明", which also labels the (shrinkable) remote alert description.
const syncAlertUndeliveredFirstAlertNoteLabel = "送达说明"

// syncAlertUndeliveredFirstAlertNote 加在"首次告警没送达就已恢复"的恢复通知标题下面。
// syncAlertUndeliveredFirstAlertNote goes right under the title of a recovery whose first alert was never delivered.
const syncAlertUndeliveredFirstAlertNote = syncAlertUndeliveredFirstAlertNoteLabel + "：此前的告警通知未能送达（渠道当时不可用），本条同时告知该告警曾经发生、现已恢复。"

// withUndeliveredFirstAlertNote 返回一份恢复消息的副本，在各渠道正文里（标题之后）加上首次告警未送达的说明，webhook
// 结构化字段里加 first_alert_delivered=false。不认识的消息体原样返回。
// withUndeliveredFirstAlertNote returns a copy of the recovery message whose body (right after the title) notes that
// the first alert was never delivered, and whose webhook payload carries first_alert_delivered=false. Unknown bodies are
// returned unchanged.
func withUndeliveredFirstAlertNote(message *alertMessage) *alertMessage {
	if message == nil || message.Build == nil {
		return message
	}
	build := message.Build
	copied := *message
	copied.Build = func(channel *NotificationChannel, recipients []string) (interface{}, error) {
		payload, err := build(channel, recipients)
		if err != nil {
			return payload, err
		}
		switch body := payload.(type) {
		case map[string]interface{}:
			if text, ok := body["message"].(string); ok {
				body["message"] = insertNoteAfterTitle(text)
			}
			if alert, ok := body["alert"].(map[string]interface{}); ok {
				alert["first_alert_delivered"] = false
			}
			if text, ok := body["text"].(map[string]string); ok {
				text["content"] = insertNoteAfterTitle(text["content"])
			}
			if content, ok := body["content"].(map[string]string); ok {
				content["text"] = insertNoteAfterTitle(content["text"])
			}
		case *emailNotificationPayload:
			body.Text = insertNoteAfterTitle(body.Text)
			// HTML 正文按带说明的版本重新渲染，说明块在卡片内，不能拼在 <!DOCTYPE html> 前面（会进怪异模式、跑到卡片外）。
			// Re-render the HTML with the note inside the card; prepending it before <!DOCTYPE html> would trigger
			// quirks mode and leave it outside the card.
			if body.HTML != "" && message.RenderEmailHTML != nil {
				body.HTML = message.RenderEmailHTML(syncAlertUndeliveredFirstAlertNote)
			}
		}
		return payload, nil
	}
	return &copied
}

func insertNoteAfterTitle(text string) string {
	title, rest, found := strings.Cut(text, "\n")
	if !found {
		return text + "\n" + syncAlertUndeliveredFirstAlertNote
	}
	return title + "\n" + syncAlertUndeliveredFirstAlertNote + "\n" + rest
}
