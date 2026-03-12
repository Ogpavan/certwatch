package notifications

import (
  "crypto/tls"
  "fmt"
  "net/smtp"
  "strings"
)

type EmailSender struct {
  Host     string
  Port     int
  Username string
  Password string
  From     string
}

const defaultDisplayName = "Cert Watch"

func (s *EmailSender) Enabled() bool {
  return s != nil && s.Host != "" && s.Port > 0 && s.Username != "" && s.Password != ""
}

func (s *EmailSender) Send(to []string, subject, body string) error {
  return s.sendWithContentType(to, subject, body, "text/plain; charset=UTF-8")
}

func (s *EmailSender) SendHTML(to []string, subject, htmlBody string) error {
  return s.sendWithContentType(to, subject, htmlBody, "text/html; charset=UTF-8")
}

func (s *EmailSender) sendWithContentType(to []string, subject, body, contentType string) error {
  if !s.Enabled() {
    return fmt.Errorf("email sender not configured")
  }
  recipients := filterRecipients(to)
  if len(recipients) == 0 {
    return fmt.Errorf("no recipients provided")
  }

  from := s.From
  if strings.TrimSpace(from) == "" {
    from = s.Username
  }
  envelopeFrom := extractAddress(from)
  headerFrom := buildHeaderFrom(from, envelopeFrom)

  addr := fmt.Sprintf("%s:%d", s.Host, s.Port)
  client, err := smtp.Dial(addr)
  if err != nil {
    return err
  }
  defer client.Close()

  if ok, _ := client.Extension("STARTTLS"); ok {
    tlsConfig := &tls.Config{ServerName: s.Host}
    if err := client.StartTLS(tlsConfig); err != nil {
      return err
    }
  }

  auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
  if err := client.Auth(auth); err != nil {
    return err
  }

  if err := client.Mail(envelopeFrom); err != nil {
    return err
  }
  for _, rcpt := range recipients {
    if err := client.Rcpt(rcpt); err != nil {
      return err
    }
  }

  writer, err := client.Data()
  if err != nil {
    return err
  }

  message := buildMessage(headerFrom, recipients, subject, body, contentType)
  if _, err := writer.Write([]byte(message)); err != nil {
    _ = writer.Close()
    return err
  }
  if err := writer.Close(); err != nil {
    return err
  }

  if err := client.Quit(); err != nil {
    if isBenignSMTPError(err) {
      return nil
    }
    return err
  }
  return nil
}

func buildMessage(from string, to []string, subject, body, contentType string) string {
  headers := []string{
    fmt.Sprintf("From: %s", from),
    fmt.Sprintf("To: %s", strings.Join(to, ", ")),
    fmt.Sprintf("Subject: %s", subject),
    "MIME-Version: 1.0",
    fmt.Sprintf("Content-Type: %s", contentType),
    "",
  }
  return strings.Join(headers, "\r\n") + body + "\r\n"
}

func filterRecipients(raw []string) []string {
  cleaned := make([]string, 0, len(raw))
  seen := make(map[string]struct{})
  for _, value := range raw {
    trimmed := strings.TrimSpace(value)
    if trimmed == "" {
      continue
    }
    key := strings.ToLower(trimmed)
    if _, exists := seen[key]; exists {
      continue
    }
    seen[key] = struct{}{}
    cleaned = append(cleaned, trimmed)
  }
  return cleaned
}

func isBenignSMTPError(err error) bool {
  if err == nil {
    return false
  }
  message := strings.ToLower(err.Error())
  if strings.Contains(message, "250 2.0.0 ok") {
    return true
  }
  if strings.HasPrefix(message, "250 ") && strings.Contains(message, "ok") {
    return true
  }
  return false
}

func extractAddress(value string) string {
  trimmed := strings.TrimSpace(value)
  if trimmed == "" {
    return ""
  }
  start := strings.Index(trimmed, "<")
  end := strings.Index(trimmed, ">")
  if start >= 0 && end > start {
    addr := strings.TrimSpace(trimmed[start+1 : end])
    if addr != "" {
      return addr
    }
  }
  return trimmed
}

func buildHeaderFrom(raw string, address string) string {
  if strings.Contains(raw, "<") && strings.Contains(raw, ">") {
    return raw
  }
  if address == "" {
    return raw
  }
  if strings.TrimSpace(defaultDisplayName) == "" {
    return address
  }
  return fmt.Sprintf("%s <%s>", defaultDisplayName, address)
}
