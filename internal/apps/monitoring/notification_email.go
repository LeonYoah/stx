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
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// emailNotificationPayload represents one normalized email message.
// emailNotificationPayload 表示一份规范化邮件消息。
type emailNotificationPayload struct {
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html,omitempty"`
	To      []string `json:"to,omitempty"`
}

func sendEmailNotification(ctx context.Context, channel *NotificationChannel, payload interface{}) (*notificationSendAttempt, error) {
	if channel == nil {
		return nil, fmt.Errorf("notification channel not found")
	}
	config := unmarshalNotificationChannelConfig(channel.Type, channel.ConfigJSON)
	if config == nil || config.Email == nil {
		return nil, fmt.Errorf("email config is not configured")
	}
	emailConfig := config.Email.Normalize()
	if err := emailConfig.Validate(); err != nil {
		return nil, err
	}

	emailPayload, err := normalizeEmailNotificationPayload(payload, emailConfig.Recipients)
	if err != nil {
		return nil, err
	}
	messageBytes, err := buildSMTPMessage(emailConfig, emailPayload)
	if err != nil {
		return nil, err
	}

	requestPayloadBytes, _ := json.Marshal(map[string]interface{}{
		"subject":    emailPayload.Subject,
		"to":         emailPayload.To,
		"from":       emailConfig.From,
		"smtp_host":  emailConfig.Host,
		"smtp_port":  emailConfig.Port,
		"smtp_mode":  emailConfig.Security,
		"channel_id": channel.ID,
	})

	if err := deliverSMTPMessage(ctx, emailConfig, emailPayload.To, messageBytes); err != nil {
		return &notificationSendAttempt{RequestPayload: string(requestPayloadBytes)}, err
	}

	now := timex.Now()
	return &notificationSendAttempt{
		RequestPayload: string(requestPayloadBytes),
		StatusCode:     250,
		ResponseBody:   "smtp accepted",
		SentAt:         &now,
	}, nil
}

func normalizeEmailNotificationPayload(payload interface{}, defaultRecipients []string) (*emailNotificationPayload, error) {
	switch typed := payload.(type) {
	case *emailNotificationPayload:
		if typed == nil {
			return nil, fmt.Errorf("email payload is required")
		}
		cloned := *typed
		cloned.Subject = strings.TrimSpace(cloned.Subject)
		cloned.Text = strings.TrimSpace(cloned.Text)
		cloned.HTML = strings.TrimSpace(cloned.HTML)
		cloned.To = normalizeEmailRecipients(cloned.To)
		if len(cloned.To) == 0 {
			cloned.To = normalizeEmailRecipients(defaultRecipients)
		}
		if cloned.Subject == "" {
			return nil, fmt.Errorf("email subject is required")
		}
		if cloned.Text == "" && cloned.HTML == "" {
			return nil, fmt.Errorf("email text or html is required")
		}
		if len(cloned.To) == 0 {
			return nil, fmt.Errorf("email recipients are required")
		}
		return &cloned, nil
	case emailNotificationPayload:
		return normalizeEmailNotificationPayload(&typed, defaultRecipients)
	default:
		return nil, fmt.Errorf("unsupported email payload")
	}
}

func buildSMTPMessage(config *NotificationChannelEmailConfig, payload *emailNotificationPayload) ([]byte, error) {
	if config == nil || payload == nil {
		return nil, fmt.Errorf("email config and payload are required")
	}

	textBody, err := encodeQuotedPrintableBody(payload.Text)
	if err != nil {
		return nil, err
	}

	fromHeader := strings.TrimSpace(config.From)
	if strings.TrimSpace(config.FromName) != "" {
		fromHeader = (&mail.Address{
			Name:    strings.TrimSpace(config.FromName),
			Address: strings.TrimSpace(config.From),
		}).String()
	}

	subject := mime.QEncoding.Encode("utf-8", strings.TrimSpace(payload.Subject))
	now := timex.Now()
	headers := []string{
		fmt.Sprintf("From: %s", fromHeader),
		fmt.Sprintf("To: %s", strings.Join(payload.To, ", ")),
		fmt.Sprintf("Subject: %s", subject),
		fmt.Sprintf("Date: %s", now.Format(time.RFC1123Z)),
		fmt.Sprintf("Message-ID: %s", buildSMTPMessageID(config.From, now)),
		"MIME-Version: 1.0",
	}
	if strings.TrimSpace(payload.HTML) == "" {
		headers = append(headers,
			"Content-Type: text/plain; charset=UTF-8",
			"Content-Transfer-Encoding: quoted-printable",
		)
		message := strings.Join(headers, "\r\n") + "\r\n\r\n" + textBody
		return []byte(message), nil
	}

	htmlBody, err := encodeQuotedPrintableBody(payload.HTML)
	if err != nil {
		return nil, err
	}
	boundary := fmt.Sprintf("=_STX_%d", now.UnixNano())
	headers = append(headers, fmt.Sprintf("Content-Type: multipart/alternative; boundary=%q", boundary))

	var message strings.Builder
	message.WriteString(strings.Join(headers, "\r\n"))
	message.WriteString("\r\n\r\n")
	message.WriteString("--" + boundary + "\r\n")
	message.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	message.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	message.WriteString(textBody)
	message.WriteString("\r\n--" + boundary + "\r\n")
	message.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	message.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
	message.WriteString(htmlBody)
	message.WriteString("\r\n--" + boundary + "--\r\n")
	return []byte(message.String()), nil
}

// buildSMTPMessageID 生成 RFC 5322 Message-ID：<时间戳.随机数@发件域>，发件域取不到时用本机主机名。
// buildSMTPMessageID builds an RFC 5322 Message-ID: <timestamp.random@sender-domain>, using the hostname when the sender
// has no domain.
func buildSMTPMessageID(from string, now time.Time) string {
	domain := ""
	if address, err := mail.ParseAddress(strings.TrimSpace(from)); err == nil {
		if at := strings.LastIndex(address.Address, "@"); at >= 0 {
			domain = address.Address[at+1:]
		}
	}
	if domain == "" {
		domain = smtpHelloName()
	}
	random := make([]byte, 8)
	_, _ = rand.Read(random)
	return fmt.Sprintf("<%d.%s@%s>", now.UnixNano(), hex.EncodeToString(random), domain)
}

func encodeQuotedPrintableBody(raw string) (string, error) {
	var body bytes.Buffer
	qp := quotedprintable.NewWriter(&body)
	if _, err := qp.Write([]byte(raw)); err != nil {
		return "", err
	}
	if err := qp.Close(); err != nil {
		return "", err
	}
	return body.String(), nil
}

func testSMTPConnection(ctx context.Context, config *NotificationChannelEmailConfig) error {
	if config == nil {
		return fmt.Errorf("email config is required")
	}
	client, conn, err := openSMTPClient(ctx, config)
	if err != nil {
		return err
	}
	defer closeSMTPClient(client, conn)
	return authenticateSMTPClient(client, config)
}

func deliverSMTPMessage(ctx context.Context, config *NotificationChannelEmailConfig, recipients []string, message []byte) error {
	if config == nil {
		return fmt.Errorf("email config is required")
	}
	if len(recipients) == 0 {
		return fmt.Errorf("email recipients are required")
	}
	client, conn, err := openSMTPClient(ctx, config)
	if err != nil {
		return err
	}
	defer closeSMTPClient(client, conn)

	if err := authenticateSMTPClient(client, config); err != nil {
		return err
	}
	if err := client.Mail(strings.TrimSpace(config.From)); err != nil {
		return err
	}
	for _, recipient := range recipients {
		if err := client.Rcpt(strings.TrimSpace(recipient)); err != nil {
			return err
		}
	}

	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	return nil
}

// SMTP 超时：连接超时 + 整个会话的读写截止时间。都是变量，便于测试缩短。
// SMTP timeouts: the connect timeout and the read/write deadline of the whole session; variables so tests can shorten them.
var (
	smtpDialTimeout = 10 * time.Second
	smtpIOTimeout   = 30 * time.Second
)

// smtpHelloName 返回 HELO/EHLO 用的本机主机名；取不到或不合法时退回 localhost。
// smtpHelloName returns the local hostname for HELO/EHLO, falling back to localhost when unavailable or invalid.
func smtpHelloName() string {
	name, err := os.Hostname()
	name = strings.TrimSpace(name)
	if err != nil || name == "" || strings.ContainsAny(name, " \t\r\n") {
		return "localhost"
	}
	return name
}

// smtpSessionDeadline 返回会话截止时间：IO 超时与 ctx 截止时间取较早者。
// smtpSessionDeadline returns the session deadline: the earlier of the IO timeout and the ctx deadline.
func smtpSessionDeadline(ctx context.Context) time.Time {
	deadline := time.Now().Add(smtpDialTimeout + smtpIOTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	return deadline
}

// openSMTPClient 建立 SMTP 会话。连上之后立刻设置读写截止时间，再读服务器 greeting，
// 这样只接 TCP 不说话的服务器也会在超时后返回，不会卡住告警队列。
// openSMTPClient opens an SMTP session. The read/write deadline is set right after connecting and before reading the
// server greeting, so a server that accepts TCP but never speaks still returns after the timeout instead of stalling
// the alert queue.
func openSMTPClient(ctx context.Context, config *NotificationChannelEmailConfig) (*smtp.Client, net.Conn, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	dialer := &net.Dialer{Timeout: smtpDialTimeout}
	host := strings.TrimSpace(config.Host)

	var (
		conn net.Conn
		err  error
	)
	if config.Security == NotificationEmailSecuritySSL {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}}
		conn, err = tlsDialer.DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, nil, err
	}
	_ = conn.SetDeadline(smtpSessionDeadline(ctx))

	// ctx 取消时立刻关闭连接，打断正在阻塞的读写。
	// Close the connection as soon as ctx is canceled to interrupt a blocked read or write.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	fail := func(cause error) (*smtp.Client, net.Conn, error) {
		stop()
		_ = conn.Close()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, nil, fmt.Errorf("smtp %s: %w", ctxErr, cause)
		}
		return nil, nil, cause
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fail(err)
	}
	if err := client.Hello(smtpHelloName()); err != nil {
		return fail(err)
	}
	if config.Security == NotificationEmailSecurityStartTLS {
		ok, _ := client.Extension("STARTTLS")
		if !ok {
			return fail(fmt.Errorf("smtp server does not support STARTTLS"))
		}
		if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return fail(err)
		}
	}
	return client, &smtpConn{Conn: conn, stop: stop}, nil
}

// smtpConn 在关闭时同时解除 ctx 监听。
// smtpConn also detaches the ctx watcher when closed.
type smtpConn struct {
	net.Conn
	stop func() bool
}

func (c *smtpConn) Close() error {
	if c.stop != nil {
		c.stop()
	}
	return c.Conn.Close()
}

func authenticateSMTPClient(client *smtp.Client, config *NotificationChannelEmailConfig) error {
	if client == nil || config == nil || strings.TrimSpace(config.Username) == "" {
		return nil
	}
	ok, _ := client.Extension("AUTH")
	if !ok {
		return fmt.Errorf("smtp server does not support AUTH")
	}
	auth := smtp.PlainAuth("", strings.TrimSpace(config.Username), config.Password, strings.TrimSpace(config.Host))
	return client.Auth(auth)
}

func closeSMTPClient(client *smtp.Client, conn net.Conn) {
	if client != nil {
		_ = client.Quit()
		_ = client.Close()
	}
	if conn != nil {
		_ = conn.Close()
	}
}
