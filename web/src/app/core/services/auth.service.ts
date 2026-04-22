import { Injectable, signal, computed, inject, PLATFORM_ID } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { Router } from '@angular/router';
import { ApiService } from './api.service';
import { Observable, tap } from 'rxjs';

export interface User {
  id: string;
  email: string;
  display_name: string;
  org_id: string;
  status: string;
}

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

  constructor() {
    this.init();
  }

  private init(): void {
    if (isPlatformBrowser(this.platformId)) {
      this.api.get<any>('/auth/me').subscribe({
        next: (res) => {
          this.currentUser.set(res.user);
        },
        error: () => {
          this.currentUser.set(null);
        }
      });
    }
  }

  login(credentials: any, oauthParams?: any): Observable<any> {
    const { email, password } = credentials;
    return this.api.post<any>('/auth/login', { email, password }).pipe(
      tap(res => {
        if (oauthParams && oauthParams.client_id && oauthParams.redirect_uri) {
          window.location.href = `${oauthParams.redirect_uri}?code=skeleton-auth-code&state=${oauthParams.state || ''}`;
          return;
        }

        this.currentUser.set(res.user);
        this.router.navigate(['/overview']);
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
