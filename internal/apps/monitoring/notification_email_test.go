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
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
)

type fakeSMTPServer struct {
	listener   net.Listener
	wg         sync.WaitGroup
	mu         sync.Mutex
	mailFrom   string
	recipients []string
	data       string
}

func newFakeSMTPServer(t *testing.T) *fakeSMTPServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp server: %v", err)
	}
	server := &fakeSMTPServer{listener: listener}
	server.wg.Add(1)
	go server.serve(t)
	return server
}

func (s *fakeSMTPServer) addr() string {
	return s.listener.Addr().String()
}

func (s *fakeSMTPServer) close() {
	_ = s.listener.Close()
	s.wg.Wait()
}

func (s *fakeSMTPServer) serve(t *testing.T) {
	defer s.wg.Done()
	conn, err := s.listener.Accept()
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeLine := func(line string) {
		_, _ = writer.WriteString(line + "\r\n")
		_ = writer.Flush()
	}
	writeLine("220 fake-smtp ready")

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "EHLO") || strings.HasPrefix(trimmed, "HELO"):
			_, _ = writer.WriteString("250-fake-smtp\r\n")
			_, _ = writer.WriteString("250 OK\r\n")
			_ = writer.Flush()
		case strings.HasPrefix(trimmed, "MAIL FROM:"):
			s.mu.Lock()
			s.mailFrom = strings.TrimPrefix(trimmed, "MAIL FROM:")
			s.mu.Unlock()
			writeLine("250 OK")
		case strings.HasPrefix(trimmed, "RCPT TO:"):
			s.mu.Lock()
			s.recipients = append(s.recipients, strings.TrimPrefix(trimmed, "RCPT TO:"))
			s.mu.Unlock()
			writeLine("250 OK")
		case trimmed == "DATA":
			writeLine("354 End data with <CR><LF>.<CR><LF>")
			var dataLines []string
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
				dataLines = append(dataLines, dataLine)
			}
			s.mu.Lock()
			s.data = strings.Join(dataLines, "")
			s.mu.Unlock()
			writeLine("250 Accepted")
		case trimmed == "QUIT":
			writeLine("221 Bye")
			return
		default:
			t.Logf("unhandled smtp line: %s", trimmed)
			writeLine("250 OK")
		}
	}
}

func TestSendEmailNotification_SMTPPlain(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()

	host, portRaw, err := net.SplitHostPort(server.addr())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portRaw, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	configJSON, err := marshalNotificationChannelConfig(NotificationChannelTypeEmail, &NotificationChannelConfig{
		Email: &NotificationChannelEmailConfig{
			Protocol:   "smtp",
			Security:   NotificationEmailSecurityNone,
			Host:       host,
			Port:       port,
			From:       "alerts@example.com",
			FromName:   "STX Alerts",
			Recipients: []string{"ops@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("marshal channel config: %v", err)
	}

	attempt, err := sendEmailNotification(context.Background(), &NotificationChannel{
		ID:         100,
		Name:       "email-demo",
		Type:       NotificationChannelTypeEmail,
		ConfigJSON: configJSON,
	}, &emailNotificationPayload{
		Subject: "STX restart alert",
		Text:    "cluster restart requested",
	})
	if err != nil {
		t.Fatalf("send email notification: %v", err)
	}
	if attempt == nil || attempt.StatusCode != 250 {
		t.Fatalf("unexpected attempt: %+v", attempt)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if !strings.Contains(server.mailFrom, "alerts@example.com") {
		t.Fatalf("unexpected mail from: %s", server.mailFrom)
	}
	if len(server.recipients) != 1 || !strings.Contains(server.recipients[0], "ops@example.com") {
		t.Fatalf("unexpected recipients: %+v", server.recipients)
	}
	if !strings.Contains(server.data, "Subject: ") || !strings.Contains(server.data, "STX restart alert") {
		t.Fatalf("unexpected message subject: %s", server.data)
	}
	if !strings.Contains(server.data, "STX Alerts") {
		t.Fatalf("unexpected from header: %s", server.data)
	}
	if !strings.Contains(server.data, "cluster restart requested") {
		t.Fatalf("unexpected message body: %s", server.data)
	}
}

func TestBuildSMTPMessage_includesHTMLAlternative(t *testing.T) {
	config := &NotificationChannelEmailConfig{
		Protocol: "smtp",
		Security: NotificationEmailSecurityNone,
		Host:     "smtp.example.com",
		Port:     25,
		From:     "alerts@example.com",
		FromName: "STX Alerts",
	}

	message, err := buildSMTPMessage(config, &emailNotificationPayload{
		Subject: "STX node alert",
		Text:    "plain text body",
		HTML:    "<html><body><strong>formatted body</strong></body></html>",
		To:      []string{"ops@example.com"},
	})
	if err != nil {
		t.Fatalf("build smtp message: %v", err)
	}

	content := string(message)
	if !strings.Contains(content, "multipart/alternative") {
		t.Fatalf("expected multipart/alternative content type, got %s", content)
	}
	if !strings.Contains(content, "Content-Type: text/html; charset=UTF-8") {
		t.Fatalf("expected html body part, got %s", content)
	}
	if !strings.Contains(content, "<strong>formatted body</strong>") {
		t.Fatalf("expected html content in message, got %s", content)
	}
}

func TestSMTPConnection_SMTPPlain(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()

	host, portRaw, err := net.SplitHostPort(server.addr())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portRaw, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	if err := testSMTPConnection(context.Background(), &NotificationChannelEmailConfig{
		Protocol: "smtp",
		Security: NotificationEmailSecurityNone,
		Host:     host,
		Port:     port,
		From:     "alerts@example.com",
	}); err != nil {
		t.Fatalf("test smtp connection: %v", err)
	}
}

func TestService_TestNotificationChannelDraft_SMTPPlain(t *testing.T) {
	server := newFakeSMTPServer(t)
	defer server.close()

	host, portRaw, err := net.SplitHostPort(server.addr())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	var port int
	if _, err := fmt.Sscanf(portRaw, "%d", &port); err != nil {
		t.Fatalf("parse port: %v", err)
	}

	database, cleanup := setupMonitoringNotificationTestDB(t)
	defer cleanup()
	if err := database.AutoMigrate(&auth.User{}); err != nil {
		t.Fatalf("auto migrate auth user: %v", err)
	}

	repo := NewRepository(database)
	service := NewService(nil, nil, repo)
	ctx := context.Background()

	user := &auth.User{
		Username:  "alice",
		Nickname:  "Alice",
		Email:     "alice@example.com",
		IsActive:  true,
		IsAdmin:   false,
		AvatarURL: "",
	}
	if err := user.Create(database); err != nil {
		t.Fatalf("create user: %v", err)
	}

	enabled := true
	result, err := service.TestNotificationChannelDraft(ctx, &NotificationChannelDraftTestRequest{
		Channel: &UpsertNotificationChannelRequest{
			Name:    "draft-email",
			Type:    NotificationChannelTypeEmail,
			Enabled: &enabled,
			Config: &NotificationChannelConfig{
				Email: &NotificationChannelEmailConfig{
					Protocol: "smtp",
					Security: NotificationEmailSecurityNone,
					Host:     host,
					Port:     port,
					From:     "alerts@example.com",
				},
			},
		},
		ReceiverUserID: user.ID,
	})
	if err != nil {
		t.Fatalf("TestNotificationChannelDraft returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ChannelID != 0 {
		t.Fatalf("expected draft channel id 0, got %d", result.ChannelID)
	}
	if result.Status != string(NotificationDeliveryStatusSent) {
		t.Fatalf("expected sent status, got %s", result.Status)
	}
	if result.Receiver != user.Email {
		t.Fatalf("expected receiver %s, got %s", user.Email, result.Receiver)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if len(server.recipients) != 1 || !strings.Contains(server.recipients[0], user.Email) {
		t.Fatalf("unexpected recipients: %+v", server.recipients)
	}
	if !strings.Contains(server.data, "STX notification test") {
		t.Fatalf("unexpected message data: %s", server.data)
	}
}
