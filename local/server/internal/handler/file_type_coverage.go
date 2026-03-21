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
		agentLines  map[string]float64 // agent name -> effective lines
	}
	byExt := make(map[string]*extData)

	for _, row := range detailFilesJSON {
		var files []commitFileCoverage
		if err := json.Unmarshal([]byte(row.DetailFiles), &files); err != nil {
			continue
		}
		override := parseOverrideAgentPercents(row.OverrideAgentPercents, "")
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
					agentLines:  make(map[string]float64),
				}
				byExt[ext] = data
			}
			data.uniqueFiles[f.Path] = struct{}{}
			data.totalLines += f.LinesTotal
			if len(override) > 0 {
				for agent, pct := range override {
					if pct <= 0 {
						continue
					}
					data.agentLines[agent] += float64(f.LinesTotal) * float64(pct) / 100
				}
				continue
			}
			for _, seg := range f.AgentSegments {
				data.agentLines[seg.Agent] += float64(seg.LinesFromAgent)
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

		roundedLinesByAgent := roundFloatAgentLines(data.agentLines, data.totalLines)
		agentNames := make([]string, 0, len(data.agentLines))
		for agent := range data.agentLines {
			agentNames = append(agentNames, agent)
		}
		sort.Strings(agentNames)

		agentLinesSum := 0.0
		for _, agent := range agentNames {
			lines := roundedLinesByAgent[agent]
			pct := 0.0
			if data.totalLines > 0 {
				pct = data.agentLines[agent] / float64(data.totalLines) * 100
			}
			row.AgentSegments = append(row.AgentSegments, agentCoverageSegment{
				Agent:          agent,
				LinesFromAgent: lines,
				LinePercent:    pct,
			})
			agentLinesSum += data.agentLines[agent]
		}
		if data.totalLines > 0 {
			manualPercent := 100 - (agentLinesSum/float64(data.totalLines))*100
			if manualPercent < 0 {
				manualPercent = 0
			}
			row.ManualPercent = manualPercent
		}

		rows = append(rows, row)
	}

	// Sort by totalFiles descending.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].TotalFiles > rows[j].TotalFiles
	})

	writeSuccess(w, http.StatusOK, rows)
}

func roundFloatAgentLines(agentLines map[string]float64, maxTotal int) map[string]int {
	if len(agentLines) == 0 || maxTotal <= 0 {
		return nil
	}

	type allocation struct {
		agent     string
		lines     int
		remainder float64
	}

	agents := make([]string, 0, len(agentLines))
	for agent, lines := range agentLines {
		if lines > 0 {
			agents = append(agents, agent)
		}
	}
	if len(agents) == 0 {
		return nil
	}
	sort.Strings(agents)

	allocations := make([]allocation, 0, len(agents))
	totalExact := 0.0
	baseSum := 0
	for _, agent := range agents {
		exact := agentLines[agent]
		lines := int(exact)
		allocations = append(allocations, allocation{
			agent:     agent,
			lines:     lines,
			remainder: exact - float64(lines),
		})
		totalExact += exact
		baseSum += lines
	}

	target := int(totalExact + 0.5)
	if target < 0 {
		target = 0
	}
	if target > maxTotal {
		target = maxTotal
	}

	sort.SliceStable(allocations, func(i, j int) bool {
		if allocations[i].remainder != allocations[j].remainder {
			return allocations[i].remainder > allocations[j].remainder
		}
		return allocations[i].agent < allocations[j].agent
	})

	for i := 0; i < target-baseSum && i < len(allocations); i++ {
		allocations[i].lines++
	}

	sort.SliceStable(allocations, func(i, j int) bool {
		return allocations[i].agent < allocations[j].agent
	})

	out := make(map[string]int, len(allocations))
	for _, alloc := range allocations {
		out[alloc.agent] = alloc.lines
	}
	return out
}
