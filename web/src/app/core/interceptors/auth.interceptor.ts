import { HttpInterceptorFn } from '@angular/common/http';

export const authInterceptor: HttpInterceptorFn = (req, next) => {
  // In a real app, we'd get the token from a secure cookie (handled by browser)
  // or a state management system. For now, we'll just pass through.
  const cloned = req.clone({
    withCredentials: true,
  });
  return next(cloned);
};
