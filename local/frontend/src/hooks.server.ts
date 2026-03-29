import { randomBytes } from 'node:crypto';
import type { Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';

const scriptOpenTagRe = /<script\b([^>]*)>/gi;
const scriptNonceAttrRe = /\bnonce\s*=/i;
const cspNonceMetaRe = /<meta[^>]+property\s*=\s*["']csp-nonce["'][^>]*>/i;
const headOpenTagRe = /<head\b[^>]*>/i;

function buildCSPHeader(nonce: string): string {
	return [
		"default-src 'none'",
		"base-uri 'none'",
		"frame-ancestors 'none'",
		"object-src 'none'",
		"form-action 'self'",
		`script-src 'self' 'nonce-${nonce}'`,
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		env.PUBLIC_READ_ONLY === 'true'
			? "connect-src 'self'"
			: "connect-src 'self' http://localhost:* http://127.0.0.1:* ws://localhost:* ws://127.0.0.1:* wss://localhost:* wss://127.0.0.1:*",
		"manifest-src 'self'",
		"frame-src 'none'",
		"worker-src 'none'"
	].join('; ');
}

function injectNonceIntoHTML(html: string, nonce: string): string {
	let output = html;
	if (!cspNonceMetaRe.test(output)) {
		const meta = `<meta property="csp-nonce" nonce="${nonce}">`;
		output = output.replace(headOpenTagRe, (m) => `${m}\n\t\t${meta}`);
	}

	return output.replace(scriptOpenTagRe, (tag) => {
		if (scriptNonceAttrRe.test(tag)) {
			return tag;
		}
		return tag.replace(/>$/, ` nonce="${nonce}">`);
	});
}

export const handle: Handle = async ({ event, resolve }) => {
	const shouldApplyCSP = event.request.method === 'GET' && !event.url.pathname.startsWith('/api/');
	if (!shouldApplyCSP) {
		return resolve(event);
	}

	const nonce = randomBytes(18).toString('base64url');
	const response = await resolve(event, {
		transformPageChunk: ({ html }) => injectNonceIntoHTML(html, nonce)
	});

	const contentType = response.headers.get('content-type') ?? '';
	if (contentType.includes('text/html')) {
		response.headers.set('Content-Security-Policy', buildCSPHeader(nonce));
	}
	return response;
};
