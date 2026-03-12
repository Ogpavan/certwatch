package models

import "time"

type User struct {
  ID        int       `json:"id"`
  Email     string    `json:"email"`
  Password  string    `json:"-"`
  CreatedAt time.Time `json:"created_at"`
}

type Project struct {
  ID        int       `json:"id"`
  UserID    int       `json:"user_id"`
  Name      string    `json:"name"`
  CreatedAt time.Time `json:"created_at"`
}

type Domain struct {
  ID        int       `json:"id"`
  ProjectID int       `json:"project_id"`
  Domain    string    `json:"domain"`
  Port      int       `json:"port"`
  CreatedAt time.Time `json:"created_at"`
}

type ScanResult struct {
  ID           int        `json:"id"`
  DomainID     int        `json:"domain_id"`
  SSLExpiry    *time.Time `json:"ssl_expiry"`
  DomainExpiry *time.Time `json:"domain_expiry"`
  TLSVersion   string     `json:"tls_version"`
  Issuer       string     `json:"issuer"`
  IPAddress    string     `json:"ip_address"`
  Status       string     `json:"status"`
  CheckedAt    time.Time  `json:"checked_at"`
}

type Alert struct {
  ID        int       `json:"id"`
  DomainID  int       `json:"domain_id"`
  Type      string    `json:"type"`
  Severity  string    `json:"severity"`
  Message   string    `json:"message"`
  CreatedAt time.Time `json:"created_at"`
  Resolved  bool      `json:"resolved"`
}
