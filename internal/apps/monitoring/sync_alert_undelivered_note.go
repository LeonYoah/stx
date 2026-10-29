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
	"html"
	"strings"
)

// syncAlertUndeliveredFirstAlertNote 加在"首次告警没送达就已恢复"的恢复通知标题下面。
// syncAlertUndeliveredFirstAlertNote goes right under the title of a recovery whose first alert was never delivered.
const syncAlertUndeliveredFirstAlertNote = "说明：此前的告警通知未能送达（渠道当时不可用），本条同时告知该告警曾经发生、现已恢复。"

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
			if body.HTML != "" {
				body.HTML = "<p><strong>" + html.EscapeString(syncAlertUndeliveredFirstAlertNote) + "</strong></p>" + body.HTML
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
