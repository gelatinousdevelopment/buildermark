package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

type fileTypeCoverageRow struct {
	Extension     string                 `json:"extension"`
	TotalFiles    int                    `json:"totalFiles"`
	TotalLines    int                    `json:"totalLines"`
	AgentSegments []agentCoverageSegment `json:"agentSegments"`
	ManualPercent float64                `json:"manualPercent"`
}

func (s *Server) handleGetFileTypeCoverage(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "projectId is required")
		return
	}

	q := r.URL.Query()
	startMs, _ := strconv.ParseInt(q.Get("start"), 10, 64)
	endMs, _ := strconv.ParseInt(q.Get("end"), 10, 64)
	if startMs <= 0 || endMs <= 0 || endMs <= startMs {
		writeError(w, http.StatusBadRequest, "start and end (ms) are required and end must be after start")
		return
	}

	startSec := startMs / 1000
	endSec := endMs / 1000

	detailFilesJSON, err := db.GetCommitDetailFilesInRange(r.Context(), s.DB, projectID, startSec, endSec)
	if err != nil {
		log.Printf("error getting commit detail files: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get file type coverage")
		return
	}

	// Aggregate by file extension.
	type extData struct {
		uniqueFiles map[string]struct{}
		totalLines  int
		agentLines  map[string]int // agent name -> lines
	}
	byExt := make(map[string]*extData)

	for _, raw := range detailFilesJSON {
		var files []commitFileCoverage
		if err := json.Unmarshal([]byte(raw), &files); err != nil {
			continue
		}
		for _, f := range files {
			if f.Ignored || f.LinesTotal == 0 {
				continue
			}
			ext := filepath.Ext(f.Path)
			if ext == "" {
				ext = filepath.Base(f.Path)
			} else {
				ext = strings.ToLower(ext)
			}

			data, ok := byExt[ext]
			if !ok {
				data = &extData{
					uniqueFiles: make(map[string]struct{}),
					agentLines:  make(map[string]int),
				}
				byExt[ext] = data
			}
			data.uniqueFiles[f.Path] = struct{}{}
			data.totalLines += f.LinesTotal
			for _, seg := range f.AgentSegments {
				data.agentLines[seg.Agent] += seg.LinesFromAgent
			}
		}
	}

	// Build response rows.
	rows := make([]fileTypeCoverageRow, 0, len(byExt))
	for ext, data := range byExt {
		row := fileTypeCoverageRow{
			Extension:  ext,
			TotalFiles: len(data.uniqueFiles),
			TotalLines: data.totalLines,
		}

		var agentLinesSum int
		for agent, lines := range data.agentLines {
			pct := 0.0
			if data.totalLines > 0 {
				pct = float64(lines) / float64(data.totalLines) * 100
			}
			row.AgentSegments = append(row.AgentSegments, agentCoverageSegment{
				Agent:          agent,
				LinesFromAgent: lines,
				LinePercent:    pct,
			})
			agentLinesSum += lines
		}

		// Sort agent segments alphabetically for consistent ordering across rows.
		sort.Slice(row.AgentSegments, func(i, j int) bool {
			return row.AgentSegments[i].Agent < row.AgentSegments[j].Agent
		})

		manualLines := data.totalLines - agentLinesSum
		if manualLines < 0 {
			manualLines = 0
		}
		if data.totalLines > 0 {
			row.ManualPercent = float64(manualLines) / float64(data.totalLines) * 100
		}

		rows = append(rows, row)
	}

	// Sort by totalFiles descending.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].TotalFiles > rows[j].TotalFiles
	})

	writeSuccess(w, http.StatusOK, rows)
}
