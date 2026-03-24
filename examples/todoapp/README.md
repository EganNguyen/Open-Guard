# OpenGuard Todo App Example

This directory contains a complete, working "Main Product" (A Simple Todo Application) that securely integrates with the OpenGuard Proxy Gateway.

## Concept: Zero-Trust Identity via the Gateway
In an OpenGuard architecture, this Todo App does **not** know how to parse JWTs, check passwords, or enforce rate limits. It completely trusts the OpenGuard Gateway to do that.

The only security rule this application enforces is that it **must receive the `X-User-ID` and `X-Org-ID` HTTP headers**, which are securely injected by the OpenGuard Gateway after a successful request authentication.

If someone attempts to hit this Todo App directly (bypassing the Gateway) without the headers, the Todo App drops the request.

## Architecture

```text
[ User ]  -->  GET /api/v1/todos  --> [ OpenGuard Gateway (:8080) ]
                                             |
                                        (Validates JWT)
                                        (Appends X-User-ID)
                                             |
                                      [ Todo App (:8081) ]
```

## How It Works

1. **The Todo App** is listening on a private/internal port (e.g., `8081`).
2. **The OpenGuard Gateway** is listening to the public internet on port `8080`.
3. When you make a request to the Gateway with a valid Bearer Token, the Gateway unwraps the token, identifies the user, and securely HTTP proxies the request to the Todo App, injecting `X-User-ID: "user-123"`.
4. The Todo App uses the `X-User-ID` to query the database and return only that specific user's todos.

## Running the Example

### 1. Start the Todo App
Run this application natively using Go:
```bash
cd examples/todoapp
go run main.go
```
The server will start on `http://localhost:8081`.

### 2. Configure the OpenGuard Gateway
Ensure your OpenGuard Gateway is configured to route traffic to this service. For example, in your gateway config:
```yaml
routes:
  - path_prefix: "/todos"
    upstream_url: "http://localhost:8081"
```

### 3. Test It!

**If you try to hit the Todo app directly (Without OpenGuard):**
```bash
curl http://localhost:8081/todos
```
> `401 Unauthorized: Missing OpenGuard identity headers. Bypassing the gateway is strictly prohibited.`

**When you hit the OpenGuard Gateway (With a valid JWT):**
```bash
curl -H "Authorization: Bearer <VALID_JWT>" http://localhost:8080/todos
```
> `200 OK: [{"id": 1, "title": "Buy milk", "user_id": "user-123"}]`
