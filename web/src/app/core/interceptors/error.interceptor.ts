import { HttpInterceptorFn, HttpErrorResponse } from '@angular/common/http';
import { inject } from '@angular/core';
import { catchError, throwError, switchMap } from 'rxjs';
import { AuthService } from '../services/auth.service';
import { ApiService } from '../services/api.service';
import { Router } from '@angular/router';
import { LoggingService } from '../services/logging.service';

export const errorInterceptor: HttpInterceptorFn = (req, next) => {
  const authService = inject(AuthService);
  const apiService = inject(ApiService);
  const router = inject(Router);
  const loggingService = inject(LoggingService);

  return next(req).pipe(
    catchError((err: HttpErrorResponse) => {
      // Log error to Loki (unless it's the logging request itself)
      if (!req.headers.has('X-Skip-Logging')) {
        loggingService.error(`HTTP ${err.status} ${req.method} ${req.url}`, {
          status: err.status,
          statusText: err.statusText,
          url: req.url,
          method: req.method,
          error: err.error
        });
      }

      // If 401 and not a refresh request, try to refresh
      if (err.status === 401 && !req.url.includes('/auth/refresh') && !req.url.includes('/auth/login') && !req.url.includes('/auth/logout')) {
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
