export interface Project {
	id: string;
	path: string;
	label: string;
	gitId: string;
	ignored: boolean;
}

export interface ProjectDetail {
	id: string;
	path: string;
	label: string;
	gitId: string;
	ignored: boolean;
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
