phase 2:
- seed user system admin for open guard
- admin can create connector, create user in open guard for task management application
- connect task management application with open guard
- when user access task management application, if no auth → redirect to login using open guard for authentication and authorization
- navigate to open guard admin if user is system admin
- after login with open guard, user can access task management application
- when user access task management application, if auth → check policy for authorization
- using open guard for audit logging