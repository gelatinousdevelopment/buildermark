type QueueTask = {
	priority: number;
	order: number;
	run: () => Promise<unknown>;
	resolve: (value: unknown) => void;
	reject: (reason?: unknown) => void;
};

const MAX_CONCURRENT = 3;
let active = 0;
let nextOrder = 0;
const pending: QueueTask[] = [];

function compareTasks(a: QueueTask, b: QueueTask): number {
	if (a.priority !== b.priority) return a.priority - b.priority;
	return a.order - b.order;
}

function pumpQueue() {
	while (active < MAX_CONCURRENT && pending.length > 0) {
		pending.sort(compareTasks);
		const task = pending.shift();
		if (!task) return;
		active += 1;
		void task
			.run()
			.then((result) => task.resolve(result))
			.catch((error) => task.reject(error))
			.finally(() => {
				active -= 1;
				pumpQueue();
			});
	}
}

export function enqueueLoad<T>(run: () => Promise<T>, priority = 0): Promise<T> {
	return new Promise<T>((resolve, reject) => {
		pending.push({
			priority,
			order: nextOrder++,
			run: () => run() as Promise<unknown>,
			resolve: (value) => resolve(value as T),
			reject
		});
		pumpQueue();
	});
}
