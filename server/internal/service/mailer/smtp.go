package mailer

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/config"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// SMTP 标准 SMTP 通道。
//
// 两种连接方式，按端口判断，不猜厂商：
//   - 465：连上就是 TLS（隐式加密，QQ/163/腾讯企业邮默认）
//   - 其他：明文连接后 STARTTLS 升级（587 常见）
//
// 之所以不用 smtp.SendMail 一把梭：它在 465 上必然失败
// （那个函数只会明文连接再 STARTTLS），而国内邮箱绝大多数是 465。
type SMTP struct {
	cfg config.MailSMTPConfig
}

// NewSMTP 创建 SMTP 通道。
func NewSMTP(cfg config.MailSMTPConfig) *SMTP { return &SMTP{cfg: cfg} }

// Name 实现 Provider。
func (s *SMTP) Name() string { return "smtp" }

// Send 实现 Provider。
func (s *SMTP) Send(ctx context.Context, msg Message) error {
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprint(s.cfg.Port))
	timeout := s.cfg.Timeout.Std()
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	// ctx 只用于"整个发送过程不被无限拖住"：net/smtp 没有 ctx 支持，
	// 用 Deadline 封住连接与读写，超时报错而不是挂住 goroutine。
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	var (
		client *smtp.Client
		err    error
	)
	if s.cfg.Port == 465 {
		conn, derr := tls.DialWithDialer(
			&net.Dialer{Timeout: timeout, Deadline: deadline},
			"tcp", addr,
			&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12},
		)
		if derr != nil {
			return errcode.ErrMailSendFailed.Wrap(fmt.Errorf("连接 %s 失败: %w", addr, derr))
		}
		_ = conn.SetDeadline(deadline)
		client, err = smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			_ = conn.Close()
			return errcode.ErrMailSendFailed.Wrap(err)
		}
	} else {
		conn, derr := net.DialTimeout("tcp", addr, timeout)
		if derr != nil {
			return errcode.ErrMailSendFailed.Wrap(fmt.Errorf("连接 %s 失败: %w", addr, derr))
		}
		_ = conn.SetDeadline(deadline)
		client, err = smtp.NewClient(conn, s.cfg.Host)
		if err != nil {
			_ = conn.Close()
			return errcode.ErrMailSendFailed.Wrap(err)
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if terr := client.StartTLS(&tls.Config{ServerName: s.cfg.Host, MinVersion: tls.VersionTLS12}); terr != nil {
				_ = client.Close()
				return errcode.ErrMailSendFailed.Wrap(fmt.Errorf("STARTTLS 失败: %w", terr))
			}
		}
	}
	defer func() { _ = client.Close() }()

	// 有些内网中继不需要认证，账号为空时跳过。
	if strings.TrimSpace(s.cfg.Username) != "" {
		auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return errcode.ErrMailSendFailed.Wrap(fmt.Errorf("SMTP 认证失败: %w", err))
			}
		}
	}

	if err := client.Mail(s.cfg.From); err != nil {
		return errcode.ErrMailSendFailed.Wrap(err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return errcode.ErrMailSendFailed.Wrap(err)
	}
	w, err := client.Data()
	if err != nil {
		return errcode.ErrMailSendFailed.Wrap(err)
	}
	if _, err := w.Write(buildMIME(s.cfg.From, s.cfg.FromName, msg)); err != nil {
		_ = w.Close()
		return errcode.ErrMailSendFailed.Wrap(err)
	}
	if err := w.Close(); err != nil {
		return errcode.ErrMailSendFailed.Wrap(err)
	}
	// Quit 顺手把未消费的服务端响应读干净；它失败不影响"信已投递"这个事实。
	_ = client.Quit()
	return nil
}

// buildMIME 拼装一封 multipart/alternative 邮件。
//
// 头字段全部用 \r\n：SMTP 规范要求，用 \n 时部分服务端会直接 554。
func buildMIME(from, fromName string, msg Message) []byte {
	const boundary = "kms-alt-boundary-7f3a"

	var b strings.Builder
	writeHeader := func(k, v string) {
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\r\n")
	}

	writeHeader("From", formatAddress(from, fromName))
	writeHeader("To", msg.To)
	// 中文主题必须 RFC 2047 编码，否则收件端是乱码。
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", msg.Subject))
	writeHeader("Date", time.Now().Format(time.RFC1123Z))
	writeHeader("MIME-Version", "1.0")
	writeHeader("Content-Type", "multipart/alternative; boundary=\""+boundary+"\"")
	b.WriteString("\r\n")

	part := func(contentType, body string) {
		b.WriteString("--" + boundary + "\r\n")
		b.WriteString("Content-Type: " + contentType + "; charset=UTF-8\r\n")
		b.WriteString("Content-Transfer-Encoding: base64\r\n\r\n")
		b.WriteString(base64Lines(body))
		b.WriteString("\r\n")
	}

	text := msg.Text
	if text == "" {
		text = stripHTML(msg.HTML)
	}
	part("text/plain", text)
	if msg.HTML != "" {
		part("text/html", msg.HTML)
	}
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

// formatAddress 生成 From 头，昵称含中文时同样要编码。
func formatAddress(addr, name string) string {
	if strings.TrimSpace(name) == "" {
		return addr
	}
	return mime.QEncoding.Encode("utf-8", name) + " <" + addr + ">"
}

// base64Lines 按 76 字符折行，这是 MIME 对 base64 的硬要求。
func base64Lines(s string) string {
	enc := base64.StdEncoding.EncodeToString([]byte(s))
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteString("\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc)
	return b.String()
}

// stripHTML 极简去标签，只作为纯文本兜底，不追求语义正确。
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return strings.TrimSpace(b.String())
}
