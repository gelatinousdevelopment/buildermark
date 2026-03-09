package handler

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

const (
	workingCopyCommitHash        = "working-copy"
	commitWindowLookaheadMs      = int64(5 * 60 * 1000)
	maxCommitsPerProject         = 200
	commitsPageSize              = 20
	currentCommitCoverageVersion = 10
	maxFormattingWindowLines     = 5
)

var defaultMessageWindowMs = func() int64 {
	if v := os.Getenv("BUILDERMARK_LOCAL_MESSAGE_WINDOW_HOURS"); v != "" {
		if hours, err := strconv.ParseInt(v, 10, 64); err == nil && hours > 0 {
			log.Printf("using custom message window: %d hours", hours)
			return hours * 60 * 60 * 1000
		}
	}
	return int64(7 * 24 * 60 * 60 * 1000) // 7 days
}()

type commitDetailCacheEntry struct {
	files         []commitFileCoverage
	agentSegments []agentCoverageSegment
	contribs      []commitContributionMessage
	matchedLines  int
	exactMatched  int
	fallbackLines int
	totalLines    int
	fetchedAt     time.Time
}

type projectCommitsResponse struct {
	Branch       string                  `json:"branch"`
	Branches     []string                `json:"branches"`
	CurrentUser  string                  `json:"currentUser"`
	CurrentEmail string                  `json:"currentEmail"`
	Summary      projectCommitSummary    `json:"summary"`
	Commits      []projectCommitCoverage `json:"commits"`
}

type agentCoverageSegment struct {
	Agent          string  `json:"agent"`
	LinesFromAgent int     `json:"linesFromAgent"`
	LinePercent    float64 `json:"linePercent"`
}

type projectCommitSummary struct {
	CommitCount    int                    `json:"commitCount"`
	LinesTotal     int                    `json:"linesTotal"`
	LinesFromAgent int                    `json:"linesFromAgent"`
	LinePercent    float64                `json:"linePercent"`
	AgentSegments  []agentCoverageSegment `json:"agentSegments,omitempty"`
}

type projectCommitCoverage struct {
	WorkingCopy         bool                   `json:"workingCopy"`
	ProjectID           string                 `json:"projectId"`
	ProjectLabel        string                 `json:"projectLabel"`
	ProjectPath         string                 `json:"projectPath"`
	ProjectGitID        string                 `json:"projectGitId"`
	CommitHash          string                 `json:"commitHash"`
	Subject             string                 `json:"subject"`
	UserName            string                 `json:"userName,omitempty"`
	UserEmail           string                 `json:"userEmail,omitempty"`
	AuthoredAtUnixMs    int64                  `json:"authoredAtUnixMs"`
	LinesTotal          int                    `json:"linesTotal"`
	LinesFromAgent      int                    `json:"linesFromAgent"`
	LinePercent         float64                `json:"linePercent"`
	LinesAdded          int                    `json:"linesAdded"`
	LinesRemoved        int                    `json:"linesRemoved"`
	AgentSegments       []agentCoverageSegment `json:"agentSegments,omitempty"`
	OverrideLinePercent *float64               `json:"overrideLinePercent,omitempty"`
	NeedsParent         bool                   `json:"needsParent,omitempty"`
}

type projectCommitDetailResponse struct {
	Branch      string                      `json:"branch"`
	Branches    []string                    `json:"branches"`
	CommitURL   string                      `json:"commitUrl"`
	Commit      projectCommitCoverage       `json:"commit"`
	Attribution commitAttribution           `json:"attribution"`
	Diff        string                      `json:"diff"`
	Files       []commitFileCoverage        `json:"files"`
	Messages    []commitContributionMessage `json:"messages"`
}

type commitAttribution struct {
	ExactMatchedLines    int  `json:"exactMatchedLines"`
	FallbackMatchedLines int  `json:"fallbackMatchedLines"`
	HasFallback          bool `json:"hasFallbackAttribution"`
	MatchedMessagesCount int  `json:"matchedMessagesCount"`
}

type projectCommitPageResponse struct {
	Branch               string                  `json:"branch"`
	Branches             []string                `json:"branches"`
	Users                []db.UserInfo           `json:"users"`
	UserFilter           string                  `json:"userFilter"`
	Agents               []string                `json:"agents"`
	AgentFilter          string                  `json:"agentFilter"`
	CurrentUser          string                  `json:"currentUser"`
	CurrentEmail         string                  `json:"currentEmail"`
	ExtraLocalUserEmails []string                `json:"extraLocalUserEmails"`
	Project              db.Project              `json:"project"`
	Refresh              commitRefreshState      `json:"refresh"`
	Summary              projectCommitSummary    `json:"summary"`
	DailySummary         []dailyCommitSummary    `json:"dailySummary"`
	Pagination           projectCommitPagination `json:"pagination"`
	Commits              []projectCommitCoverage `json:"commits"`
}

type commitRefreshState struct {
	State          string `json:"state"`
	IsStale        bool   `json:"isStale"`
	LastStartedAt  int64  `json:"lastStartedAt"`
	LastFinishedAt int64  `json:"lastFinishedAt"`
	LastDurationMs int64  `json:"lastDurationMs"`
	LastError      string `json:"lastError"`
}

type dailyCommitSummary struct {
	Date           string                 `json:"date"`
	LinesTotal     int                    `json:"linesTotal"`
	LinesFromAgent int                    `json:"linesFromAgent"`
	LinePercent    float64                `json:"linePercent"`
	AgentSegments  []agentCoverageSegment `json:"agentSegments,omitempty"`
	Commits        []dailyCommitRef       `json:"commits"`
}

type dailyCommitRef struct {
	CommitHash string `json:"commitHash"`
	Subject    string `json:"subject"`
	ProjectID  string `json:"projectId"`
}

type projectCommitPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

type commitContributionMessage struct {
	ID                string `json:"id"`
	Timestamp         int64  `json:"timestamp"`
	ConversationID    string `json:"conversationId"`
	ConversationTitle string `json:"conversationTitle"`
	Agent             string `json:"agent"`
	Model             string `json:"model"`
	Content           string `json:"content"`
	LinesMatched      int    `json:"linesMatched"`
}

type commitFileCoverage struct {
	Path            string                 `json:"path"`
	Added           int                    `json:"added"`
	Removed         int                    `json:"removed"`
	Ignored         bool                   `json:"ignored"`
	Moved           bool                   `json:"moved"`
	MovedFrom       string                 `json:"movedFrom"`
	CopiedFromAgent bool                   `json:"copiedFromAgent"`
	LinesTotal      int                    `json:"linesTotal"`
	LinesFromAgent  int                    `json:"linesFromAgent"`
	LinePercent     float64                `json:"linePercent"`
	AgentSegments   []agentCoverageSegment `json:"agentSegments,omitempty"`
}

type gitIdentity struct {
	Name  string
	Email string
}

type gitCommit struct {
	Hash          string
	Subject       string
	UserName      string
	UserEmail     string
	TimestampUnix int64
}

type projectGroup struct {
	GitID    string
	Projects []db.Project
}

type messageDiff struct {
	ID                string
	Timestamp         int64
	ConversationID    string
	ConversationTitle string
	Agent             string
	Model             string
	Content           string
	Tokens            []diffToken
}

type diffToken struct {
	Path         string
	Sign         byte
	Norm         string
	Key          string
	Attributable bool
}

type tokenSource struct {
	msgIdx   int
	tokenPos int
}

// agentStats tracks per-agent line counts for commit coverage.
type agentStats struct {
	lines int
}

// CommitDetailResult holds all computed detail data for a single commit.
type CommitDetailResult struct {
	Commit        db.Commit
	Files         []commitFileCoverage
	AgentSegments []agentCoverageSegment
	ContribMsgs   []commitContributionMessage
	ExactMatched  int
	FallbackLines int
	ByAgent       map[string]agentStats
	ConvIDs       []string
}

// serializeDetail marshals the detail fields onto result.Commit's detail columns.
func serializeDetail(result *CommitDetailResult) {
	if len(result.Files) > 0 {
		b, _ := json.Marshal(result.Files)
		result.Commit.DetailFiles = string(b)
	}
	if len(result.ContribMsgs) > 0 {
		b, _ := json.Marshal(result.ContribMsgs)
		result.Commit.DetailMessages = string(b)
	}
	if len(result.AgentSegments) > 0 {
		b, _ := json.Marshal(result.AgentSegments)
		result.Commit.DetailAgentSegments = string(b)
	}
	result.Commit.DetailExactMatched = result.ExactMatched
	result.Commit.DetailFallbackLines = result.FallbackLines
}
