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
  styleUrls: ['./login.css']
})
export class LoginComponent implements OnInit {
  private fb = inject(FormBuilder);
  private authService = inject(AuthService);
  private route = inject(ActivatedRoute);

  loginForm = this.fb.group({
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required]],
    rememberMe: [false]
  });

  isLoading = signal(false);
  showPassword = signal(false);
  errorMessage = signal<string | null>(null);
  
  mfaRequired = signal(false);
  mfaChallenge = signal<string | null>(null);
  mfaCode = signal('');

  oauthParams: any = null;

  ngOnInit() {
    this.route.queryParams.subscribe(params => {
      if (params['client_id'] && params['redirect_uri']) {
        this.oauthParams = {
          client_id: params['client_id'],
          redirect_uri: params['redirect_uri'],
          state: params['state']
        };
      }
    });
  }

  togglePassword(): void {
    this.showPassword.update(v => !v);
  }

  onSubmit(): void {
    if (this.loginForm.valid) {
      this.isLoading.set(true);
      this.errorMessage.set(null);
      
      this.authService.login(this.loginForm.value, this.oauthParams).subscribe({
        next: (res) => {
          if (res.mfa_required) {
            this.mfaRequired.set(true);
            this.mfaChallenge.set(res.mfa_challenge);
            this.isLoading.set(false);
          }
        },
        error: (err) => {
          this.errorMessage.set(err.message || 'Login failed');
          this.isLoading.set(false);
        }
      });
    }
  }

  onMfaSubmit(): void {
    if (this.mfaCode().length === 6) {
      this.isLoading.set(true);
      this.authService.verifyMfa(this.mfaChallenge()!, this.mfaCode(), this.oauthParams).subscribe({
        error: (err) => {
          this.errorMessage.set(err.message || 'MFA verification failed');
          this.isLoading.set(false);
        }
      });
    }
  }
}
