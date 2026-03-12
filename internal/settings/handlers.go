package settings

import (
  "database/sql"
  "encoding/json"
  "net/http"
  "net/mail"
  "sort"
  "strconv"
  "strings"
  "time"

  "domainguard/internal/middleware"
  "github.com/gin-gonic/gin"
)

type Handler struct {
  DB *sql.DB
}

type notificationSettings struct {
  EmailEnabled    bool     `json:"email_enabled"`
  EmailRecipients []string `json:"email_recipients"`
  NotifyDays      []string `json:"notify_days"`
  ScheduleTime    string    `json:"schedule_time"`
  LastScannedAt   *time.Time `json:"last_scanned_at"`
}

func (h *Handler) GetNotifications(c *gin.Context) {
  userID := middleware.GetUserID(c)
  var enabled bool
  var recipientsRaw string
  var daysRaw string
  var scheduleTime string
  var lastScanned sql.NullTime
  err := h.DB.QueryRow(
    "SELECT email_enabled, email_recipients, notify_days, schedule_time, last_scanned_at FROM user_notification_settings WHERE user_id=@p1",
    userID,
  ).Scan(&enabled, &recipientsRaw, &daysRaw, &scheduleTime, &lastScanned)
  if err != nil {
    if err == sql.ErrNoRows {
      c.JSON(http.StatusOK, notificationSettings{
        EmailEnabled:    true,
        EmailRecipients: []string{},
        NotifyDays:      defaultNotifyDays(),
        ScheduleTime:    defaultScheduleTime(),
        LastScannedAt:   nil,
      })
      return
    }
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load settings"})
    return
  }

  recipients := []string{}
  if strings.TrimSpace(recipientsRaw) != "" {
    if err := json.Unmarshal([]byte(recipientsRaw), &recipients); err != nil {
      recipients = []string{}
    }
  }

  days := []string{}
  if strings.TrimSpace(daysRaw) != "" {
    if err := json.Unmarshal([]byte(daysRaw), &days); err != nil {
      days = []string{}
    }
  }
  c.JSON(http.StatusOK, notificationSettings{
    EmailEnabled:    enabled,
    EmailRecipients: recipients,
    NotifyDays:      normalizeDays(days),
    ScheduleTime:    normalizeScheduleTime(scheduleTime),
    LastScannedAt:   toTimePtr(lastScanned),
  })
}

func (h *Handler) UpdateNotifications(c *gin.Context) {
  userID := middleware.GetUserID(c)
  var req notificationSettings
  if err := c.ShouldBindJSON(&req); err != nil {
    c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
    return
  }

  cleaned := dedupeEmails(req.EmailRecipients)
  if req.NotifyDays == nil {
    req.NotifyDays = defaultNotifyDays()
  }
  normalizedDays := normalizeDays(req.NotifyDays)

  payload, err := json.Marshal(cleaned)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize recipients"})
    return
  }

  daysPayload, err := json.Marshal(normalizedDays)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to serialize notify days"})
    return
  }

  scheduleValue := normalizeScheduleTime(req.ScheduleTime)
  lastOverride, err := h.computeScheduleOverride(userID, scheduleValue)
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to evaluate schedule update"})
    return
  }

  _, err = h.DB.Exec(
    `IF EXISTS (SELECT 1 FROM dbo.user_notification_settings WHERE user_id=@p1)
        UPDATE dbo.user_notification_settings
        SET email_enabled=@p2,
            email_recipients=@p3,
            notify_days=@p4,
            schedule_time=@p5,
            last_scanned_at=COALESCE(@p6, last_scanned_at),
            updated_at=GETDATE()
        WHERE user_id=@p1
      ELSE
        INSERT INTO dbo.user_notification_settings (user_id, email_enabled, email_recipients, notify_days, schedule_time, last_scanned_at)
        VALUES (@p1, @p2, @p3, @p4, @p5, @p6);`,
    userID, req.EmailEnabled, string(payload), string(daysPayload), scheduleValue, lastOverride,
  )
  if err != nil {
    c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save settings"})
    return
  }

  c.JSON(http.StatusOK, notificationSettings{
    EmailEnabled:    req.EmailEnabled,
    EmailRecipients: cleaned,
    NotifyDays:      normalizedDays,
    ScheduleTime:    scheduleValue,
    LastScannedAt:   nil,
  })
}

func dedupeEmails(raw []string) []string {
  seen := make(map[string]struct{})
  cleaned := make([]string, 0, len(raw))
  for _, value := range raw {
    trimmed := strings.ToLower(strings.TrimSpace(value))
    if trimmed == "" {
      continue
    }
    if _, err := mail.ParseAddress(trimmed); err != nil {
      continue
    }
    if _, exists := seen[trimmed]; exists {
      continue
    }
    seen[trimmed] = struct{}{}
    cleaned = append(cleaned, trimmed)
  }
  return cleaned
}

func defaultNotifyDays() []string {
  return []string{"30", "14", "7", "3"}
}

func normalizeDays(raw []string) []string {
  if len(raw) == 0 {
    return []string{}
  }
  seen := make(map[int]struct{})
  values := make([]int, 0, len(raw))
  for _, value := range raw {
    trimmed := strings.TrimSpace(value)
    if trimmed == "" {
      continue
    }
    parsed, err := strconv.Atoi(trimmed)
    if err != nil {
      continue
    }
    if parsed < 1 || parsed > 365 {
      continue
    }
    if _, exists := seen[parsed]; exists {
      continue
    }
    seen[parsed] = struct{}{}
    values = append(values, parsed)
  }
  sort.Ints(values)
  normalized := make([]string, 0, len(values))
  for _, value := range values {
    normalized = append(normalized, strconv.Itoa(value))
  }
  return normalized
}

func toTimePtr(value sql.NullTime) *time.Time {
  if !value.Valid {
    return nil
  }
  return &value.Time
}

func defaultScheduleTime() string {
  return "03:00"
}

func normalizeScheduleTime(value string) string {
  trimmed := strings.TrimSpace(value)
  if trimmed == "" {
    return defaultScheduleTime()
  }
  if _, err := time.Parse("15:04", trimmed); err != nil {
    return defaultScheduleTime()
  }
  return trimmed
}

func (h *Handler) computeScheduleOverride(userID int, scheduleValue string) (*time.Time, error) {
  var existingSchedule sql.NullString
  var lastScanned sql.NullTime
  err := h.DB.QueryRow(
    "SELECT schedule_time, last_scanned_at FROM user_notification_settings WHERE user_id=@p1",
    userID,
  ).Scan(&existingSchedule, &lastScanned)
  if err != nil {
    if err == sql.ErrNoRows {
      return nil, nil
    }
    return nil, err
  }

  normalizedExisting := normalizeScheduleTime(existingSchedule.String)
  if normalizedExisting == scheduleValue {
    return nil, nil
  }

  if !lastScanned.Valid {
    return nil, nil
  }

  now := time.Now()
  target, ok := scheduleTargetTime(now, scheduleValue)
  if !ok {
    return nil, nil
  }

  if now.Before(target) {
    return nil, nil
  }

  if sameDay(lastScanned.Time, now) && lastScanned.Time.After(target) {
    override := target.Add(-time.Second)
    return &override, nil
  }

  return nil, nil
}

func scheduleTargetTime(now time.Time, scheduleValue string) (time.Time, bool) {
  parsed, err := time.Parse("15:04", scheduleValue)
  if err != nil {
    return time.Time{}, false
  }
  return time.Date(now.Year(), now.Month(), now.Day(), parsed.Hour(), parsed.Minute(), 0, 0, now.Location()), true
}

func sameDay(a, b time.Time) bool {
  return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}
