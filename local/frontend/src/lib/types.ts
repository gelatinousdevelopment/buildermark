export interface Project {
	id: string;
	path: string;
	oldPaths: string;
	label: string;
	gitId: string;
	defaultBranch: string;
	remote: string;
	ignored: boolean;
	ignoreDiffPaths: string;
	ignoreDefaultDiffPaths: boolean;
	teamServerId: string;
	gitWorktreePaths: string;
}

export interface TeamServer {
	id: string;
	label: string;
	url: string;
	apiKey: string;
}

export interface AgentConversationSearchPath {
	agent: string;
	path: string;
	exists: boolean;
}

export interface LocalSettings {
	homePath: string;
	dbPath: string;
	listenAddr: string;
	conversationSearchPaths: AgentConversationSearchPath[];
	extraAgentHomes: string[];
	extraLocalUserEmails: string[];
	commitSortOrder: string;
}

export interface ProjectDetail {
	id: string;
	path: string;
	oldPaths: string;
	label: string;
	gitId: string;
	defaultBranch: string;
	currentBranch: string;
	remote: string;
	remoteUrl: string;
	localUser: string;
	localEmail: string;
	ignored: boolean;
	ignoreDiffPaths: string;
	ignoreDefaultDiffPaths: boolean;
	teamServerId: string;
	gitWorktreePaths: string;
	agents: string[];
	conversationPagination: ConversationPagination;
	conversations: ConversationWithRatings[];
}

export interface ConversationPagination {
	page: number;
	pageSize: number;
	total: number;
	totalPages: number;
}

export interface ConversationWithRatings {
	id: string;
	agent: string;
	title: string;
	parentConversationId: string;
	hidden: boolean;
	lastMessageTimestamp: number;
	userPromptCount: number;
	ratings: Rating[];
	filesEdited: string[];
}

export interface ConversationBatchDetail {
	conversationId: string;
	userMessages: MessageRead[];
	ratings: Rating[];
}

export interface Conversation {
	id: string;
	projectId: string;
	agent: string;
	title: string;
	hidden: boolean;
	parentConversationId: string;
}

export interface ConversationRef {
	id: string;
	title: string;
}

export interface ConversationDetail {
	id: string;
	projectId: string;
	agent: string;
	title: string;
	hidden: boolean;
	parentConversationId: string;
	childConversations: ConversationRef[];
	messages: MessageRead[];
	ratings: Rating[];
}

export interface MessageRead {
	id: string;
	timestamp: number;
	conversationId: string;
	role: string;
	model?: string;
	content: string;
	rawJson: string;
}

export interface Rating {
	id: string;
	conversationId: string;
	tempConversationId: string;
	rating: number;
	note: string;
	analysis: string;
	createdAt: number;
	matchedTimestamp?: number;
}

export interface ProjectCommitCoverageResponse {
	branch: string;
	branches: string[];
	currentUser: string;
	currentEmail: string;
	summary: ProjectCommitSummary;
	commits: ProjectCommitCoverage[];
}

export interface UserInfo {
	name: string;
	email: string;
}

export interface ProjectCommitPageResponse {
	branch: string;
	branches: string[];
	users: UserInfo[];
	userFilter: string;
	agents: string[];
	agentFilter: string;
	currentUser: string;
	currentEmail: string;
	extraLocalUserEmails?: string[];
	project: Project;
	refresh?: CommitRefreshState;
	summary: ProjectCommitSummary;
	dailySummary?: DailyCommitSummary[];
	pagination: ProjectCommitPagination;
	commits: ProjectCommitCoverage[];
}

export interface CommitRefreshState {
	state: 'idle' | 'queued' | 'running' | 'failed' | string;
	isStale: boolean;
	lastStartedAt: number;
	lastFinishedAt: number;
	lastDurationMs: number;
	lastError: string;
}

export interface DailyCommitSummary {
	date: string;
	linesTotal: number;
	linesFromAgent: number;
	linePercent: number;
	agentSegments?: AgentCoverageSegment[];
	commits: DailyCommitRef[];
}

export interface DailyCommitRef {
	commitHash: string;
	subject: string;
	projectId: string;
}

export interface ProjectCommitPagination {
	page: number;
	pageSize: number;
	total: number;
	totalPages: number;
}

export interface ProjectCommitDetailResponse {
	branch: string;
	branches: string[];
	commitUrl: string;
	commit: ProjectCommitCoverage;
	attribution: ProjectCommitAttribution;
	diff: string;
	files: ProjectCommitFileCoverage[];
	messages: ProjectCommitContributionMessage[];
}

export interface ProjectCommitAttribution {
	exactMatchedLines: number;
	fallbackMatchedLines: number;
	hasFallbackAttribution: boolean;
	matchedMessagesCount: number;
}

export interface ProjectCommitFileCoverage {
	path: string;
	added: number;
	removed: number;
	ignored: boolean;
	moved: boolean;
	movedFrom: string;
	copiedFromAgent: boolean;
	linesTotal: number;
	linesFromAgent: number;
	linePercent: number;
	agentSegments?: AgentCoverageSegment[];
}

export interface ProjectCommitContributionMessage {
	id: string;
	timestamp: number;
	conversationId: string;
	conversationTitle: string;
	agent: string;
	model: string;
	content: string;
	linesMatched: number;
	charsMatched: number;
}

export interface AgentCoverageSegment {
	agent: string;
	linesFromAgent: number;
	charsFromAgent: number;
	linePercent: number;
}

export interface ProjectCommitSummary {
	commitCount: number;
	linesTotal: number;
	linesFromAgent: number;
	linePercent: number;
	charsTotal: number;
	charsFromAgent: number;
	characterPercent: number;
	agentSegments?: AgentCoverageSegment[];
}

export interface IngestCommitsResponse {
	ingested: number;
	reachedRoot: boolean;
}

export interface CommitIngestionStatusResponse {
	ingestedCount: number;
	totalGitCommits: number;
	estimatedTotalCommits?: number;
	reachedRoot: boolean;
	state?: 'idle' | 'queued' | 'running' | 'failed' | string;
	lastStartedAt?: number;
	lastFinishedAt?: number;
	lastDurationMs?: number;
	lastError?: string;
}

export interface ImportableProject {
	path: string;
	label: string;
	projectId?: string;
	tracked: boolean;
}

export interface ProjectTrackingOption {
	path: string;
	label: string;
	projectId?: string;
	tracked: boolean;
	importable: boolean;
	missingOnDisk: boolean;
}

export interface DiscoverImportableProjectsResponse {
	projects: ImportableProject[];
	since: string;
}

export interface ImportProjectsResponse {
	started: boolean;
}

export interface ProjectCommitCoverage {
	workingCopy?: boolean;
	projectId: string;
	projectLabel: string;
	projectPath: string;
	projectGitId: string;
	commitHash: string;
	subject: string;
	userName?: string;
	userEmail?: string;
	authoredAtUnixMs: number;
	linesTotal: number;
	linesFromAgent: number;
	linePercent: number;
	charsTotal: number;
	charsFromAgent: number;
	characterPercent: number;
	linesAdded: number;
	linesRemoved: number;
	agentSegments?: AgentCoverageSegment[];
	overrideLinePercent?: number | null;
}

export interface ProjectSearchMatch {
	project: Project;
	conversationMatches: number;
	commitMatches: number;
}

export interface CommitConversationLinks {
	commitToConversations: Record<string, string[]>;
	conversationToCommits: Record<string, string[]>;
}
