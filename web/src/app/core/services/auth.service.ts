import { Injectable, signal, computed, inject, PLATFORM_ID } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { Router } from '@angular/router';
import { ApiService } from './api.service';
import { Observable, tap } from 'rxjs';

import { User, AuthResponse, LoginCredentials } from '../models/user.model';


@Injectable({
  providedIn: 'root'
})
export class AuthService {
  private api = inject(ApiService);
  private router = inject(Router);
  private platformId = inject(PLATFORM_ID);

  private currentUser = signal<User | null>(null);
  user = this.currentUser.asReadonly();

  isAuthenticated = computed(() => !!this.currentUser());
  currentOrgId = computed(() => this.currentUser()?.org_id);

  getCurrentOrgId(): string | undefined {
    return this.currentUser()?.org_id;
  }

  constructor() {
    this.init();
  }

  private init(): void {
    if (isPlatformBrowser(this.platformId)) {
      this.api.get<{ user: User }>('/auth/me').subscribe({
        next: (res) => {
          this.currentUser.set(res.user);
        },
        error: () => {
          this.currentUser.set(null);
        }
      });
    }
  }

  login(credentials: LoginCredentials, oauthParams?: Record<string, string>): Observable<AuthResponse> {
    const { email, password } = credentials;
    const url = oauthParams ? '/auth/oauth/login' : '/auth/login';
    const body = oauthParams ? { email, password, ...oauthParams } : { email, password };

    return this.api.post<AuthResponse>(url, body).pipe(
      tap(res => {
        if (res.mfa_required) {
          return;
        }

        if (oauthParams && res.code) {
          window.location.href = `${oauthParams.redirect_uri}?code=${res.code}&state=${res.state || ''}`;
          return;
        }

        this.currentUser.set(res.user);
        this.router.navigate(['/']); // R-06: /overview does not exist
      })
    );
  }

  verifyMfa(mfaChallenge: string, code: string, oauthParams?: Record<string, string>): Observable<AuthResponse> {
    return this.api.post<AuthResponse>('/auth/mfa/verify', { mfa_challenge: mfaChallenge, code }).pipe(
      tap(res => {
        if (oauthParams && res.code) {
          window.location.href = `${oauthParams.redirect_uri}?code=${res.code}&state=${res.state || ''}`;
          return;
        }

        this.currentUser.set(res.user);
        this.router.navigate(['/']);
      })
    );
  }

  logout(): void {
    this.api.post('/auth/logout', {}).subscribe(() => {
      this.currentUser.set(null);
      this.router.navigate(['/login']);
    });
  }

  setCurrentUser(user: User | null): void {
    this.currentUser.set(user);
  }
}
