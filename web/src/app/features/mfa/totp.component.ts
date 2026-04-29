import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule, ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { AuthService } from '../../core/services/auth.service';

@Component({
  selector: 'app-mfa-totp',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule],
  template: `
    <div class="min-h-screen flex items-center justify-center bg-gray-50 py-12 px-4 sm:px-6 lg:px-8">
      <div class="max-w-md w-full space-y-8 p-10 bg-white rounded-xl shadow-lg">
        <div>
          <h2 class="mt-6 text-center text-3xl font-extrabold text-gray-900">MFA Verification</h2>
          <p class="mt-2 text-center text-sm text-gray-600">
            Enter the 6-digit code from your authenticator app.
          </p>
        </div>
        <form class="mt-8 space-y-6" [formGroup]="totpForm" (ngSubmit)="onSubmit()">
          <div class="rounded-md shadow-sm -space-y-px">
            <div>
              <label for="code" class="sr-only">TOTP Code</label>
              <input
                id="code"
                name="code"
                type="text"
                formControlName="code"
                required
                class="appearance-none rounded-none relative block w-full px-3 py-2 border border-gray-300 placeholder-gray-500 text-gray-900 rounded-t-md focus:outline-none focus:ring-indigo-500 focus:border-indigo-500 focus:z-10 sm:text-sm"
                placeholder="000000"
                maxlength="6"
              />
            </div>
          </div>

          <div *ngIf="errorMessage()" class="text-red-500 text-sm text-center">
            {{ errorMessage() }}
          </div>

          <div>
            <button
              type="submit"
              [disabled]="totpForm.invalid || isLoading()"
              class="group relative w-full flex justify-center py-2 px-4 border border-transparent text-sm font-medium rounded-md text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500 disabled:opacity-50"
            >
              <span *ngIf="isLoading()">Verifying...</span>
              <span *ngIf="!isLoading()">Verify</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  `,
})
export class TotpComponent implements OnInit {
  private fb = inject(FormBuilder);
  private authService = inject(AuthService);
  private route = inject(ActivatedRoute);
  private router = inject(Router);

  totpForm = this.fb.group({
    code: ['', [Validators.required, Validators.pattern('^[0-9]{6}$')]],
  });

  isLoading = signal(false);
  errorMessage = signal<string | null>(null);
  challengeToken: string | null = null;

  ngOnInit() {
    this.challengeToken = this.route.snapshot.queryParamMap.get('challenge');
    if (!this.challengeToken) {
      this.router.navigate(['/login']);
    }
  }

  onSubmit() {
    if (this.totpForm.valid && this.challengeToken) {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      const code = this.totpForm.get('code')?.value || '';
      
      this.authService.verifyMfa(this.challengeToken, code).subscribe({
        next: () => {
          this.router.navigate(['/']);
        },
        error: (err) => {
          this.errorMessage.set(err.message || 'Invalid code');
          this.isLoading.set(false);
        },
      });
    }
  }
}
