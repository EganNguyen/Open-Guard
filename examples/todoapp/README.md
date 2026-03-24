# OpenGuard Todo App: Real-World Integration Guide

This guide walks you through a complete, end-to-end integration of a real product (the Todo App) into the OpenGuard ecosystem. Unlike a simple test, this flow uses the actual OpenGuard Gateway, IAM Service, and Policy Engine.

## Prerequisites

1. **OpenGuard Stack Running**: 
   Ensure the core services are running via Docker:
   ```bash
   cd services
   docker-compose up -d
   ```
2. **Todo App Running**:
   Start the Todo App Go server:
   ```bash
   cd examples/todoapp
   go run .
   ```
   *The Todo App starts on `http://localhost:8081`.*

---

## Step 1: Register the Route in OpenGuard

OpenGuard’s Gateway acts as a secure "Front Door". You must tell it to "open the door" for your Todo App.

1. Open `services/gateway/pkg/router/router.go`.
2. Locate the `New` function and add a proxy for the Todo App:
   ```go
   // 1. Create a proxy for the Todo App (Internal Address)
   todoProxy, _ := proxy.NewReverseProxy("http://host.docker.internal:8081", cfg.Logger, iamBreaker, cfg.TLSConfig)

   // 2. Register the route prefix
   r.Handle("/api/v1/todos/*", http.StripPrefix("/api/v1", todoProxy))
   ```
3. Restart the Gateway service to apply changes.

---

## Step 2: Create your User (Signup & Login)

Now that the Gateway knows where the Todo App is, you need a valid identity to enter.

1. Visit the **OpenGuard Dashboard**: `http://localhost:3000`.
2. **Sign Up**: Create a new account. This stores your credentials securely in the OpenGuard IAM database.
3. **Login**: Sign in to receive your **JWT (JSON Web Token)**.
4. **Copy your Token**: You will need this to authenticate your requests.

---

## Step 3: Define the Security Policy

Even with a valid login, you are "Unauthorized" until a policy allows you into the Todo App.

1. In the **Dashboard**, navigate to **Policies**.
2. Create a new **Access Rule**:
   - **Service**: `todoapp`
   - **Resource**: `*` (All tasks)
   - **Action**: `read`, `write`
   - **Subject**: Your User ID or Role.
3. Save the Policy. The Policy Engine now has a dynamic database record allowing your access.

---

## Step 4: Use the Protected Product

Now, interact with your Todo App as a fully secured, enterprise-grade user.

1. Open the **Todo App UI**: `http://localhost:8081`.
2. Set the **API Endpoint** to the **Gateway Address**: `http://localhost:8080/api/v1/todos`.
   - *Note: You are now hitting the Todo App **through** OpenGuard!*
3. Paste your **JWT Token** into the Bearer field.
4. **Add a Task**:
   - The Gateway validates your JWT.
   - The Policy Engine confirms your permissions.
   - The Gateway injects your `X-User-ID` header.
   - Your Todo App receives the request and saves the task to your personal list.

---

## Summary of Protection
When you follow this flow, OpenGuard is doing the heavy lifting:
- **IAM** manages your password and MFA.
- **Gateway** blocks any unauthenticated traffic matching `/todos`.
- **Policy Engine** ensures only approved users can "write" new tasks.
- **Todo App** remains simple, focusing only on managing tasks for the `X-User-ID` provided in the header.
