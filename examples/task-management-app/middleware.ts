import { NextResponse } from 'next/server';
import type { NextRequest } from 'next/server';

export function middleware(request: NextRequest) {
  const token = request.cookies.get('auth_token');

  // If trying to access any page without a token (except API and static)
  if (!token) {
    const clientID = process.env.OPENGUARD_CLIENT_ID || "task-app";
    const redirectURI = process.env.REDIRECT_URI || "http://localhost:3000/api/auth/callback";
    const authURL = process.env.OPENGUARD_AUTH_URL || "http://localhost:8080/auth/authorize";
    const state = Math.random().toString(36).substring(7);
    
    const loginURL = `${authURL}?client_id=${clientID}&redirect_uri=${encodeURIComponent(redirectURI)}&state=${state}`;
    return NextResponse.redirect(new URL(loginURL, request.url));
  }

  return NextResponse.next();
}

export const config = {
  // Match all request paths except for the ones starting with:
  // - api (API routes)
  // - _next/static (static files)
  // - _next/image (image optimization files)
  // - favicon.ico (favicon file)
  matcher: ['/((?!api|_next/static|_next/image|favicon.ico).*)'],
};
