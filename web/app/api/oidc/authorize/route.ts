import { auth } from "@/auth";
import { NextResponse } from "next/server";

export async function POST(req: Request) {
  const session = await auth();
  if (!session) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const { client_id, redirect_uri, state, scope } = await req.json();

  try {
    // Call IAM internal endpoint to generate a real authorization code
    const res = await fetch(`${process.env.API_INTERNAL_URL || "http://openguard-controlplane:8080"}/api/v1/auth/oidc/authorize`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        client_id,
        redirect_uri,
        user_id: (session as any).user?.id || (session as any).id,
        org_id: (session as any).orgId,
        state,
        scope,
      }),
    });

    if (!res.ok) {
      const error = await res.text();
      return NextResponse.json({ error: `IAM Authorization failed: ${error}` }, { status: res.status });
    }

    const { code } = await res.json();
    return NextResponse.json({ code });
  } catch (error) {
    console.error("OIDC Authorization Error:", error);
    return NextResponse.json({ error: "Internal Server Error" }, { status: 500 });
  }
}
