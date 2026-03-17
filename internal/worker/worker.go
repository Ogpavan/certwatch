package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"domainguard/internal/notifications"
	"domainguard/internal/scanner"
)

type progressEvent struct {
	Domain    string    `json:"domain,omitempty"`
	Status    string    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type runTracker struct {
	wg sync.WaitGroup
}

func newRunTracker() *runTracker {
	tracker := &runTracker{}
	tracker.wg.Add(1)
	return tracker
}

func (t *runTracker) addJob() {
	t.wg.Add(1)
}

func (t *runTracker) done() {
	t.wg.Done()
}

func (t *runTracker) wait() {
	t.wg.Wait()
}

func (t *runTracker) dispatchDone() {
	t.wg.Done()
}

type DomainJob struct {
	ID      int
	Domain  string
	Port    int
	UserID  int
	RunID   string
	tracker *runTracker
}

type scanRequest struct {
	ctx     context.Context
	runID   string
	userIDs []int
}

type Scheduler struct {
	DB                *sql.DB
	Workers           int
	Interval          time.Duration
	Logger            *log.Logger
	Emailer           *notifications.EmailSender
	JobQueueSize      int
	DispatchQueueSize int
	jobs              chan DomainJob
	dispatchQueue     chan scanRequest

	progressMu      sync.Mutex
	progressHistory map[string][]progressEvent
	runStatus       map[string]bool

	trackerMu   sync.Mutex
	runTrackers map[string]*runTracker
	reportMu    sync.Mutex
	reportByRun map[string]map[int]*scanReport
}

func (s *Scheduler) Start(ctx context.Context) {
	if s.Workers <= 0 {
		s.Workers = 5
	}
	if s.Interval <= 0 {
		s.Interval = 24 * time.Hour
	}
	if s.Logger == nil {
		s.Logger = log.Default()
	}
	if s.JobQueueSize <= 0 {
		s.JobQueueSize = maxInt(s.Workers*256, 10000)
	}
	if s.DispatchQueueSize <= 0 {
		s.DispatchQueueSize = maxInt(s.Workers*4, 64)
	}

	if s.jobs == nil {
		s.jobs = make(chan DomainJob, s.JobQueueSize)
	}
	if s.dispatchQueue == nil {
		s.dispatchQueue = make(chan scanRequest, s.DispatchQueueSize)
	}
	for i := 0; i < s.Workers; i++ {
		go s.worker(ctx, s.jobs)
	}
	go s.dispatcher(ctx)

	go s.scheduleLoop(ctx)

	go func() {
		s.runAuto(ctx, nil)
		ticker := time.NewTicker(s.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAuto(ctx, nil)
			}
		}
	}()
}

func (s *Scheduler) scheduleLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			userIDs, err := s.dueUserIDs(now)
			if err != nil {
				if s.Logger != nil {
					s.Logger.Printf("scheduler: failed to compute due users: %v", err)
				}
				continue
			}
			if len(userIDs) == 0 {
				continue
			}
			if err := s.markUsersScanned(userIDs, now); err != nil && s.Logger != nil {
				s.Logger.Printf("scheduler: failed to mark users scanned: %v", err)
			}
			s.runAuto(ctx, userIDs)
		}
	}
}

func (s *Scheduler) dueUserIDs(now time.Time) ([]int, error) {
	rows, err := s.DB.Query(
		`SELECT DISTINCT p.user_id, s.schedule_time, s.last_scanned_at
     FROM projects p
     LEFT JOIN user_notification_settings s ON s.user_id=p.user_id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []int
	for rows.Next() {
		var userID int
		var schedule sql.NullString
		var last sql.NullTime
		if err := rows.Scan(&userID, &schedule, &last); err != nil {
			return nil, err
		}
		if last.Valid {
			last.Time = coerceLocalTime(last.Time)
		}
		if shouldRunNow(now, schedule.String, last) {
			result = append(result, userID)
		}
	}
	return result, nil
}

func shouldRunNow(now time.Time, schedule string, last sql.NullTime) bool {
	hour, minute := parseSchedule(schedule)
	target := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if now.Before(target) {
		return false
	}
	if last.Valid {
		lastTime := last.Time
		if sameDay(lastTime, now) && lastTime.After(target) {
			return false
		}
	}
	return true
}

func parseSchedule(value string) (int, int) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 3, 0
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 {
		return 3, 0
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 3, 0
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 3, 0
	}
	return hour, minute
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func coerceLocalTime(value time.Time) time.Time {
	return time.Date(
		value.Year(),
		value.Month(),
		value.Day(),
		value.Hour(),
		value.Minute(),
		value.Second(),
		value.Nanosecond(),
		time.Local,
	)
}

func (s *Scheduler) markUsersScanned(userIDs []int, now time.Time) error {
	for _, id := range userIDs {
		if _, err := s.DB.Exec(
			`IF EXISTS (SELECT 1 FROM dbo.user_notification_settings WHERE user_id=@p1)
        UPDATE dbo.user_notification_settings
        SET last_scanned_at=@p2
        WHERE user_id=@p1
      ELSE
        INSERT INTO dbo.user_notification_settings (user_id, email_enabled, email_recipients, notify_days, schedule_time, last_scanned_at)
        VALUES (@p1, 1, '[]', '["30","14","7","3"]', '03:00', @p2);`,
			id, now,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Scheduler) dispatcher(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req, ok := <-s.dispatchQueue:
			if !ok {
				return
			}
			s.dispatchRun(ctx, req)
		}
	}
}

func (s *Scheduler) dispatchRun(parent context.Context, req scanRequest) {
	tracker := s.trackerForRun(req.runID)
	if tracker == nil {
		return
	}
	defer tracker.dispatchDone()

	ctx := parent
	if req.ctx != nil {
		ctx = req.ctx
	}

	query := `SELECT d.id, d.domain, d.port, p.user_id
     FROM domains d
     JOIN projects p ON d.project_id=p.id`
	args := []interface{}{}
	if len(req.userIDs) > 0 {
		placeholders := make([]string, len(req.userIDs))
		for i, id := range req.userIDs {
			placeholders[i] = fmt.Sprintf("@p%d", i+1)
			args = append(args, id)
		}
		query += " WHERE p.user_id IN (" + strings.Join(placeholders, ",") + ")"
	}
	rows, err := s.DB.Query(query, args...)
	if err != nil {
		s.Logger.Printf("scanner: failed to load domains: %v", err)
		if req.runID != "" && !isAutoRunID(req.runID) {
			s.recordProgress(req.runID, progressEvent{Status: "error", Message: "failed to load domains", Timestamp: time.Now()})
		}
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, port, userID int
		var domain string
		if err := rows.Scan(&id, &domain, &port, &userID); err != nil {
			s.Logger.Printf("scanner: failed to read domain row: %v", err)
			continue
		}

		if tracker != nil {
			tracker.addJob()
		}

		select {
		case <-ctx.Done():
			if tracker != nil {
				tracker.done()
			}
			return
		case s.jobs <- DomainJob{ID: id, Domain: domain, Port: port, UserID: userID, RunID: req.runID, tracker: tracker}:
		}
	}
	if err := rows.Err(); err != nil && s.Logger != nil {
		s.Logger.Printf("scanner: rows iteration failed: %v", err)
		if req.runID != "" && !isAutoRunID(req.runID) {
			s.recordProgress(req.runID, progressEvent{Status: "error", Message: "domain iteration failed", Timestamp: time.Now()})
		}
	}
}

func (s *Scheduler) queueRun(ctx context.Context, runID string, userIDs []int) (*runTracker, error) {
	if s.dispatchQueue == nil {
		return nil, errors.New("scanner not started")
	}

	var tracker *runTracker
	if runID != "" {
		tracker = newRunTracker()
		s.storeTracker(runID, tracker)
		s.initReport(runID)
		if !isAutoRunID(runID) {
			s.resetProgress(runID)
			s.recordProgress(runID, progressEvent{Status: "accepted", Message: "scan request accepted", Timestamp: time.Now()})
		}
	}

	req := scanRequest{
		runID:   runID,
		userIDs: append([]int(nil), userIDs...),
	}

	select {
	case <-ctx.Done():
		if tracker != nil {
			s.deleteTracker(runID)
		}
		return nil, ctx.Err()
	case s.dispatchQueue <- req:
		if runID != "" && !isAutoRunID(runID) {
			s.recordProgress(runID, progressEvent{Status: "queued", Message: "scan queued for dispatch", Timestamp: time.Now()})
		}
		return tracker, nil
	default:
		if tracker != nil {
			s.deleteTracker(runID)
		}
		return nil, errors.New("scan queue is full")
	}
}

func (s *Scheduler) worker(ctx context.Context, jobs <-chan DomainJob) {
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-jobs:
			if !ok {
				return
			}

			if job.RunID != "" && !isAutoRunID(job.RunID) {
				s.recordProgress(job.RunID, progressEvent{Domain: job.Domain, Status: "scanning", Timestamp: time.Now()})
			}

			s.logEvent(job.ID, "INFO", fmt.Sprintf("Scan started for %s", job.Domain))
			result := scanner.ScanDomain(ctx, job.Domain, job.Port)
			if result.Error != nil {
				s.Logger.Printf("scanner: %s error: %v", job.Domain, result.Error)
				s.logEvent(job.ID, "ERROR", fmt.Sprintf("Scan error for %s: %v", job.Domain, result.Error))
			}

			if err := s.storeResult(job.ID, result); err != nil {
				s.Logger.Printf("scanner: failed to store result: %v", err)
				s.logEvent(job.ID, "ERROR", fmt.Sprintf("Failed to store scan result for %s: %v", job.Domain, err))
			}

			settings, err := s.loadNotificationSettings(job.ID)
			if err != nil {
				s.Logger.Printf("scanner: failed to load notification settings: %v", err)
				s.logEvent(job.ID, "ERROR", fmt.Sprintf("Failed to load notification settings for %s: %v", job.Domain, err))
			} else if err := s.generateAlerts(job.ID, job.Domain, job.Port, result, settings); err != nil {
				s.Logger.Printf("scanner: failed to generate alerts: %v", err)
				s.logEvent(job.ID, "ERROR", fmt.Sprintf("Failed to generate alerts for %s: %v", job.Domain, err))
			}
			s.logEvent(job.ID, "INFO", fmt.Sprintf("Scan completed for %s (status: %s)", job.Domain, result.Status))

			if job.RunID != "" && !isAutoRunID(job.RunID) {
				status := result.Status
				if status == "" {
					status = "completed"
				}
				s.recordProgress(job.RunID, progressEvent{Domain: job.Domain, Status: status, Message: "scanned", Timestamp: time.Now()})
			}

			if job.RunID != "" {
				s.appendReport(job.RunID, job.UserID, scanSummary{
					Domain:       job.Domain,
					Port:         job.Port,
					Status:       result.Status,
					SSLExpiry:    result.SSLExpiry,
					DomainExpiry: result.DomainExpiry,
					IPAddress:    result.IPAddress,
					Issuer:       result.Issuer,
					IssuerDN:     result.IssuerDN,
					TLSVersion:   result.TLSVersion,
					Error:        result.Error,
				})
			}

			if job.tracker != nil {
				job.tracker.done()
			}
		}
	}
}

func (s *Scheduler) storeResult(domainID int, result scanner.Result) error {
	_, err := s.DB.Exec(
		`INSERT INTO scan_results (domain_id, ssl_expiry, domain_expiry, tls_version, issuer, issuer_dn, ip_address, status, nameservers)
     VALUES (@p1, @p2, @p3, @p4, @p5, @p6, @p7, @p8, @p9)`,
		domainID,
		result.SSLExpiry,
		result.DomainExpiry,
		nullIfEmpty(result.TLSVersion),
		nullIfEmpty(result.Issuer),
		nullIfEmpty(result.IssuerDN),
		nullIfEmpty(result.IPAddress),
		nullIfEmpty(result.Status),
		nullIfEmpty(joinNameservers(result.Nameservers)),
	)
	return err
}

func (s *Scheduler) generateAlerts(domainID int, domain string, port int, result scanner.Result, settings notificationSettings) error {
	if result.SSLExpiry != nil {
		daysLeftRaw := int(math.Ceil(time.Until(*result.SSLExpiry).Hours() / 24))
		isExpired := daysLeftRaw < 0
		daysLeft := daysLeftRaw
		if isExpired {
			daysLeft = 0
		}
		threshold := pickThreshold(settings.NotifyDays, daysLeft)
		if threshold > 0 {
			severity := "Warning"
			if threshold <= 7 {
				severity = "Critical"
			}
			message := fmt.Sprintf("SSL certificate expires in %d day%s (threshold %d)", daysLeft, plural(daysLeft), threshold)
			if isExpired {
				message = fmt.Sprintf("SSL certificate has expired (threshold %d)", threshold)
			}
			created, err := s.upsertAlert(domainID, "SSL", severity, message)
			if err != nil {
				return err
			}
			if created {
				s.logEvent(domainID, "WARN", fmt.Sprintf("SSL alert created for %s: %s", domain, message))
			}
		}
	}

	if result.DomainExpiry != nil {
		days := int(time.Until(*result.DomainExpiry).Hours() / 24)
		if days < 30 {
			if _, err := s.upsertAlert(domainID, "Domain", "Warning", "Domain expires in less than 30 days"); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Scheduler) upsertAlert(domainID int, alertType, severity, message string) (bool, error) {
	var count int
	err := s.DB.QueryRow(
		`SELECT COUNT(1) FROM alerts
     WHERE domain_id=@p1 AND type=@p2 AND severity=@p3 AND message=@p4 AND resolved=0`,
		domainID, alertType, severity, message,
	).Scan(&count)
	if err != nil {
		return false, err
	}

	if count > 0 {
		return false, nil
	}

	_, err = s.DB.Exec(
		`INSERT INTO alerts (domain_id, type, severity, message)
     VALUES (@p1, @p2, @p3, @p4)`,
		domainID, alertType, severity, message,
	)
	if err != nil {
		return false, err
	}
	return true, nil
}

func nullIfEmpty(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func joinNameservers(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return strings.Join(names, ", ")
}

type notificationSettings struct {
	EmailEnabled    bool
	EmailRecipients []string
	NotifyDays      []int
}

func (s *Scheduler) loadNotificationSettings(domainID int) (notificationSettings, error) {
	var enabled sql.NullBool
	var recipientsRaw sql.NullString
	var daysRaw sql.NullString
	err := s.DB.QueryRow(
		`SELECT s.email_enabled, s.email_recipients, s.notify_days
     FROM domains d
     JOIN projects p ON d.project_id=p.id
     LEFT JOIN user_notification_settings s ON s.user_id=p.user_id
     WHERE d.id=@p1`,
		domainID,
	).Scan(&enabled, &recipientsRaw, &daysRaw)
	if err != nil {
		return notificationSettings{}, err
	}

	recipients := []string{}
	if recipientsRaw.Valid && strings.TrimSpace(recipientsRaw.String) != "" {
		_ = json.Unmarshal([]byte(recipientsRaw.String), &recipients)
	}

	notifyDays := defaultNotifyDays()
	if daysRaw.Valid && strings.TrimSpace(daysRaw.String) != "" {
		var raw []string
		if err := json.Unmarshal([]byte(daysRaw.String), &raw); err == nil {
			notifyDays = parseNotifyDays(raw)
		}
	}

	emailEnabled := true
	if enabled.Valid {
		emailEnabled = enabled.Bool
	}

	return notificationSettings{
		EmailEnabled:    emailEnabled,
		EmailRecipients: recipients,
		NotifyDays:      notifyDays,
	}, nil
}

func defaultNotifyDays() []int {
	return []int{30, 14, 7, 3}
}

func parseNotifyDays(raw []string) []int {
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
	return values
}

func pickThreshold(thresholds []int, daysLeft int) int {
	if len(thresholds) == 0 {
		return 0
	}
	for _, value := range thresholds {
		if daysLeft <= value {
			return value
		}
	}
	return 0
}

func plural(value int) string {
	if value == 1 {
		return ""
	}
	return "s"
}

func (s *Scheduler) logEvent(domainID int, level, message string) {
	if s.DB == nil {
		return
	}
	userID, err := s.userIDForDomain(domainID)
	if err != nil {
		return
	}
	_, err = s.DB.Exec(
		"INSERT INTO logs (user_id, domain_id, level, message) VALUES (@p1, @p2, @p3, @p4)",
		userID, domainID, level, message,
	)
	if err != nil && s.Logger != nil {
		s.Logger.Printf("logger: failed to write log: %v", err)
	}
}

func (s *Scheduler) userIDForDomain(domainID int) (int, error) {
	var userID int
	err := s.DB.QueryRow(
		`SELECT p.user_id
     FROM domains d
     JOIN projects p ON d.project_id=p.id
     WHERE d.id=@p1`,
		domainID,
	).Scan(&userID)
	return userID, err
}

func (s *Scheduler) ScanNow(ctx context.Context, runID string) error {
	tracker, err := s.queueRun(ctx, runID, nil)
	if err != nil {
		return err
	}
	go func() {
		s.finishRun(runID, tracker, true)
	}()
	return nil
}

func (s *Scheduler) recordProgress(runID string, event progressEvent) {
	if runID == "" || isAutoRunID(runID) {
		return
	}
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if s.progressHistory == nil {
		s.progressHistory = make(map[string][]progressEvent)
	}
	s.progressHistory[runID] = append(s.progressHistory[runID], event)
	if len(s.progressHistory[runID]) > 64 {
		s.progressHistory[runID] = s.progressHistory[runID][1:]
	}
}

func (s *Scheduler) resetProgress(runID string) {
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if s.progressHistory == nil {
		s.progressHistory = make(map[string][]progressEvent)
	}
	if s.runStatus == nil {
		s.runStatus = make(map[string]bool)
	}
	s.progressHistory[runID] = nil
	s.runStatus[runID] = false
}

func (s *Scheduler) markRunDone(runID string) {
	if runID == "" || isAutoRunID(runID) {
		return
	}
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	if s.runStatus == nil {
		s.runStatus = make(map[string]bool)
	}
	s.runStatus[runID] = true
}

func (s *Scheduler) GetProgress(runID string) ([]progressEvent, bool) {
	if runID == "" {
		return nil, false
	}
	s.progressMu.Lock()
	defer s.progressMu.Unlock()
	events := append([]progressEvent(nil), s.progressHistory[runID]...)
	done := s.runStatus[runID]
	if done {
		delete(s.progressHistory, runID)
		delete(s.runStatus, runID)
	}
	return events, done
}

type scanSummary struct {
	Domain       string
	Port         int
	Status       string
	SSLExpiry    *time.Time
	DomainExpiry *time.Time
	IPAddress    string
	Issuer       string
	IssuerDN     string
	TLSVersion   string
	Error        error
}

type scanReport struct {
	UserID  int
	Started time.Time
	Items   []scanSummary
}

func (s *Scheduler) trackerForRun(runID string) *runTracker {
	if runID == "" {
		return nil
	}
	s.trackerMu.Lock()
	defer s.trackerMu.Unlock()
	return s.runTrackers[runID]
}

func (s *Scheduler) storeTracker(runID string, tracker *runTracker) {
	if runID == "" || tracker == nil {
		return
	}
	s.trackerMu.Lock()
	defer s.trackerMu.Unlock()
	if s.runTrackers == nil {
		s.runTrackers = make(map[string]*runTracker)
	}
	s.runTrackers[runID] = tracker
}

func (s *Scheduler) deleteTracker(runID string) {
	if runID == "" {
		return
	}
	s.trackerMu.Lock()
	defer s.trackerMu.Unlock()
	delete(s.runTrackers, runID)
}

func (s *Scheduler) initReport(runID string) {
	if runID == "" {
		return
	}
	s.reportMu.Lock()
	defer s.reportMu.Unlock()
	if s.reportByRun == nil {
		s.reportByRun = make(map[string]map[int]*scanReport)
	}
	if _, exists := s.reportByRun[runID]; !exists {
		s.reportByRun[runID] = make(map[int]*scanReport)
	}
}

func (s *Scheduler) appendReport(runID string, userID int, summary scanSummary) {
	if runID == "" {
		return
	}
	s.reportMu.Lock()
	defer s.reportMu.Unlock()
	if s.reportByRun == nil {
		s.reportByRun = make(map[string]map[int]*scanReport)
	}
	runMap, exists := s.reportByRun[runID]
	if !exists {
		runMap = make(map[int]*scanReport)
		s.reportByRun[runID] = runMap
	}
	report := runMap[userID]
	if report == nil {
		report = &scanReport{
			UserID:  userID,
			Started: time.Now(),
		}
		runMap[userID] = report
	}
	report.Items = append(report.Items, summary)
}

func (s *Scheduler) finishRun(runID string, tracker *runTracker, markDone bool) {
	if tracker != nil {
		tracker.wait()
		s.deleteTracker(runID)
	}
	if markDone {
		s.recordProgress(runID, progressEvent{Status: "done", Message: "scan finished", Timestamp: time.Now()})
		s.markRunDone(runID)
	}
	if runID != "" && !isAutoRunID(runID) {
		s.markManualRunScanned(runID)
	}
	s.sendAggregatedReports(runID)
}

func (s *Scheduler) markManualRunScanned(runID string) {
	if runID == "" {
		return
	}
	s.reportMu.Lock()
	runMap := s.reportByRun[runID]
	s.reportMu.Unlock()
	if len(runMap) == 0 {
		return
	}
	userIDs := make([]int, 0, len(runMap))
	for userID := range runMap {
		userIDs = append(userIDs, userID)
	}
	now := time.Now()
	if err := s.markUsersScanned(userIDs, now); err != nil && s.Logger != nil {
		s.Logger.Printf("scheduler: failed to mark manual scan as completed: %v", err)
	}
}

func (s *Scheduler) sendAggregatedReports(runID string) {
	if runID == "" {
		return
	}
	if s.Emailer == nil || !s.Emailer.Enabled() {
		return
	}
	s.reportMu.Lock()
	runMap := s.reportByRun[runID]
	delete(s.reportByRun, runID)
	s.reportMu.Unlock()

	if len(runMap) == 0 {
		return
	}

	for userID, report := range runMap {
		if len(report.Items) == 0 {
			continue
		}

		settings, err := s.loadNotificationSettingsForUser(userID)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Printf("email: failed to load notification settings for user %d: %v", userID, err)
			}
			continue
		}
		if !settings.EmailEnabled || len(settings.EmailRecipients) == 0 {
			continue
		}

		expiring := filterExpiring(report.Items, settings.NotifyDays)
		if len(expiring) == 0 {
			continue
		}

		sort.Slice(expiring, func(i, j int) bool {
			if expiring[i].Domain == expiring[j].Domain {
				return expiring[i].Port < expiring[j].Port
			}
			return expiring[i].Domain < expiring[j].Domain
		})

		now := time.Now().UTC()
		body := buildExpiryEmailHTML(expiring, settings.NotifyDays, now)
		subject := fmt.Sprintf("[SSL] Expiring certificates (%d)", len(expiring))
		if err := s.Emailer.SendHTML(settings.EmailRecipients, subject, body); err != nil {
			if s.Logger != nil {
				s.Logger.Printf("email: failed to send expiry mail for user %d: %v", userID, err)
			}
		}
	}
}

func filterExpiring(items []scanSummary, thresholds []int) []scanSummary {
	if len(items) == 0 {
		return nil
	}
	maxThreshold := 0
	for _, value := range thresholds {
		if value > maxThreshold {
			maxThreshold = value
		}
	}
	if maxThreshold <= 0 {
		return nil
	}
	result := make([]scanSummary, 0, len(items))
	for _, item := range items {
		if item.SSLExpiry == nil {
			continue
		}
		daysLeft := int(math.Ceil(time.Until(*item.SSLExpiry).Hours() / 24))
		if daysLeft < 0 || daysLeft <= maxThreshold {
			result = append(result, item)
		}
	}
	return result
}

func buildExpiryEmailHTML(items []scanSummary, thresholds []int, now time.Time) string {
	builder := strings.Builder{}
	writeLine := func(value string) {
		builder.WriteString(value)
		builder.WriteString("\r\n")
	}

	writeLine("<!doctype html>")
	writeLine("<html>")
	writeLine("<body style=\"margin:0;padding:0;background:#f8fafc;font-family:Arial,Helvetica,sans-serif;color:#0f172a;\">")
	writeLine("<div style=\"max-width:980px;margin:0 auto;padding:24px;\">")
	writeLine("<div style=\"background:#ffffff;border:1px solid #e2e8f0;border-radius:12px;padding:20px;\">")
	writeLine("<h2 style=\"margin:0 0 8px 0;font-size:20px;\">SSL expiry summary</h2>")
	writeLine(fmt.Sprintf("<div style=\"font-size:13px;color:#475569;\">Expiring certificates: %d</div>", len(items)))
	writeLine(fmt.Sprintf("<div style=\"font-size:13px;color:#475569;margin-bottom:16px;\">Generated at (UTC): %s</div>", htmlEscape(now.Format(time.RFC3339))))
	writeLine("<table style=\"width:100%;border-collapse:collapse;font-size:13px;\">")
	writeLine("<thead>")
	writeLine("<tr style=\"background:#0f172a;color:#f8fafc;\">")
	writeLine("<th style=\"text-align:left;padding:10px 12px;\">Domain</th>")
	writeLine("<th style=\"text-align:left;padding:10px 12px;\">SSL Expiry</th>")
	writeLine("<th style=\"text-align:left;padding:10px 12px;\">Days Left</th>")
	writeLine("<th style=\"text-align:left;padding:10px 12px;\">Domain Expiry</th>")
	writeLine("<th style=\"text-align:left;padding:10px 12px;\">IP Address</th>")
	writeLine("<th style=\"text-align:left;padding:10px 12px;\">Issuer</th>")
	writeLine("<th style=\"text-align:left;padding:10px 12px;\">TLS</th>")
	writeLine("</tr>")
	writeLine("</thead>")
	writeLine("<tbody>")

	for _, item := range items {
		name := fmt.Sprintf("%s:%d", item.Domain, item.Port)
		sslExpiry := formatShortDate(item.SSLExpiry)
		daysLeft := "-"
		if item.SSLExpiry != nil {
			daysLeftValue := int(math.Ceil(time.Until(*item.SSLExpiry).Hours() / 24))
			if daysLeftValue < 0 {
				daysLeftValue = 0
			}
			daysLeft = strconv.Itoa(daysLeftValue)
		}
		domainExpiry := formatShortDate(item.DomainExpiry)
		ipAddress := valueOrDash(item.IPAddress)
		issuer := valueOrDash(item.Issuer)
		if issuer == "-" && item.IssuerDN != "" {
			issuer = item.IssuerDN
		}
		tls := valueOrDash(item.TLSVersion)

		rowColor := rowColorForExpiry(item.SSLExpiry, thresholds)
		writeLine(fmt.Sprintf("<tr style=\"background:%s;\">", rowColor))
		writeLine(fmt.Sprintf("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;\">%s</td>", htmlEscape(name)))
		writeLine(fmt.Sprintf("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;\">%s</td>", htmlEscape(sslExpiry)))
		writeLine(fmt.Sprintf("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;\">%s</td>", htmlEscape(daysLeft)))
		writeLine(fmt.Sprintf("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;\">%s</td>", htmlEscape(domainExpiry)))
		writeLine(fmt.Sprintf("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;\">%s</td>", htmlEscape(ipAddress)))
		writeLine(fmt.Sprintf("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;\">%s</td>", htmlEscape(issuer)))
		writeLine(fmt.Sprintf("<td style=\"padding:10px 12px;border-bottom:1px solid #e2e8f0;\">%s</td>", htmlEscape(tls)))
		writeLine("</tr>")
	}

	writeLine("</tbody>")
	writeLine("</table>")
	writeLine("<div style=\"margin-top:14px;font-size:12px;color:#64748b;\">This email includes only SSL certificates expiring based on your configured notification days.</div>")
	writeLine("</div>")
	writeLine("</div>")
	writeLine("</body>")
	writeLine("</html>")
	return builder.String()
}

func rowColorForExpiry(sslExpiry *time.Time, thresholds []int) string {
	if sslExpiry == nil {
		return "#f1f5f9"
	}
	daysLeft := int(math.Ceil(time.Until(*sslExpiry).Hours() / 24))
	if daysLeft < 0 {
		return "#fecaca"
	}
	threshold := pickThreshold(thresholds, daysLeft)
	if threshold <= 0 {
		return "#dcfce7"
	}
	if threshold <= 3 {
		return "#fee2e2"
	}
	if threshold <= 7 {
		return "#ffedd5"
	}
	return "#fef9c3"
}

func formatShortDate(value *time.Time) string {
	if value == nil {
		return "-"
	}
	return value.Format("02-Jan-2006")
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}

func htmlEscape(value string) string {
	return html.EscapeString(value)
}

func (s *Scheduler) loadNotificationSettingsForUser(userID int) (notificationSettings, error) {
	var enabled sql.NullBool
	var recipientsRaw sql.NullString
	var daysRaw sql.NullString
	err := s.DB.QueryRow(
		`SELECT email_enabled, email_recipients, notify_days
     FROM user_notification_settings
     WHERE user_id=@p1`,
		userID,
	).Scan(&enabled, &recipientsRaw, &daysRaw)
	if err != nil {
		if err == sql.ErrNoRows {
			return notificationSettings{
				EmailEnabled:    true,
				EmailRecipients: []string{},
				NotifyDays:      defaultNotifyDays(),
			}, nil
		}
		return notificationSettings{}, err
	}

	recipients := []string{}
	if recipientsRaw.Valid && strings.TrimSpace(recipientsRaw.String) != "" {
		_ = json.Unmarshal([]byte(recipientsRaw.String), &recipients)
	}

	notifyDays := defaultNotifyDays()
	if daysRaw.Valid && strings.TrimSpace(daysRaw.String) != "" {
		var raw []string
		if err := json.Unmarshal([]byte(daysRaw.String), &raw); err == nil {
			notifyDays = parseNotifyDays(raw)
		}
	}

	emailEnabled := true
	if enabled.Valid {
		emailEnabled = enabled.Bool
	}

	return notificationSettings{
		EmailEnabled:    emailEnabled,
		EmailRecipients: recipients,
		NotifyDays:      notifyDays,
	}, nil
}

func (s *Scheduler) runAuto(ctx context.Context, userIDs []int) {
	runID := fmt.Sprintf("auto-%d", time.Now().UnixNano())
	tracker, err := s.queueRun(ctx, runID, userIDs)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Printf("scheduler: failed to queue automatic scan: %v", err)
		}
		return
	}
	go s.finishRun(runID, tracker, false)
}

func isAutoRunID(runID string) bool {
	return strings.HasPrefix(runID, "auto-")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
