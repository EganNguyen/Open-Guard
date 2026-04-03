import { auth } from "@/auth";
import { NextResponse } from "next/server";

export default auth((req) => {
  const isLoggedIn = !!req.auth;
  const isDashboardRoute = req.nextUrl.pathname.startsWith("/dashboard");

  if (isDashboardRoute && !isLoggedIn) {
    const loginUrl = new URL("/login", req.nextUrl);
    return NextResponse.redirect(loginUrl);
  }

  return NextResponse.next();
});

// Optionally, you can also protect /api routes or others here
export const config = {
  matcher: ["/dashboard/:path*", "/api/((?!auth|oidc/authorize).*)"],
};
