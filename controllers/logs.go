package controllers

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	appLogPath   = "./logs/app.log"
	errorLogPath = "./logs/error.log"
)

// readLogEntries reads a zap JSON-lines log file into generic maps. Log
// fields vary per call site (extra zap.Field key/values), so a fixed struct
// would silently drop data — a map keeps every field the app ever logs.
func readLogEntries(path string) []map[string]any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var entries []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

// AdminListLogsController serves a paginated, filterable view over the
// application's log files (./logs/app.log, ./logs/error.log), newest first.
func AdminListLogsController(c *gin.Context) {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	level := c.Query("level") // INFO, WARN, ERROR
	search := c.Query("search")

	var entries []map[string]any
	switch c.DefaultQuery("source", "all") {
	case "app":
		entries = readLogEntries(appLogPath)
	case "error":
		entries = readLogEntries(errorLogPath)
	default:
		entries = append(readLogEntries(appLogPath), readLogEntries(errorLogPath)...)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		ti, _ := entries[i]["ts"].(string)
		tj, _ := entries[j]["ts"].(string)
		return ti > tj // ISO8601 strings sort lexicographically = chronologically
	})

	filtered := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if level != "" {
			lvl, _ := e["level"].(string)
			if !strings.EqualFold(lvl, level) {
				continue
			}
		}
		if search != "" {
			msg, _ := e["msg"].(string)
			if !strings.Contains(strings.ToLower(msg), strings.ToLower(search)) {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	total := len(filtered)
	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	totalPages := 0
	if limit > 0 {
		totalPages = (total + limit - 1) / limit
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data":    filtered[start:end],
		"meta": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": totalPages,
		},
	})
}
