import { Component, OnInit, signal, inject, PLATFORM_ID } from '@angular/core';
import { isPlatformBrowser, CommonModule } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { FormsModule, ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { environment } from '../../environments/environment';
import { catchError, of } from 'rxjs';

@Component({
  selector: 'app-connectors',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule],
  templateUrl: './connectors.html',
  styleUrl: './connectors.css'
})
export class ConnectorsComponent implements OnInit {
  private http = inject(HttpClient);
  private fb = inject(FormBuilder);
  private platformId = inject(PLATFORM_ID);

  connectors = signal<any[]>([]);
  loading = signal(true);
  error = signal('');
  showModal = signal(false);
  submitting = signal(false);
  registrationResult = signal<any>(null);
  isEditing = signal(false);
  editingConnectorId = signal('');

  connectorForm: FormGroup = this.fb.group({
    name: ['', [Validators.required, Validators.minLength(3)]],
    redirect_uri: ['http://localhost:3000/api/auth/callback', [Validators.required]]
  });

  ngOnInit() {
    this.fetchConnectors();
  }

  fetchConnectors() {
    this.loading.set(true);
    this.error.set('');

    this.http.get<any[]>(`${environment.apiUrl}/mgmt/connectors`).pipe(
      catchError(err => {
        console.error('Failed to load connectors', err);
        this.error.set('Failed to load connectors');
        this.loading.set(false);
        return of([]);
      })
    ).subscribe({
      next: (data) => {
        if (!Array.isArray(data)) {
          this.connectors.set([]);
        } else {
          this.connectors.set(data.map(c => ({
            id: c.id,
            name: c.name,
            redirect_uris: c.redirect_uris,
            status: 'Active',
            scopes: ['openid', 'profile', 'email'],
            createdDate: 'Apr 17, 2026',
            eventVolume: '0'
          })));
        }
        this.loading.set(false);
      }
    });
  }

  openModal() {
    this.isEditing.set(false);
    this.editingConnectorId.set('');
    this.showModal.set(true);
    this.registrationResult.set(null);
    this.connectorForm.reset({
      name: '',
      redirect_uri: 'http://localhost:3000/api/auth/callback'
    });
  }

  closeModal() {
    this.showModal.set(false);
    this.registrationResult.set(null);
  }

  onRegisterConnector() {
    if (this.connectorForm.invalid) {
      return;
    }

    this.submitting.set(true);
    const formValue = this.connectorForm.value;
    
    if (this.isEditing()) {
      const updatedConnector = {
        name: formValue.name,
        redirect_uris: [formValue.redirect_uri]
      };

      this.http.put(`${environment.apiUrl}/mgmt/connectors/${this.editingConnectorId()}`, updatedConnector).subscribe({
        next: () => {
          this.submitting.set(false);
          this.closeModal();
          this.fetchConnectors();
        },
        error: (err) => {
          this.submitting.set(false);
          console.error('Failed to update connector', err);
          alert('Failed to update connector: ' + (err.error || err.message));
        }
      });
    } else {
      const newConnector = {
        id: `app-${Math.random().toString(36).substring(7)}`,
        name: formValue.name,
        client_secret: 'sk_' + Math.random().toString(36).substring(7) + Math.random().toString(36).substring(7),
        redirect_uris: [formValue.redirect_uri]
      };

      this.http.post(`${environment.apiUrl}/mgmt/connectors`, newConnector).subscribe({
        next: (res: any) => {
          this.submitting.set(false);
          this.registrationResult.set({
            ...newConnector,
            org_id: res.org_id
          });
          this.fetchConnectors();
        },
        error: (err) => {
          this.submitting.set(false);
          console.error('Failed to register connector', err);
          alert('Failed to register connector: ' + (err.error || err.message));
        }
      });
    }
  }

  editConnector(connector: any) {
    this.isEditing.set(true);
    this.editingConnectorId.set(connector.id);
    this.showModal.set(true);
    this.registrationResult.set(null);
    this.connectorForm.reset({
      name: connector.name,
      redirect_uri: connector.redirect_uris && connector.redirect_uris.length > 0 ? connector.redirect_uris[0] : ''
    });
  }

  deleteConnector(id: string) {
    if (confirm('Are you sure you want to delete this connector?')) {
      this.http.delete(`${environment.apiUrl}/mgmt/connectors/${id}`).subscribe({
        next: () => {
          this.fetchConnectors();
        },
        error: (err) => {
          console.error('Failed to delete connector', err);
          alert('Failed to delete connector');
        }
      });
    }
  }
}
