# OpenGuard Todo App: Real-World Integration Guide

This guide walks you through a complete, end-to-end integration of a real product (the Todo App) into the OpenGuard ecosystem using the **Control Plane SDK model**.

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

## Step 1: Create your User (Signup & Login)

To access your Todo App, you need a valid identity governed by OpenGuard.

1. Visit the **OpenGuard Dashboard**: `http://localhost:3000`.
2. **Sign Up**: Create a new account. This stores your credentials securely in the OpenGuard IAM database.
3. **Login**: Sign in to receive your **JWT (JSON Web Token)**.
4. **Copy your Token**: You will need this to authenticate your requests.

---

## Step 2: Define the Security Policy

Even with a valid login, the Todo App will ask the Control Plane if you are allowed to read or write tasks.

1. In the **Dashboard**, navigate to **Policies**.
2. Create a new **Access Rule**:
   - **Service**: `todoapp`
   - **Resource**: `todos`
   - **Action**: `read`, `write`
   - **Subject**: Your User ID or Role.
3. Save the Policy. The Policy Engine now has a dynamic database record allowing your access.

---

## Step 3: Use the Protected Product

Now, interact with your Todo App as a fully secured, enterprise-grade user.

1. Open the **Todo App UI**: `http://localhost:8081`.
2. The UI natively talks to the Todo App API at `http://localhost:8081/api/v1/todos`.
3. Paste your **JWT Token** into the Bearer field.
4. **Add a Task**:
   - The Todo App parses your JWT to identify you.
   - The Todo App makes a REST/SDK call to the Control Plane (`http://localhost:8080/v1/policy/evaluate`) to verify if you have the `write` permission on the `todos` resource.
   - The Control Plane checks the dynamic Policy Engine and returns `true`.
   - Your Todo App proceeds with the request and saves the task.

---

## Summary of Protection
When you follow this flow, OpenGuard is doing the heavy lifting via centralized governance:
- **IAM** manages your password and issues standards-compliant JWTs.
- **Todo App Middleware** secures its own edge, blindly trusting the Control Plane answer.
- **Policy Engine** ensures only approved users can "write" new tasks through centralized RBAC.
