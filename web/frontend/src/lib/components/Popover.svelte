<script lang="ts">
	import type { Snippet } from 'svelte';

	type Position = 'leading' | 'trailing' | 'above' | 'below';

	interface Props {
		position?: Position;
		width?: string;
		padding?: string;
		children: Snippet;
		popover: Snippet;
	}

	let { position = 'leading', width, padding = '1rem', children, popover }: Props = $props();

	let visible = $state(false);
	let wrapperEl: HTMLDivElement | undefined = $state();
	let popoverEl: HTMLDivElement | undefined = $state();
	let offsetX = $state(0);
	let offsetY = $state(0);
	let resolvedPosition: Position = $state('leading');
	let maxHeight = $state('0px');
	let positioned = $state(false);

	/** Safe area inset (px) from viewport edges. */
	const MARGIN = 8; // 0.5rem
	const GAP = 6;

	const ALL_POSITIONS: Position[] = ['leading', 'trailing', 'above', 'below'];

	function baseTransform(pos: Position): string {
		if (pos === 'above' || pos === 'below') return 'translateX(-50%)';
		return 'translateY(-50%)';
	}

	/** Max height the popover can be for a given position without exceeding the safe area. */
	function availableHeight(wrapRect: DOMRect, pos: Position): number {
		if (pos === 'above') return wrapRect.top - GAP - MARGIN;
		if (pos === 'below') return window.innerHeight - MARGIN - (wrapRect.bottom + GAP);
		return window.innerHeight - 2 * MARGIN;
	}

	/** Compute where the popover rect would land for a given position, using constrained size. */
	function computeRect(wrapRect: DOMRect, pos: Position, natW: number, natH: number): DOMRect {
		const mh = availableHeight(wrapRect, pos);
		const h = Math.min(natH, Math.max(0, mh));
		const w = natW;
		let top: number, left: number;
		if (pos === 'above') {
			top = wrapRect.top - GAP - h;
			left = wrapRect.left + wrapRect.width / 2 - w / 2;
		} else if (pos === 'below') {
			top = wrapRect.bottom + GAP;
			left = wrapRect.left + wrapRect.width / 2 - w / 2;
		} else if (pos === 'leading') {
			top = wrapRect.top + wrapRect.height / 2 - h / 2;
			left = wrapRect.left - GAP - w;
		} else {
			top = wrapRect.top + wrapRect.height / 2 - h / 2;
			left = wrapRect.right + GAP;
		}
		return new DOMRect(left, top, w, h);
	}

	/** How much a rect overflows the safe area. Returns 0 if fully visible. */
	function overflow(rect: DOMRect): number {
		let total = 0;
		if (rect.left < MARGIN) total += MARGIN - rect.left;
		if (rect.right > window.innerWidth - MARGIN) total += rect.right - (window.innerWidth - MARGIN);
		if (rect.top < MARGIN) total += MARGIN - rect.top;
		if (rect.bottom > window.innerHeight - MARGIN)
			total += rect.bottom - (window.innerHeight - MARGIN);
		return total;
	}

	/** Shift needed on the secondary axis to keep the rect in the safe area. */
	function secondaryShift(rect: DOMRect, pos: Position): { dx: number; dy: number } {
		if (pos === 'leading' || pos === 'trailing') {
			let dy = 0;
			if (rect.top < MARGIN) dy = MARGIN - rect.top;
			else if (rect.bottom > window.innerHeight - MARGIN)
				dy = window.innerHeight - MARGIN - rect.bottom;
			return { dx: 0, dy };
		}
		let dx = 0;
		if (rect.left < MARGIN) dx = MARGIN - rect.left;
		else if (rect.right > window.innerWidth - MARGIN) dx = window.innerWidth - MARGIN - rect.right;
		return { dx, dy: 0 };
	}

	/** Try a position: returns fit result with shift and available height. */
	function tryPosition(
		wrapRect: DOMRect,
		pos: Position,
		natW: number,
		natH: number
	): { fits: boolean; shift: { dx: number; dy: number }; mh: number } {
		const mh = availableHeight(wrapRect, pos);
		if (mh <= 0) return { fits: false, shift: { dx: 0, dy: 0 }, mh: 0 };
		const rect = computeRect(wrapRect, pos, natW, natH);
		if (overflow(rect) === 0) return { fits: true, shift: { dx: 0, dy: 0 }, mh };
		const shift = secondaryShift(rect, pos);
		const shifted = new DOMRect(rect.x + shift.dx, rect.y + shift.dy, rect.width, rect.height);
		if (overflow(shifted) === 0) return { fits: true, shift, mh };
		return { fits: false, shift, mh };
	}

	$effect(() => {
		if (!visible || !popoverEl || !wrapperEl) {
			maxHeight = '0px';
			positioned = false;
			return;
		}

		const el = popoverEl;
		const wrapRect = wrapperEl.getBoundingClientRect();

		// Lock document scroll during measurement to prevent jitter when
		// the unconstrained popover would extend beyond the viewport.
		const prevOverflow = document.documentElement.style.overflow;
		document.documentElement.style.overflow = 'hidden';

		// Measure natural size by temporarily removing constraints via direct DOM manipulation.
		// This bypasses Svelte's batched updates so we get an accurate measurement.
		el.style.maxHeight = 'none';
		void el.offsetHeight;
		const natW = el.offsetWidth;
		const natH = el.offsetHeight;

		// Restore scroll behavior
		document.documentElement.style.overflow = prevOverflow;

		// Now compute everything mathematically — no more DOM measurement needed.
		const opposite: Position =
			position === 'leading'
				? 'trailing'
				: position === 'trailing'
					? 'leading'
					: position === 'above'
						? 'below'
						: 'above';
		const candidates = ALL_POSITIONS.filter((p) => p !== position);
		candidates.sort((a, b) => (a === opposite ? -1 : b === opposite ? 1 : 0));

		// Try preferred position first
		let result = tryPosition(wrapRect, position, natW, natH);
		if (result.fits) {
			resolvedPosition = position;
			maxHeight = `${result.mh}px`;
			el.style.maxHeight = maxHeight;
			offsetX = result.shift.dx;
			offsetY = result.shift.dy;
			positioned = true;
			return;
		}

		// Try other positions
		for (const candidate of candidates) {
			result = tryPosition(wrapRect, candidate, natW, natH);
			if (result.fits) {
				resolvedPosition = candidate;
				maxHeight = `${result.mh}px`;
				el.style.maxHeight = maxHeight;
				offsetX = result.shift.dx;
				offsetY = result.shift.dy;
				positioned = true;
				return;
			}
		}

		// Fallback: preferred position, constrained height, best-effort shift
		const mh = Math.max(0, availableHeight(wrapRect, position));
		const rect = computeRect(wrapRect, position, natW, natH);
		const shift = secondaryShift(rect, position);
		resolvedPosition = position;
		maxHeight = `${mh}px`;
		el.style.maxHeight = maxHeight;
		offsetX = shift.dx;
		offsetY = shift.dy;
		positioned = true;
	});

	let transformStyle = $derived(
		`${baseTransform(resolvedPosition)} translate(${offsetX}px, ${offsetY}px)`
	);
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
	class="popover-wrap"
	bind:this={wrapperEl}
	onmouseenter={() => (visible = true)}
	onmouseleave={() => (visible = false)}
>
	{@render children()}
	{#if visible}
		<div class="popover-bridge {resolvedPosition}"></div>
		<div
			class="popover-bubble {resolvedPosition}"
			style:width
			style:max-height={maxHeight}
			style:transform={transformStyle}
			style:padding
			style:visibility={positioned ? null : 'hidden'}
			bind:this={popoverEl}
		>
			{@render popover()}
		</div>
	{/if}
</div>

<style>
	.popover-wrap {
		position: relative;
		display: block;
	}

	.popover-bridge {
		position: absolute;
		z-index: 9;
	}

	.popover-bridge.above {
		bottom: 100%;
		left: -200px;
		right: -200px;
		height: 6px;
	}

	.popover-bridge.below {
		top: 100%;
		left: -200px;
		right: -200px;
		height: 6px;
	}

	.popover-bridge.leading {
		right: 100%;
		top: -200px;
		bottom: -200px;
		width: 6px;
	}

	.popover-bridge.trailing {
		left: 100%;
		top: -200px;
		bottom: -200px;
		width: 6px;
	}

	.popover-bubble {
		position: absolute;
		box-sizing: border-box;
		padding: 1rem;
		background: rgba(255, 255, 255, 0.85);
		backdrop-filter: blur(10px);
		-webkit-backdrop-filter: blur(10px);
		border: 0.5px solid rgba(0, 0, 0, 0.12);
		border-radius: 8px;
		box-shadow: 0 1px 3px rgba(0, 0, 0, 0.18);
		z-index: 10;
		white-space: nowrap;
		overflow-y: auto;
	}

	.popover-bubble.above {
		bottom: calc(100% + 6px);
		left: 50%;
	}

	.popover-bubble.below {
		top: calc(100% + 6px);
		left: 50%;
	}

	.popover-bubble.leading {
		right: calc(100% + 6px);
		top: 50%;
	}

	.popover-bubble.trailing {
		left: calc(100% + 6px);
		top: 50%;
	}
</style>
