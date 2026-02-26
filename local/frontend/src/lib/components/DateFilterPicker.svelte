<script lang="ts">
	import { dateStringToUnixMsRange, unixMsToDateString } from '$lib/utils';

	interface Props {
		start?: number;
		onchange: (range: { from: number; to: number } | null) => void;
	}

	let { start, onchange }: Props = $props();

	const dateInputValue = $derived(start ? unixMsToDateString(start) : '');

	function handleChange(event: Event) {
		const value = (event.currentTarget as HTMLInputElement).value;
		if (value) {
			onchange(dateStringToUnixMsRange(value));
		} else {
			onchange(null);
		}
	}

	function clear() {
		onchange(null);
	}
</script>

<div class="date-picker">
	<input type="date" value={dateInputValue} onchange={handleChange} />
	{#if start}
		<button class="clear-date" onclick={clear}>×</button>
	{/if}
</div>
