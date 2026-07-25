import { NextResponse } from 'next/server';
import { readSession } from '../../../lib/oidc';
export async function GET() { const session=await readSession(); return session ? NextResponse.json({authenticated:true,email:session.email,name:session.name}) : NextResponse.json({authenticated:false},{status:401}); }
