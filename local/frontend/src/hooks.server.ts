// This file is typically only used in dev mode, not in production, since the go server runs the static production build.
// It is required for dev mode because the port for the dev sveltekit frontend is different from the port for the go server.

import { randomBytes } from 'node:crypto';
import { env } from '$env/dynamic/public';
import type { Handle } from '@sveltejs/kit';

const scriptOpenTagRe = /<script\b([^>]*)>/gi;
const scriptNonceAttrRe = /\bnonce\s*=/i;
const cspNonceMetaRe = /<meta[^>]+property\s*=\s*["']csp-nonce["'][^>]*>/i;
const headOpenTagRe = /<head\b[^>]*>/i;

function apiConnectSources(): string {
	const raw = env.PUBLIC_API_URL;
	if (!raw) return '';
	try {
		const url = new URL(raw);
		const origin = url.origin;
		const wsOrigin = origin.replace(/^http/, 'ws');
		return `${origin} ${wsOrigin}`;
	} catch {
		return '';
	}
}

function buildCSPHeader(nonce: string): string {
	const extraConnect = apiConnectSources();
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
		`connect-src 'self'${extraConnect ? ` ${extraConnect}` : ''}`,
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
