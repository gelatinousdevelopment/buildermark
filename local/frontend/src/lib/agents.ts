const KNOWN_AGENT_VALUES = [
	'claude',
	'claude_cloud',
	'codex',
	'codex_cloud',
	'cursor',
	'gemini'
] as const;

export const KNOWN_AGENTS: string[] = [...KNOWN_AGENT_VALUES];

export type KnownAgent = (typeof KNOWN_AGENT_VALUES)[number];

export interface KnownAgentInfo {
	supportsResumeFromBuildermark: boolean;
	resumeCommandTemplate: string | null;
	resumePrompt: string | null;
}

export const KNOWN_AGENT_INFO: Record<KnownAgent, KnownAgentInfo> = {
	claude: {
		supportsResumeFromBuildermark: true,
		resumeCommandTemplate: 'claude -r {{sessionId}} {{resumePrompt}}',
		resumePrompt: '/rate-buildermark'
	},
	claude_cloud: {
		supportsResumeFromBuildermark: false,
		resumeCommandTemplate: null,
		resumePrompt: null
	},
	codex: {
		supportsResumeFromBuildermark: true,
		resumeCommandTemplate: 'codex resume {{sessionId}} -a on-request {{resumePrompt}}',
		resumePrompt: '$rate-buildermark'
	},
	codex_cloud: {
		supportsResumeFromBuildermark: false,
		resumeCommandTemplate: null,
		resumePrompt: null
	},
	cursor: {
		supportsResumeFromBuildermark: false,
		resumeCommandTemplate: null,
		resumePrompt: null
	},
	gemini: {
		supportsResumeFromBuildermark: true,
		resumeCommandTemplate: 'gemini -r {{sessionId}} {{resumePrompt}}',
		resumePrompt: '/rate-buildermark'
	}
};

function shellQuote(value: string): string {
	return `'${value.replaceAll("'", `'"'"'`)}'`;
}

export function buildResumeCommand(
	agent: string,
	sessionId: string,
	projectPath?: string | null
): string | null {
	const info = KNOWN_AGENT_INFO[agent as KnownAgent];
	if (!info?.supportsResumeFromBuildermark || !info.resumeCommandTemplate || !info.resumePrompt) {
		return null;
	}

	const command = info.resumeCommandTemplate
		.replace('{{sessionId}}', shellQuote(sessionId))
		.replace('{{resumePrompt}}', shellQuote(info.resumePrompt));

	const trimmedProjectPath = projectPath?.trim() ?? '';
	if (!trimmedProjectPath) {
		return command;
	}

	return `cd ${shellQuote(trimmedProjectPath)} && ${command}`;
}
