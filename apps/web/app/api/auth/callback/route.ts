import { NextRequest, NextResponse } from 'next/server';
import { completeLogin } from '../../../lib/oidc';
export async function GET(request: NextRequest) { const code=request.nextUrl.searchParams.get('code')||''; const state=request.nextUrl.searchParams.get('state')||''; try { await completeLogin(code,state); return NextResponse.redirect(new URL('/',request.url)); } catch(error) { return NextResponse.json({error:error instanceof Error?error.message:'OIDC callback failed'},{status:401}); } }
