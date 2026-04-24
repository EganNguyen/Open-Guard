import { HttpInterceptorFn, HttpErrorResponse } from '@angular/common/http';
import { inject } from '@angular/core';
import { catchError, throwError, switchMap } from 'rxjs';
import { AuthService } from '../services/auth.service';
import { ApiService } from '../services/api.service';
import { Router } from '@angular/router';

export const errorInterceptor: HttpInterceptorFn = (req, next) => {
  const authService = inject(AuthService);
  const apiService = inject(ApiService);
  const router = inject(Router);

  return next(req).pipe(
    catchError((err: HttpErrorResponse) => {
      // If 401 and not a refresh request, try to refresh
      if (err.status === 401 && !req.url.includes('/auth/refresh') && !req.url.includes('/auth/login')) {
        return apiService.post('/auth/refresh', {}).pipe(
          switchMap(() => next(req)), // Retry the original request
          catchError((refreshErr) => {
            // Refresh failed, log out
            authService.logout();
            router.navigate(['/login']);
            return throwError(() => refreshErr);
          })
        );
      }
      
      // For other errors or if it was a refresh/login request, just throw
      if (err.status === 401) {
        authService.logout();
        router.navigate(['/login']);
      }
      
      return throwError(() => err);
    })
  );
};
