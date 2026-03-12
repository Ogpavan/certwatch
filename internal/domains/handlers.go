package domains

import (
	"database/sql"
	"net/http"
	"strconv"

	"domainguard/internal/middleware"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	DB *sql.DB
}

type createRequest struct {
	ProjectID int    `json:"project_id" binding:"required"`
	Domain    string `json:"domain" binding:"required"`
	Port      int    `json:"port"`
}

func (h *Handler) List(c *gin.Context) {
	userID := middleware.GetUserID(c)
	rows, err := h.DB.Query(
		`SELECT d.id, d.project_id, d.domain, d.port, d.created_at, p.name
     FROM domains d
     INNER JOIN projects p ON d.project_id = p.id
     WHERE p.user_id=@p1
     ORDER BY d.created_at DESC`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch domains"})
		return
	}
	defer rows.Close()

	var result []gin.H
	for rows.Next() {
		var id, projectID, port int
		var domain string
		var createdAt sql.NullTime
		var projectName string
		if err := rows.Scan(&id, &projectID, &domain, &port, &createdAt, &projectName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse domains"})
			return
		}
		result = append(result, gin.H{
			"id":           id,
			"project_id":   projectID,
			"project_name": projectName,
			"domain":       domain,
			"port":         port,
			"created_at":   createdAt.Time,
		})
	}

	c.JSON(http.StatusOK, result)
}

func (h *Handler) Create(c *gin.Context) {
	userID := middleware.GetUserID(c)
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Port == 0 {
		req.Port = 443
	}

	var exists int
	err := h.DB.QueryRow("SELECT COUNT(1) FROM projects WHERE id=@p1 AND user_id=@p2", req.ProjectID, userID).Scan(&exists)
	if err != nil || exists == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project"})
		return
	}

	var id int
	err = h.DB.QueryRow(
		"INSERT INTO domains (project_id, domain, port) OUTPUT INSERTED.id VALUES (@p1, @p2, @p3)",
		req.ProjectID, req.Domain, req.Port,
	).Scan(&id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create domain"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *Handler) Delete(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	tx, err := h.DB.Begin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM alerts WHERE domain_id=@p1", id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if _, err := tx.Exec("DELETE FROM scan_results WHERE domain_id=@p1", id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	res, err := tx.Exec(
		`DELETE d
     FROM domains d
     INNER JOIN projects p ON d.project_id = p.id
     WHERE d.id=@p1 AND p.user_id=@p2`,
		id, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if rows, _ := res.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *Handler) Get(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	var domainID, projectID, port int
	var domain string
	var createdAt sql.NullTime
	var projectName string
	err = h.DB.QueryRow(
		`SELECT d.id, d.project_id, d.domain, d.port, d.created_at, p.name
     FROM domains d
     INNER JOIN projects p ON d.project_id = p.id
     WHERE d.id=@p1 AND p.user_id=@p2`,
		id, userID,
	).Scan(&domainID, &projectID, &domain, &port, &createdAt, &projectName)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	var scan gin.H
	row := h.DB.QueryRow(
		`SELECT TOP 1 ssl_expiry, domain_expiry, tls_version, issuer, issuer_dn, ip_address, status, nameservers, checked_at
     FROM scan_results WHERE domain_id=@p1 ORDER BY checked_at DESC`,
		id,
	)
	var sslExpiry, domainExpiry sql.NullTime
	var tlsVersion, issuer, issuerDN, ipAddress, status, nameservers sql.NullString
	var checkedAt sql.NullTime
	if err := row.Scan(&sslExpiry, &domainExpiry, &tlsVersion, &issuer, &issuerDN, &ipAddress, &status, &nameservers, &checkedAt); err == nil {
		scan = gin.H{
			"ssl_expiry":    sslExpiry.Time,
			"domain_expiry": domainExpiry.Time,
			"tls_version":   tlsVersion.String,
			"issuer":        issuer.String,
			"issuer_dn":     issuerDN.String,
			"ip_address":    ipAddress.String,
			"status":        status.String,
			"nameservers":   nameservers.String,
			"checked_at":    checkedAt.Time,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           domainID,
		"project_id":   projectID,
		"project_name": projectName,
		"domain":       domain,
		"port":         port,
		"created_at":   createdAt.Time,
		"latest_scan":  scan,
	})
}

func (h *Handler) History(c *gin.Context) {
	userID := middleware.GetUserID(c)
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid domain id"})
		return
	}

	var exists int
	err = h.DB.QueryRow(
		`SELECT COUNT(1)
     FROM domains d
     INNER JOIN projects p ON d.project_id = p.id
     WHERE d.id=@p1 AND p.user_id=@p2`,
		id, userID,
	).Scan(&exists)
	if err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "domain not found"})
		return
	}

	rows, err := h.DB.Query(
		`SELECT TOP 50 id, ssl_expiry, domain_expiry, tls_version, issuer, issuer_dn, ip_address, status, nameservers, checked_at
     FROM scan_results WHERE domain_id=@p1 ORDER BY checked_at DESC`,
		id,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch history"})
		return
	}
	defer rows.Close()

	var result []gin.H
	for rows.Next() {
		var scanID int
		var sslExpiry, domainExpiry, checkedAt sql.NullTime
		var tlsVersion, issuer, issuerDN, ipAddress, status, nameservers sql.NullString
		if err := rows.Scan(&scanID, &sslExpiry, &domainExpiry, &tlsVersion, &issuer, &issuerDN, &ipAddress, &status, &nameservers, &checkedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse history"})
			return
		}
		result = append(result, gin.H{
			"id":            scanID,
			"ssl_expiry":    sslExpiry.Time,
			"domain_expiry": domainExpiry.Time,
			"tls_version":   tlsVersion.String,
			"issuer":        issuer.String,
			"issuer_dn":     issuerDN.String,
			"ip_address":    ipAddress.String,
			"status":        status.String,
			"nameservers":   nameservers.String,
			"checked_at":    checkedAt.Time,
		})
	}

	c.JSON(http.StatusOK, result)
}
