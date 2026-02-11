export interface Project {
	id: string;
	path: string;
}

export interface ProjectDetail {
	id: string;
	path: string;
	conversations: ConversationWithRatings[];
}

export interface ConversationWithRatings {
	id: string;
	agent: string;
	ratings: Rating[];
}

export interface Conversation {
	id: string;
	projectId: string;
	agent: string;
}

export interface ConversationDetail {
	id: string;
	projectId: string;
	agent: string;
	turns: TurnRead[];
	ratings: Rating[];
}

export interface TurnRead {
	id: string;
	timestamp: number;
	conversationId: string;
	role: string;
	content: string;
}

export interface Rating {
	id: string;
	conversationId: string;
	rating: number;
	note: string;
	analysis: string;
	createdAt: string;
}
