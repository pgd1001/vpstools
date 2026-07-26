import { cookies } from 'next/headers';
import { createRemoteJWKSet, jwtDecrypt, jwtVerify, EncryptJWT } from 'jose';
import { randomBytes } from 'node:crypto';

const sessionCookie = 'vps_session';
const stateCookie = 'vps_oidc_state';
const verifierCookie = 'vps_oidc_verifier';

function config() {
  const issuer = process.env.ZITADEL_ISSUER_URL?.replace(/\/$/, '');
  const clientId = process.env.ZITADEL_CLIENT_ID;
  const secret = process.env.SESSION_SECRET;
  if (!issuer || !clientId || !secret || secret.length < 32) throw new Error('ZITADEL_ISSUER_URL, ZITADEL_CLIENT_ID, and a SESSION_SECRET of at least 32 characters are required');
  return { issuer, clientId, secret: new TextEncoder().encode(secret.padEnd(32, '0').slice(0, 32)) };
}

export function cookieOptions(maxAge: number) {
  return { httpOnly: true, secure: process.env.NODE_ENV === 'production', sameSite: 'lax' as const, path: '/', maxAge };
}

export async function beginLogin() {
  const { issuer, clientId } = config();
  const discovery = await fetch(`${issuer}/.well-known/openid-configuration`, { cache: 'no-store' }).then(r => r.json());
  const state = randomBytes(32).toString('base64url');
  const verifier = randomBytes(48).toString('base64url');
  const challenge = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier)).then(x => Buffer.from(x).toString('base64url'));
  const jar = await cookies();
  jar.set(stateCookie, state, cookieOptions(600));
  jar.set(verifierCookie, verifier, cookieOptions(600));
  const redirectUri = process.env.ZITADEL_REDIRECT_URI || `${process.env.APP_URL || 'http://localhost:3000'}/api/auth/callback`;
  const url = new URL(discovery.authorization_endpoint);
  url.searchParams.set('client_id', clientId); url.searchParams.set('response_type', 'code'); url.searchParams.set('scope', 'openid profile email');
  url.searchParams.set('redirect_uri', redirectUri); url.searchParams.set('state', state); url.searchParams.set('code_challenge', challenge); url.searchParams.set('code_challenge_method', 'S256');
  return url.toString();
}

export async function completeLogin(code: string, state: string) {
  const { issuer, clientId, secret } = config();
  const clientSecret = process.env.ZITADEL_CLIENT_SECRET;
  const jar = await cookies();
  const savedState = jar.get(stateCookie)?.value;
  if (!savedState) throw new Error('OIDC state cookie is missing. Use the same hostname for the console and callback, usually http://localhost:3000.');
  if (!state || state !== savedState) throw new Error('OIDC state value did not match. Start a fresh sign-in and do not reuse an older callback tab.');
  const verifier = jar.get(verifierCookie)?.value; if (!verifier) throw new Error('OIDC verifier is missing');
  const discovery = await fetch(`${issuer}/.well-known/openid-configuration`, { cache: 'no-store' }).then(r => r.json());
  const redirectUri = process.env.ZITADEL_REDIRECT_URI || `${process.env.APP_URL || 'http://localhost:3000'}/api/auth/callback`;
  const tokenHeaders: Record<string, string> = {'content-type':'application/x-www-form-urlencoded'};
  const tokenBody = new URLSearchParams({grant_type:'authorization_code',client_id:clientId,code,redirect_uri:redirectUri,code_verifier:verifier});
  if (clientSecret) tokenHeaders.authorization = `Basic ${Buffer.from(`${clientId}:${clientSecret}`).toString('base64')}`;
  const tokenResponse = await fetch(discovery.token_endpoint, { method: 'POST', headers: tokenHeaders, body: tokenBody });
  if (!tokenResponse.ok) {
    const failure = await tokenResponse.json().catch(() => ({}));
    const detail = typeof failure.error_description === 'string' ? `: ${failure.error_description}` : '';
    throw new Error(`ZITADEL token exchange failed (${tokenResponse.status}, ${failure.error || 'unknown_error'})${detail}`);
  }
  const tokens = await tokenResponse.json();
  const jwks = createRemoteJWKSet(new URL(discovery.jwks_uri));
  const verified = await jwtVerify(tokens.id_token, jwks, { issuer, audience: clientId });
  const claims: Record<string, unknown> = { ...verified.payload };
  if (typeof claims.sub !== 'string' || typeof claims.email !== 'string') {
    if (!discovery.userinfo_endpoint || typeof tokens.access_token !== 'string') throw new Error('ZITADEL identity has no subject or email. Add the email scope to the application.');
    const userInfoResponse = await fetch(discovery.userinfo_endpoint, { headers: { authorization: `Bearer ${tokens.access_token}` } });
    if (!userInfoResponse.ok) throw new Error('ZITADEL UserInfo did not return an email claim. Check the email scope and user profile.');
    const userInfo = await userInfoResponse.json();
    if (typeof claims.sub === 'string' && typeof userInfo.sub === 'string' && claims.sub !== userInfo.sub) throw new Error('ZITADEL UserInfo subject did not match the ID token');
    Object.assign(claims, userInfo);
  }
  if (typeof claims.sub !== 'string' || typeof claims.email !== 'string') throw new Error('ZITADEL identity has no subject or email. Check that the ZITADEL user has an email address and that the email scope is enabled.');
  const session = await new EncryptJWT({ sub: claims.sub, email: claims.email, name: claims.name || claims.preferred_username || claims.email, id_token: tokens.id_token }).setProtectedHeader({alg:'dir',enc:'A256GCM'}).setIssuedAt().setExpirationTime('8h').encrypt(secret);
  jar.set(sessionCookie, session, cookieOptions(8 * 60 * 60));
  jar.delete(stateCookie); jar.delete(verifierCookie);
}

export async function readSession() {
  const { secret } = config(); const value = (await cookies()).get(sessionCookie)?.value; if (!value) return null;
  try { const { payload } = await jwtDecrypt(value, secret, { keyManagementAlgorithms:['dir'], contentEncryptionAlgorithms:['A256GCM'] }); return payload as {sub:string;email:string;name?:string;id_token?:string}; } catch { return null; }
}

export async function clearSession() { (await cookies()).delete(sessionCookie); }

export async function providerLogoutUrl(session: {id_token?:string}) {
  const { issuer } = config(); const discovery = await fetch(`${issuer}/.well-known/openid-configuration`, { cache: 'no-store' }).then(r => r.json());
  const redirectUri = process.env.ZITADEL_POST_LOGOUT_REDIRECT_URI || `${process.env.APP_URL || 'http://localhost:3000'}/`;
  const url = new URL(discovery.end_session_endpoint || `${issuer}/oidc/v1/end_session`); url.searchParams.set('post_logout_redirect_uri', redirectUri); if (session.id_token) url.searchParams.set('id_token_hint', session.id_token); return url.toString();
}
