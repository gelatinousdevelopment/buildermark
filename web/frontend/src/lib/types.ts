export interface Project {
	id: string;
	path: string;
	label: string;
	gitId: string;
	ignored: boolean;
	ignoreDiffPaths: string;
	ignoreDefaultDiffPaths: boolean;
}

export interface ProjectDetail {
	id: string;
	path: string;
	label: string;
	gitId: string;
	ignored: boolean;
	ignoreDiffPaths: string;
	ignoreDefaultDiffPaths: boolean;
	conversations: ConversationWithRatings[];
}

export interface ConversationWithRatings {
	id: string;
	agent: string;
	title: string;
	ratings: Rating[];
}

export interface Conversation {
	id: string;
	projectId: string;
	agent: string;
	title: string;
}

export interface ConversationDetail {
	id: string;
	projectId: string;
	agent: string;
	title: string;
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
	rating: number;
	note: string;
	analysis: string;
	createdAt: string;
}

export interface ProjectCommitCoverageResponse {
	branch: string;
	currentUser: string;
	currentEmail: string;
	summary: ProjectCommitSummary;
	commits: ProjectCommitCoverage[];
}

export interface ProjectCommitPageResponse {
	branch: string;
	currentUser: string;
	currentEmail: string;
	project: Project;
	summary: ProjectCommitSummary;
	pagination: ProjectCommitPagination;
	commits: ProjectCommitCoverage[];
}

export interface ProjectCommitPagination {
	page: number;
	pageSize: number;
	total: number;
	totalPages: number;
}

export interface ProjectCommitDetailResponse {
	branch: string;
	commit: ProjectCommitCoverage;
	diff: string;
	files: ProjectCommitFileCoverage[];
	messages: ProjectCommitContributionMessage[];
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
}

export interface ProjectCommitContributionMessage {
	id: string;
	timestamp: number;
	conversationId: string;
	conversationTitle: string;
	model: string;
	content: string;
	linesMatched: number;
	charsMatched: number;
}

export interface ProjectCommitSummary {
	commitCount: number;
	linesTotal: number;
	linesFromAgent: number;
	linePercent: number;
	charsTotal: number;
	charsFromAgent: number;
	characterPercent: number;
}

export interface IngestCommitsResponse {
	ingested: number;
	reachedRoot: boolean;
}

export interface CommitIngestionStatusResponse {
	ingestedCount: number;
	totalGitCommits: number;
	reachedRoot: boolean;
}

export interface ProjectCommitCoverage {
	workingCopy?: boolean;
	projectId: string;
	projectLabel: string;
	projectPath: string;
	projectGitId: string;
	commitHash: string;
	subject: string;
	authoredAtUnixMs: number;
	linesTotal: number;
	linesFromAgent: number;
	linePercent: number;
	charsTotal: number;
	charsFromAgent: number;
	characterPercent: number;
}
