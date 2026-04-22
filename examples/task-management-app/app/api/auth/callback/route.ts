import { NextRequest, NextResponse } from 'next/server';

export async function GET(request: NextRequest) {
  const searchParams = request.nextUrl.searchParams;
  const code = searchParams.get('code');

  if (!code) {
    return NextResponse.json({ error: 'Missing code' }, { status: 400 });
  }

  try {
    const tokenURL = process.env.OPENGUARD_TOKEN_URL || 'http://localhost:8080/auth/token';
    
    // Exchange the authorization code for an access token
    const tokenResponse = await fetch(tokenURL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: new URLSearchParams({
        client_id: process.env.OPENGUARD_CLIENT_ID || 'task-app',
        client_secret: process.env.OPENGUARD_CLIENT_SECRET || '',
        code: code,
        grant_type: 'authorization_code',
      }),
    });

    if (!tokenResponse.ok) {
      const errText = await tokenResponse.text();
      console.error("Token exchange failed:", errText);
      return NextResponse.json({ error: 'Failed to exchange token' }, { status: tokenResponse.status });
    }

    const data = await tokenResponse.json();

    // Redirect to the dashboard
    const response = NextResponse.redirect(new URL('/', request.url));
    
    // Store the auth token as a cookie
    response.cookies.set('auth_token', data.access_token, {
      httpOnly: true,
      secure: process.env.NODE_ENV === 'production',
      sameSite: 'lax',
      path: '/',
      maxAge: parseInt(data.expires_in || '3600', 10),
    });

    return response;
  } catch (error) {
    console.error("Callback error:", error);
    return NextResponse.json({ error: 'Internal Server Error' }, { status: 500 });
  }
}
