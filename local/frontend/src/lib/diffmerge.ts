import { createTwoFilesPatch } from 'diff';

const PLACEHOLDER = '\x00PLACEHOLDER\x00';

interface Hunk {
	oldStart: number;
	oldCount: number;
	newStart: number;
	newCount: number;
	lines: string[]; // each line starts with ' ', '+', or '-'
}

interface ParsedDiff {
	oldFile: string;
	newFile: string;
	hunks: Hunk[];
}

function stripFences(input: string): string {
	let s = input.trim();
	if (s.startsWith('```diff')) {
		s = s.slice('```diff'.length);
	} else if (s.startsWith('```')) {
		s = s.slice('```'.length);
	}
	if (s.endsWith('```')) {
		s = s.slice(0, -3);
	}
	return s.trim();
}

function parseDiff(raw: string): ParsedDiff {
	const lines = raw.split('\n');
	let oldFile = 'a/file';
	let newFile = 'b/file';
	const hunks: Hunk[] = [];
	let current: Hunk | null = null;
	let remainingOld = 0;
	let remainingNew = 0;

	for (const line of lines) {
		if (line.startsWith('--- ')) {
			oldFile = line.slice(4).trim();
		} else if (line.startsWith('+++ ')) {
			newFile = line.slice(4).trim();
		} else {
			const m = line.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/);
			if (m) {
				current = {
					oldStart: parseInt(m[1]),
					oldCount: m[2] !== undefined ? parseInt(m[2]) : 1,
					newStart: parseInt(m[3]),
					newCount: m[4] !== undefined ? parseInt(m[4]) : 1,
					lines: []
				};
				hunks.push(current);
				remainingOld = current.oldCount;
				remainingNew = current.newCount;
			} else if (
				current &&
				(line.startsWith(' ') || line.startsWith('+') || line.startsWith('-'))
			) {
				current.lines.push(line);
				if (line[0] === ' ') {
					remainingOld--;
					remainingNew--;
				} else if (line[0] === '-') {
					remainingOld--;
				} else if (line[0] === '+') {
					remainingNew--;
				}
			} else if (current && line === '' && (remainingOld > 0 || remainingNew > 0)) {
				// Empty line within a hunk = context line with empty content
				current.lines.push(' ');
				remainingOld--;
				remainingNew--;
			}
		}
	}

	return { oldFile, newFile, hunks };
}

/**
 * An edit in original-file coordinates.
 */
interface OrigEdit {
	origStart: number; // 1-based position in the original file
	origCount: number; // how many original lines are consumed
	newLines: string[]; // replacement content
	diffIndex: number; // which diff this came from
	addedLines: string[]; // lines that were + prefixed (for deduplication)
}

interface PriorEdit {
	origStart: number;
	origCount: number;
	newCount: number;
	outputToOrig: number[]; // for each output line, its original position or -1 if added
}

/**
 * Map a single post-previous position to its original position.
 * Returns the original position and whether it falls on a line added by a prior edit.
 */
function mapPostPosToOrig(
	postPos: number,
	priorEdits: PriorEdit[]
): { origPos: number; isAdded: boolean } {
	const sorted = [...priorEdits].sort((a, b) => a.origStart - b.origStart);
	let offset = 0;

	for (const edit of sorted) {
		const postEditStart = edit.origStart + offset;
		const postEditEnd = postEditStart + edit.newCount;

		if (postPos < postEditStart) {
			return { origPos: postPos - offset, isAdded: false };
		}

		if (postPos < postEditEnd) {
			const idx = postPos - postEditStart;
			const origPos = edit.outputToOrig[idx];
			if (origPos === -1) {
				return { origPos: edit.origStart, isAdded: true };
			}
			return { origPos, isAdded: false };
		}

		offset += edit.newCount - edit.origCount;
	}

	return { origPos: postPos - offset, isAdded: false };
}

/**
 * Map a range from post-previous coordinates back to original coordinates.
 * Uses per-position mapping to correctly handle ranges that span prior edits.
 */
function mapPostRangeToOrig(
	postStart: number,
	postCount: number,
	priorEdits: PriorEdit[]
): { origStart: number; origCount: number } {
	if (postCount === 0) {
		const { origPos } = mapPostPosToOrig(postStart, priorEdits);
		return { origStart: origPos, origCount: 0 };
	}

	let minOrig = Infinity;
	let maxOrig = -Infinity;

	for (let p = postStart; p < postStart + postCount; p++) {
		const { origPos, isAdded } = mapPostPosToOrig(p, priorEdits);
		if (!isAdded) {
			if (origPos < minOrig) minOrig = origPos;
			if (origPos > maxOrig) maxOrig = origPos;
		}
	}

	if (minOrig === Infinity) {
		// All positions are added lines — use start mapping
		const { origPos } = mapPostPosToOrig(postStart, priorEdits);
		return { origStart: origPos, origCount: 0 };
	}

	return { origStart: minOrig, origCount: maxOrig - minOrig + 1 };
}

/**
 * Compute Jaccard similarity between two sets of lines (after trimming whitespace).
 */
function computeSimilarity(a: string[], b: string[]): number {
	if (a.length === 0 || b.length === 0) return 0;
	const normalize = (s: string) => s.trim();
	const setA = new Set(a.map(normalize));
	const setB = new Set(b.map(normalize));
	let intersection = 0;
	for (const item of setA) {
		if (setB.has(item)) intersection++;
	}
	const union = new Set([...setA, ...setB]).size;
	return union === 0 ? 0 : intersection / union;
}

/**
 * Remove duplicate edits from different diffs that add substantially similar content
 * at different positions. The earlier diff's edit is removed (later diff wins).
 */
function deduplicateEdits(edits: OrigEdit[]): OrigEdit[] {
	const toRemove = new Set<number>();

	for (let i = 0; i < edits.length; i++) {
		if (toRemove.has(i)) continue;
		for (let j = i + 1; j < edits.length; j++) {
			if (toRemove.has(j)) continue;

			const a = edits[i];
			const b = edits[j];

			if (a.diffIndex === b.diffIndex) continue;
			if (a.addedLines.length < 3 || b.addedLines.length < 3) continue;

			const sim = computeSimilarity(a.addedLines, b.addedLines);
			if (sim > 0.5) {
				if (a.diffIndex < b.diffIndex) {
					toRemove.add(i);
				} else {
					toRemove.add(j);
				}
			}
		}
	}

	return edits.filter((_, i) => !toRemove.has(i));
}

/**
 * Merges multiple sequential unified diffs into a single cumulative diff.
 *
 * Each diff after the first is assumed to reference the post-previous-diff
 * state (the LLM applied diff N, re-read the file, then generated diff N+1).
 * Coordinates are mapped back to the original file for merging.
 */
export function mergeSequentialDiffs(diffs: string[]): string {
	if (diffs.length === 0) return '';

	const parsed = diffs.map((d) => parseDiff(stripFences(d)));

	// Collect context info and edits from all diffs
	const origContent = new Map<number, string>();
	const allEdits: OrigEdit[] = [];
	let maxOrigLine = 0;
	const priorEdits: PriorEdit[] = [];

	for (let di = 0; di < parsed.length; di++) {
		const diff = parsed[di];
		for (const hunk of diff.hunks) {
			const shouldMap = di > 0 && priorEdits.length > 0;

			// Collect original content and build outputToOrig in one pass
			let hunkOldCount = 0;
			const newLines: string[] = [];
			const addedLinesArr: string[] = [];
			const outputToOrig: number[] = [];
			let pos = hunk.oldStart;

			for (const line of hunk.lines) {
				const prefix = line[0];
				const content = line.slice(1);

				if (prefix === ' ') {
					hunkOldCount++;
					newLines.push(content);
					if (shouldMap) {
						const { origPos, isAdded } = mapPostPosToOrig(pos, priorEdits);
						outputToOrig.push(isAdded ? -1 : origPos);
						if (!isAdded && !origContent.has(origPos)) {
							origContent.set(origPos, content);
							if (origPos > maxOrigLine) maxOrigLine = origPos;
						}
					} else {
						outputToOrig.push(pos);
						origContent.set(pos, content);
						if (pos > maxOrigLine) maxOrigLine = pos;
					}
					pos++;
				} else if (prefix === '-') {
					hunkOldCount++;
					if (shouldMap) {
						const { origPos, isAdded } = mapPostPosToOrig(pos, priorEdits);
						if (!isAdded && !origContent.has(origPos)) {
							origContent.set(origPos, content);
							if (origPos > maxOrigLine) maxOrigLine = origPos;
						}
					} else {
						origContent.set(pos, content);
						if (pos > maxOrigLine) maxOrigLine = pos;
					}
					pos++;
				} else if (prefix === '+') {
					newLines.push(content);
					addedLinesArr.push(content);
					outputToOrig.push(-1);
				}
			}

			// Compute edit coordinates in original file
			let editOrigStart = hunk.oldStart;
			let editOrigCount = hunkOldCount;

			if (shouldMap) {
				const mapped = mapPostRangeToOrig(hunk.oldStart, hunkOldCount, priorEdits);
				editOrigStart = mapped.origStart;
				editOrigCount = mapped.origCount;
			}

			allEdits.push({
				origStart: editOrigStart,
				origCount: editOrigCount,
				newLines,
				diffIndex: di,
				addedLines: addedLinesArr
			});

			// Update priorEdits — remove any that overlap with the new edit
			for (let i = priorEdits.length - 1; i >= 0; i--) {
				const existing = priorEdits[i];
				const existEnd = existing.origStart + existing.origCount;
				const newEnd = editOrigStart + editOrigCount;
				if (existing.origStart < newEnd && existEnd > editOrigStart) {
					priorEdits.splice(i, 1);
				}
			}
			priorEdits.push({
				origStart: editOrigStart,
				origCount: editOrigCount,
				newCount: newLines.length,
				outputToOrig
			});
			priorEdits.sort((a, b) => a.origStart - b.origStart);
		}
	}

	// Resolve overlapping edits, then deduplicate similar content from different diffs
	const resolved = resolveEdits(allEdits);
	const deduped = deduplicateEdits(resolved);

	// Build original file
	const originalFile: string[] = [];
	for (let i = 1; i <= maxOrigLine; i++) {
		originalFile.push(origContent.has(i) ? origContent.get(i)! : PLACEHOLDER);
	}

	// Apply resolved edits (bottom to top)
	const finalFile = [...originalFile];
	const sortedEdits = [...deduped].sort((a, b) => b.origStart - a.origStart);

	for (const edit of sortedEdits) {
		const startIdx = edit.origStart - 1;
		while (finalFile.length < startIdx + edit.origCount) {
			finalFile.push(PLACEHOLDER);
		}
		finalFile.splice(startIdx, edit.origCount, ...edit.newLines);
	}

	// Generate diff
	const oldFile = parsed[0].oldFile;
	const newFile = parsed[parsed.length - 1].newFile;

	const originalText = originalFile.join('\n') + '\n';
	const finalText = finalFile.join('\n') + '\n';

	const patch = createTwoFilesPatch(oldFile, newFile, originalText, finalText, '', '', {
		context: 3
	});

	const patchLines = patch.split('\n');
	const startIdx = patchLines.findIndex((l) => l.startsWith('--- '));
	if (startIdx === -1) return '';

	const diffOutput = patchLines.slice(startIdx).join('\n');
	return rebuildWithoutPlaceholders(diffOutput);
}

/**
 * Resolve overlapping edits.
 *
 * For edits that overlap on shared context lines only, merge them.
 * For edits that truly conflict (modifying the same original lines differently),
 * the later diff (higher diffIndex) wins.
 */
function resolveEdits(edits: OrigEdit[]): OrigEdit[] {
	if (edits.length === 0) return [];

	// Sort by origStart, then by diffIndex
	const sorted = [...edits].sort((a, b) => {
		if (a.origStart !== b.origStart) return a.origStart - b.origStart;
		return a.diffIndex - b.diffIndex;
	});

	const result: OrigEdit[] = [sorted[0]];

	for (let i = 1; i < sorted.length; i++) {
		const edit = sorted[i];
		const last = result[result.length - 1];
		const lastEnd = last.origStart + last.origCount; // exclusive

		if (edit.origStart >= lastEnd) {
			// No overlap - just add
			result.push(edit);
		} else {
			// Overlap detected - merge or resolve
			const overlap = lastEnd - edit.origStart; // number of overlapping original lines

			if (edit.diffIndex >= last.diffIndex) {
				// Later edit has priority. Merge: keep the non-overlapping part
				// of 'last', then use 'edit' for the rest.
				//
				// The overlap consists of lines that are context in both edits.
				// Trim 'last' to not include the overlap, then append 'edit'.

				// Find where in 'last's newLines the overlap starts.
				// The overlap covers original positions [edit.origStart, lastEnd).
				// In 'last's output, we need to find the lines corresponding to
				// original positions before edit.origStart.
				const lastNewTrimmed = trimNewLinesFromEnd(last, overlap);

				// Merge: combine trimmed last + full edit
				const merged: OrigEdit = {
					origStart: last.origStart,
					origCount: last.origCount - overlap + edit.origCount,
					newLines: [...lastNewTrimmed, ...edit.newLines],
					diffIndex: edit.diffIndex,
					addedLines: [...last.addedLines, ...edit.addedLines]
				};
				result[result.length - 1] = merged;
			} else {
				// Earlier edit has priority - trim 'edit's start
				const editNewTrimmed = trimNewLinesFromStart(edit, overlap);
				const trimmed: OrigEdit = {
					origStart: lastEnd,
					origCount: edit.origCount - overlap,
					newLines: editNewTrimmed,
					diffIndex: edit.diffIndex,
					addedLines: edit.addedLines
				};
				if (trimmed.origCount > 0 || trimmed.newLines.length > 0) {
					result.push(trimmed);
				}
			}
		}
	}

	return result;
}

/**
 * From an edit's newLines, remove the lines corresponding to the last
 * `overlapCount` original lines. Original lines in newLines are those
 * that came from context (not added). We walk backwards through newLines
 * and remove until we've accounted for `overlapCount` original positions.
 *
 * Since we don't track which newLines are context vs added, we use
 * a heuristic: the last `overlapCount` lines of newLines that were
 * originally context lines. For simplicity, we trim the last `overlapCount`
 * items from newLines (this works when the overlap is all context).
 */
function trimNewLinesFromEnd(edit: OrigEdit, overlapCount: number): string[] {
	// The overlap is at the END of this edit's original region.
	// We need to remove from newLines the lines that correspond to
	// those overlapping original positions.
	//
	// Strategy: walk the edit's hunk backwards. For each original line
	// (context) in the overlap region, find and remove its corresponding
	// newLine entry.
	//
	// Simple approach: the last `overlapCount` items of newLines are the
	// context lines for the overlapping region (since they're at the end
	// of the hunk).
	if (overlapCount <= 0) return [...edit.newLines];
	return edit.newLines.slice(0, edit.newLines.length - overlapCount);
}

function trimNewLinesFromStart(edit: OrigEdit, overlapCount: number): string[] {
	if (overlapCount <= 0) return [...edit.newLines];
	return edit.newLines.slice(overlapCount);
}

/**
 * Remove placeholder lines from the unified diff output and rebuild hunks.
 */
function rebuildWithoutPlaceholders(diffText: string): string {
	const inputLines = diffText.split('\n');

	let headerIdx = 0;
	while (headerIdx < inputLines.length && !inputLines[headerIdx].startsWith('--- ')) {
		headerIdx++;
	}
	if (headerIdx >= inputLines.length) return '';

	const headerMinus = inputLines[headerIdx];
	const headerPlus = inputLines[headerIdx + 1];

	const rawHunks: { header: string; lines: string[] }[] = [];
	let currentHunk: { header: string; lines: string[] } | null = null;

	for (let i = headerIdx + 2; i < inputLines.length; i++) {
		const line = inputLines[i];
		if (line.match(/^@@ /)) {
			currentHunk = { header: line, lines: [] };
			rawHunks.push(currentHunk);
		} else if (currentHunk) {
			if (line.startsWith(' ') || line.startsWith('+') || line.startsWith('-')) {
				currentHunk.lines.push(line);
			}
		}
	}

	const processedHunks: {
		oldStart: number;
		oldCount: number;
		newStart: number;
		newCount: number;
		lines: string[];
	}[] = [];

	for (const hunk of rawHunks) {
		const m = hunk.header.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/);
		if (!m) continue;

		const hunkOldStart = parseInt(m[1]);
		const hunkNewStart = parseInt(m[3]);

		type LineInfo = {
			text: string;
			oldPos: number;
			newPos: number;
			isPlaceholder: boolean;
			type: 'context' | 'add' | 'del';
		};

		const lineInfos: LineInfo[] = [];
		let oldPos = hunkOldStart;
		let newPos = hunkNewStart;

		for (const line of hunk.lines) {
			const content = line.slice(1);
			const isPlaceholder = content === PLACEHOLDER;

			if (line[0] === ' ') {
				lineInfos.push({ text: line, oldPos, newPos, isPlaceholder, type: 'context' });
				oldPos++;
				newPos++;
			} else if (line[0] === '-') {
				lineInfos.push({ text: line, oldPos, newPos: 0, isPlaceholder, type: 'del' });
				oldPos++;
			} else if (line[0] === '+') {
				lineInfos.push({ text: line, oldPos: 0, newPos, isPlaceholder, type: 'add' });
				newPos++;
			}
		}

		// Filter out placeholder lines
		const filtered = lineInfos.filter((li) => !li.isPlaceholder);
		if (filtered.length === 0) continue;
		if (!filtered.some((li) => li.type === 'add' || li.type === 'del')) continue;

		// Split into sub-hunks at gaps where placeholders were removed
		const subHunks: LineInfo[][] = [];
		let currentSub: LineInfo[] = [];

		for (let i = 0; i < filtered.length; i++) {
			if (currentSub.length > 0) {
				const prev = filtered[i - 1];
				const curr = filtered[i];
				let gap = false;

				if ((curr.type === 'context' || curr.type === 'del') && curr.oldPos > 0) {
					const prevOldEnd = prev.type === 'context' || prev.type === 'del' ? prev.oldPos + 1 : 0;
					if (prevOldEnd > 0 && curr.oldPos > prevOldEnd) gap = true;
				}
				if (!gap && (curr.type === 'context' || curr.type === 'add') && curr.newPos > 0) {
					const prevNewEnd = prev.type === 'context' || prev.type === 'add' ? prev.newPos + 1 : 0;
					if (prevNewEnd > 0 && curr.newPos > prevNewEnd) gap = true;
				}

				if (gap) {
					if (currentSub.some((l) => l.type === 'add' || l.type === 'del')) {
						subHunks.push(currentSub);
					}
					currentSub = [];
				}
			}
			currentSub.push(filtered[i]);
		}
		if (currentSub.length > 0 && currentSub.some((l) => l.type === 'add' || l.type === 'del')) {
			subHunks.push(currentSub);
		}

		for (const sub of subHunks) {
			const trimmed = trimContext(sub, 3);
			if (trimmed.length === 0) continue;
			if (!trimmed.some((l) => l.type === 'add' || l.type === 'del')) continue;

			let subOldStart = 0,
				subNewStart = 0;
			for (const li of trimmed) {
				if (li.oldPos > 0 && subOldStart === 0) subOldStart = li.oldPos;
				if (li.newPos > 0 && subNewStart === 0) subNewStart = li.newPos;
				if (subOldStart > 0 && subNewStart > 0) break;
			}
			if (subOldStart === 0) subOldStart = subNewStart;
			if (subNewStart === 0) subNewStart = subOldStart;

			let oldCount = 0,
				newCount = 0;
			const hunkLines: string[] = [];
			for (const li of trimmed) {
				hunkLines.push(li.text);
				if (li.type === 'context') {
					oldCount++;
					newCount++;
				} else if (li.type === 'del') {
					oldCount++;
				} else if (li.type === 'add') {
					newCount++;
				}
			}

			processedHunks.push({
				oldStart: subOldStart,
				oldCount,
				newStart: subNewStart,
				newCount,
				lines: hunkLines
			});
		}
	}

	if (processedHunks.length === 0) return '';

	const outputLines: string[] = [headerMinus, headerPlus];
	for (const h of processedHunks) {
		outputLines.push(`@@ -${h.oldStart},${h.oldCount} +${h.newStart},${h.newCount} @@`);
		outputLines.push(...h.lines);
	}

	while (outputLines.length > 0 && outputLines[outputLines.length - 1] === '') {
		outputLines.pop();
	}

	return outputLines.join('\n');
}

function trimContext(
	lines: { text: string; oldPos: number; newPos: number; type: string }[],
	maxContext: number
): typeof lines {
	let firstChange = lines.findIndex((l) => l.type !== 'context');
	if (firstChange === -1) return [];
	const startTrim = Math.max(0, firstChange - maxContext);

	let lastChange = -1;
	for (let i = lines.length - 1; i >= 0; i--) {
		if (lines[i].type !== 'context') {
			lastChange = i;
			break;
		}
	}
	if (lastChange === -1) return [];
	const endTrim = Math.min(lines.length, lastChange + maxContext + 1);

	return lines.slice(startTrim, endTrim);
}
