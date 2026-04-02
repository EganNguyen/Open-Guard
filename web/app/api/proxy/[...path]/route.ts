import { NextRequest, NextResponse } from "next/server";
import { auth } from "@/auth";

const BACKEND_URL = process.env.API_INTERNAL_URL || "http://control-plane:8080";

export async function proxyRequest(
  req: NextRequest,
  { params }: { params: { path: string[] } }
) {
  const session = (await auth()) as any;
  const { path } = await params;

  if (!session?.accessToken) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const pathStr = path.join("/");
  const url = new URL(`${BACKEND_URL}/api/v1/${pathStr}${req.nextUrl.search}`);

  try {
    const body = req.method !== "GET" && req.method !== "HEAD" ? await req.text() : undefined;
    
    const res = await fetch(url.toString(), {
      method: req.method,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${session.accessToken}`,
        "X-Org-ID": session.orgId,
      },
      body,
    });

    const data = await res.json().catch(() => ({}));
    return NextResponse.json(data, { status: res.status });
  } catch (error) {
    console.error(`Proxy error [${req.method}] ${pathStr}:`, error);
    return NextResponse.json(
      { error: "Internal Server Error", details: (error as Error).message },
      { status: 500 }
    );
  }
}

export const GET = proxyRequest;
export const POST = proxyRequest;
export const PUT = proxyRequest;
export const PATCH = proxyRequest;
export const DELETE = proxyRequest;
