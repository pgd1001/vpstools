import { NextResponse } from 'next/server';
import { beginLogin } from '../../../lib/oidc';
export async function GET() { try { return NextResponse.redirect(await beginLogin()); } catch (error) { return NextResponse.json({error: error instanceof Error ? error.message : 'OIDC configuration error'}, {status:500}); } }
