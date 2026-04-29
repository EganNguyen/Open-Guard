import { Component, inject, signal, OnInit } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule, ReactiveFormsModule, FormBuilder, Validators } from '@angular/forms';
import { AuthService } from '../../core/services/auth.service';
import { ActivatedRoute } from '@angular/router';

@Component({
  selector: 'app-login',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule],
  templateUrl: './login.html',
  styleUrls: ['./login.css'],
})
export class LoginComponent implements OnInit {
  private fb = inject(FormBuilder);
  private authService = inject(AuthService);
  private route = inject(ActivatedRoute);

  loginForm = this.fb.group({
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required]],
    rememberMe: [false],
  });

  isLoading = signal(false);
  showPassword = signal(false);
  errorMessage = signal<string | null>(null);

  mfaRequired = signal(false);
  mfaChallenge = signal<string | null>(null);
  mfaCode = signal('');

  oauthParams: {
    client_id: string;
    redirect_uri: string;
    state?: string;
    code_challenge?: string;
    code_challenge_method?: string;
  } | null = null;

  ngOnInit() {
    this.route.queryParams.subscribe((params) => {
      if (params['client_id'] && params['redirect_uri']) {
        this.oauthParams = {
          client_id: params['client_id'],
          redirect_uri: params['redirect_uri'],
          state: params['state'],
          code_challenge: params['code_challenge'],
          code_challenge_method: params['code_challenge_method'],
        };
      }
    });
  }

  togglePassword(): void {
    this.showPassword.update((v) => !v);
  }

  onSubmit(): void {
    if (this.loginForm.valid) {
      this.isLoading.set(true);
      this.errorMessage.set(null);

      this.authService.login(this.loginForm.value as any, this.oauthParams ?? undefined).subscribe({
        next: (res) => {
          if (res.mfa_required) {
            this.mfaRequired.set(true);
            this.mfaChallenge.set(res.mfa_challenge || null);
            this.isLoading.set(false);
          }
        },
        error: (err) => {
          this.errorMessage.set(err.message || 'Login failed');
          this.isLoading.set(false);
        },
      });
    }
  }

  onMfaSubmit(): void {
    if (this.mfaCode().length === 6) {
      this.isLoading.set(true);
      this.authService.verifyMfa(this.mfaChallenge()!, this.mfaCode(), this.oauthParams ?? undefined).subscribe({
        error: (err) => {
          this.errorMessage.set(err.message || 'MFA verification failed');
          this.isLoading.set(false);
        },
      });
    }
  }
}
