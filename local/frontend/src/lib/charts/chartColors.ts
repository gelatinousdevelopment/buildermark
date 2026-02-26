export const DEFAULT_COLORS = [
	'#e07b39',
	'#10b981',
	'#3b82f6',
	'#8b5cf6',
	'#ec4899',
	'#f59e0b',
	'#06b6d4',
	'#ef4444',
	'#84cc16',
	'#6366f1',
	'#14b8a6',
	'#f97316',
	'#a855f7',
	'#22d3ee',
	'#fb7185',
	'#fbbf24',
	'#34d399',
	'#818cf8',
	'#f472b6',
	'#2dd4bf'
];

export const MANUAL_COLOR = 'var(--agent-color-manual, #656565)';

/** Convert an agent name to its CSS variable reference with a fallback. */
export function agentColor(name: string, index: number): string {
	const varName = `--agent-color-${name.replace(/[^a-zA-Z0-9]/g, '-').toLowerCase()}`;
	return `var(${varName}, ${DEFAULT_COLORS[index % DEFAULT_COLORS.length]})`;
}
