package delivery

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"

	"github.com/richrobertson/notification-platform/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type SMTPSender struct {
	cfg  config.Config
	dial func(network, addr string) (net.Conn, error)
}

type EmailRequest struct {
	To             string
	Subject        string
	Body           string
	AttemptID      string
	NotificationID string
}

func NewSMTPSender(cfg config.Config) *SMTPSender {
	return &SMTPSender{cfg: cfg, dial: net.Dial}
}

func NewSecondarySMTPSender(cfg config.Config) *SMTPSender {
	if strings.TrimSpace(cfg.SecondarySMTPHost) == "" || cfg.SecondarySMTPPort <= 0 {
		return nil
	}
	secondary := cfg
	secondary.SMTPHost = cfg.SecondarySMTPHost
	secondary.SMTPPort = cfg.SecondarySMTPPort
	secondary.SMTPUsername = cfg.SecondarySMTPUsername
	secondary.SMTPPassword = cfg.SecondarySMTPPassword
	if strings.TrimSpace(cfg.SecondarySMTPFrom) != "" {
		secondary.SMTPFrom = cfg.SecondarySMTPFrom
	}
	secondary.SMTPUseTLS = cfg.SecondarySMTPUseTLS
	secondary.SMTPStartTLS = cfg.SecondarySMTPStartTLS
	secondary.SMTPInsecureSkipVerify = cfg.SecondarySMTPInsecureSkipVerify
	return &SMTPSender{cfg: secondary, dial: net.Dial}
}

func (s *SMTPSender) Send(ctx context.Context, req EmailRequest) error {
	_, span := otel.Tracer("notification-platform/delivery").Start(ctx, "email.send")
	defer span.End()
	span.SetAttributes(attribute.String("delivery.channel", "email"), attribute.String("email.to", req.To))

	addr := fmt.Sprintf("%s:%d", s.cfg.SMTPHost, s.cfg.SMTPPort)
	conn, err := s.dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("dial smtp server: %w", err)
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if s.cfg.SMTPUseTLS {
		conn = tls.Client(conn, &tls.Config{ServerName: s.cfg.SMTPHost, InsecureSkipVerify: s.cfg.SMTPInsecureSkipVerify})
	}

	client, err := smtp.NewClient(conn, s.cfg.SMTPHost)
	if err != nil {
		return fmt.Errorf("create smtp client: %w", err)
	}
	defer client.Close()

	if s.cfg.SMTPStartTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return fmt.Errorf("smtp server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: s.cfg.SMTPHost, InsecureSkipVerify: s.cfg.SMTPInsecureSkipVerify}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	if s.cfg.SMTPUsername != "" {
		auth := smtp.PlainAuth("", s.cfg.SMTPUsername, s.cfg.SMTPPassword, s.cfg.SMTPHost)
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("smtp auth: %w", err)
			}
		}
	}
	if err := client.Mail(s.cfg.SMTPFrom); err != nil {
		return fmt.Errorf("set smtp from: %w", err)
	}
	if err := client.Rcpt(req.To); err != nil {
		return fmt.Errorf("set smtp recipient: %w", err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("start smtp data: %w", err)
	}
	message := buildEmailMessage(s.cfg.SMTPFrom, req)
	if _, err := wc.Write([]byte(message)); err != nil {
		_ = wc.Close()
		return fmt.Errorf("write smtp message: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("finish smtp message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("smtp quit: %w", err)
	}
	return nil
}

func buildEmailMessage(from string, req EmailRequest) string {
	body := strings.ReplaceAll(req.Body, "\n", "\r\n")
	headers := []string{
		fmt.Sprintf("To: %s", req.To),
		fmt.Sprintf("From: %s", from),
		fmt.Sprintf("Subject: %s", req.Subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
	}
	if id := sanitizeIdentifier(req.AttemptID); id != "" {
		headers = append(headers, fmt.Sprintf("X-Notification-Attempt-ID: %s", id))
		headers = append(headers, fmt.Sprintf("Message-ID: <%s@notification-service>", id))
	}
	if id := sanitizeIdentifier(req.NotificationID); id != "" {
		headers = append(headers, fmt.Sprintf("X-Notification-ID: %s", id))
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + body + "\r\n"
}
