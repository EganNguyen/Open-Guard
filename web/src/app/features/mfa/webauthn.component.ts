import { Component, OnInit, inject, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { ActivatedRoute, Router } from '@angular/router';
import { AuthService } from '../../core/services/auth.service';

@Component({
  selector: 'app-mfa-webauthn',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div class="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div class="max-w-md w-full space-y-8 p-10 bg-white rounded-xl shadow-lg text-center">
        <h2 class="text-3xl font-extrabold text-gray-900">Security Key</h2>
        <p class="mt-2 text-sm text-gray-600">
          Please insert your security key and follow the browser prompts.
        </p>
        <div class="mt-8">
          <button
            (click)="authenticate()"
            [disabled]="isLoading()"
            class="w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none disabled:opacity-50"
          >
            {{ isLoading() ? 'Waiting for key...' : 'Use Security Key' }}
          </button>
        </div>
        <div *ngIf="errorMessage()" class="mt-4 text-red-500 text-sm">
          {{ errorMessage() }}
        </div>
      </div>
    </div>
  `,
})
export class WebauthnComponent implements OnInit {
  private authService = inject(AuthService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);

  isLoading = signal(false);
  errorMessage = signal<string | null>(null);
  challengeToken: string | null = null;

  ngOnInit() {
    this.challengeToken = this.route.snapshot.queryParamMap.get('challenge');
    if (!this.challengeToken) {
      this.router.navigate(['/login']);
    }
  }

  async authenticate() {
    this.isLoading.set(true);
    this.errorMessage.set(null);
    
    // Placeholder for actual navigator.credentials.get() flow
    this.errorMessage.set('WebAuthn flow not fully implemented in this prototype');
    this.isLoading.set(false);
  }
}
