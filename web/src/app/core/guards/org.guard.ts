import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from '../services/auth.service';
import { map, take } from 'rxjs';

/**
 * OrgGuard ensures that the user has a valid organization context (org_id)
 * before accessing protected dashboard routes.
 */
export const orgGuard: CanActivateFn = (route, state) => {
  const authService = inject(AuthService);
  const router = inject(Router);

  const orgId = authService.getCurrentOrgId();
  if (orgId) {
    return true;
  }

  // If no orgId is found in the current session state, redirect to org selection or login
  console.warn('OrgGuard: No organization context found, redirecting to login');
  return router.createUrlTree(['/auth/login'], {
    queryParams: { returnUrl: state.url, error: 'no_org_context' },
  });
};
