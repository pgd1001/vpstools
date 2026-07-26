import { NextRequest, NextResponse } from 'next/server';
import { clearSession, providerLogoutUrl, readSession } from '../../../lib/oidc';
export async function GET(request: NextRequest) { const session=await readSession(); await clearSession(); if(process.env.ZITADEL_PROVIDER_LOGOUT==='true' && session){try{return NextResponse.redirect(await providerLogoutUrl(session));}catch{}} return NextResponse.redirect(new URL('/',request.url)); }
