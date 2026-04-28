import { Component, OnInit, signal, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import {
  FormsModule,
  ReactiveFormsModule,
  FormBuilder,
  FormGroup,
  Validators,
} from '@angular/forms';
import { ConnectorService, ConnectorUI } from '../core/services/connector.service';
import { Connector, ConnectorRegistrationResult } from '../core/models/connector.model';

import { ConfirmDialogComponent } from '../core/components/confirm-dialog';
import { Subject, switchMap, startWith, shareReplay } from 'rxjs';

@Component({
  selector: 'app-connectors',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule, ConfirmDialogComponent],
  templateUrl: './connectors.html',
  styleUrl: './connectors.css',
})
export class ConnectorsComponent {
  private connectorService = inject(ConnectorService);
  private fb = inject(FormBuilder);

  private refresh$ = new Subject<void>();

  connectors$ = this.refresh$.pipe(
    startWith(undefined),
    switchMap(() => this.connectorService.listConnectors()),
    shareReplay(1),
  );

  showModal = signal(false);
  submitting = signal(false);
  registrationResult = signal<Connector | null>(null);
  isEditing = signal(false);
  editingConnectorId = signal('');

  showConfirm = signal(false);
  connectorToDelete = signal<Connector | null>(null);

  connectorForm: FormGroup = this.fb.group({
    name: ['', [Validators.required, Validators.minLength(3)]],
    redirect_uri: ['http://localhost:3000/api/auth/callback', [Validators.required]],
  });

  openModal() {
    this.isEditing.set(false);
    this.editingConnectorId.set('');
    this.showModal.set(true);
    this.registrationResult.set(null);
    this.connectorForm.reset({
      name: '',
      redirect_uri: 'http://localhost:3000/api/auth/callback',
    });
  }

  closeModal() {
    this.showModal.set(false);
    this.registrationResult.set(null);
  }

  onRegisterConnector() {
    if (this.connectorForm.invalid) return;

    this.submitting.set(true);
    const formValue = this.connectorForm.value;

    if (this.isEditing()) {
      const updatedConnector = {
        name: formValue.name,
        redirect_uris: [formValue.redirect_uri],
      };

      this.connectorService.updateConnector(this.editingConnectorId(), updatedConnector).subscribe({
        next: () => {
          this.submitting.set(false);
          this.closeModal();
          this.refresh$.next();
        },
        error: (err) => {
          this.submitting.set(false);
          console.error('Update failed', err);
        },
      });
    } else {
      const newConnector = {
        id: `app-${Math.random().toString(36).substring(7)}`,
        name: formValue.name,
        client_secret:
          'sk_' + Math.random().toString(36).substring(7) + Math.random().toString(36).substring(7),
        redirect_uris: [formValue.redirect_uri],
      };

      this.connectorService.createConnector(newConnector).subscribe({
        next: (res) => {
          this.submitting.set(false);
          this.registrationResult.set({
            ...newConnector,
            org_id: res.org_id,
          });
          this.refresh$.next();
        },
        error: (err) => {
          this.submitting.set(false);
          console.error('Registration failed', err);
        },
      });
    }
  }

  editConnector(connector: ConnectorUI) {
    this.isEditing.set(true);
    this.editingConnectorId.set(connector.id);
    this.showModal.set(true);
    this.registrationResult.set(null);
    this.connectorForm.reset({
      name: connector.name,
      redirect_uri: connector.redirect_uris?.[0] || '',
    });
  }

  confirmDelete(connector: ConnectorUI) {
    this.connectorToDelete.set(connector);
    this.showConfirm.set(true);
  }

  deleteConnector() {
    const connector = this.connectorToDelete();
    if (!connector) return;

    this.connectorService.deleteConnector(connector.id).subscribe({
      next: () => {
        this.showConfirm.set(false);
        this.refresh$.next();
      },
      error: (err) => console.error('Delete failed', err),
    });
  }
}
