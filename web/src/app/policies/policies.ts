import { Component, inject, OnInit, signal } from '@angular/core';
import { CommonModule } from '@angular/common';
import { FormsModule, ReactiveFormsModule, FormBuilder, FormGroup, Validators, FormArray } from '@angular/forms';
import { PolicyService } from '../core/services/policy.service';
import { AuthService } from '../core/services/auth.service';
import { Policy, PolicyLogic, EvaluateRequest, EvaluateResponse } from '../core/models/policy.model';
import { ConfirmDialogComponent } from '../core/components/confirm-dialog';

@Component({
  selector: 'app-policies',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule, ConfirmDialogComponent],
  templateUrl: './policies.html',
  styleUrls: ['./policies.css']
})
export class PoliciesComponent implements OnInit {
  private policyService = inject(PolicyService);
  private authService = inject(AuthService);
  private fb = inject(FormBuilder);

  policies = signal<Policy[]>([]);
  loading = signal(true);
  showModal = signal(false);
  isEditing = signal(false);
  editingId = signal<string | null>(null);
  
  // Playground state
  playgroundRequest = signal<EvaluateRequest>({
    org_id: '',
    subject_id: 'user:admin',
    action: 'read',
    resource: 'document:123'
  });
  playgroundResponse = signal<EvaluateResponse | null>(null);
  evaluating = signal(false);

  // Confirm Dialog state
  showConfirm = signal(false);
  policyToDelete = signal<Policy | null>(null);

  policyForm: FormGroup = this.fb.group({
    name: ['', Validators.required],
    description: [''],
    logic: this.fb.group({
      type: ['rbac', Validators.required],
      subjects: this.fb.array([]),
      actions: this.fb.array([]),
      resources: this.fb.array([]),
      expression: ['']
    })
  });

  ngOnInit() {
    this.loadPolicies();
    
    // Subscribe to type changes to adjust validators
    this.policyForm.get('logic.type')?.valueChanges.subscribe(type => {
      const expressionControl = this.policyForm.get('logic.expression');
      if (type === 'cel') {
        expressionControl?.setValidators([Validators.required]);
      } else {
        expressionControl?.clearValidators();
      }
      expressionControl?.updateValueAndValidity();
    });
  }

  loadPolicies() {
    const user = this.authService.user();
    if (!user) return;

    this.loading.set(true);
    this.policyService.listPolicies(user.org_id).subscribe({
      next: (res) => {
        this.policies.set(res?.policies || []);
        this.loading.set(false);
      },
      error: (err) => {
        console.error('Failed to load policies', err);
        this.loading.set(false);
      }
    });
  }

  get subjects() { return this.policyForm.get('logic.subjects') as FormArray; }
  get actions() { return this.policyForm.get('logic.actions') as FormArray; }
  get resources() { return this.policyForm.get('logic.resources') as FormArray; }

  addValue(array: FormArray, value: string = '') {
    array.push(this.fb.control(value, Validators.required));
  }

  removeValue(array: FormArray, index: number) {
    array.removeAt(index);
  }

  openCreateModal() {
    this.isEditing.set(false);
    this.editingId.set(null);
    this.policyForm.reset({
      logic: { type: 'rbac' }
    });
    this.clearFormArrays();
    this.addValue(this.subjects, '*');
    this.addValue(this.actions, '*');
    this.addValue(this.resources, '*');
    this.showModal.set(true);
  }

  clearFormArrays() {
    while (this.subjects.length) this.subjects.removeAt(0);
    while (this.actions.length) this.actions.removeAt(0);
    while (this.resources.length) this.resources.removeAt(0);
  }

  openEditModal(policy: Policy) {
    this.isEditing.set(true);
    this.editingId.set(policy.id);
    
    const logic = policy.logic as PolicyLogic;
    this.clearFormArrays();
    
    if (logic.subjects) logic.subjects.forEach(s => this.addValue(this.subjects, s));
    if (logic.actions) logic.actions.forEach(a => this.addValue(this.actions, a));
    if (logic.resources) logic.resources.forEach(r => this.addValue(this.resources, r));

    this.policyForm.patchValue({
      name: policy.name,
      description: policy.description,
      logic: { 
        type: logic.type,
        expression: logic.expression || ''
      }
    });
    
    this.showModal.set(true);
  }

  savePolicy() {
    if (this.policyForm.invalid) return;

    const user = this.authService.user();
    if (!user) return;

    const formValue = this.policyForm.value;
    const policyData: Partial<Policy> = {
      org_id: user.org_id,
      name: formValue.name,
      description: formValue.description,
      logic: formValue.logic
    };

    if (this.isEditing()) {
      this.policyService.updatePolicy(this.editingId()!, policyData).subscribe({
        next: () => {
          this.showModal.set(false);
          this.loadPolicies();
        },
        error: (err) => console.error('Update failed', err)
      });
    } else {
      this.policyService.createPolicy(policyData).subscribe({
        next: () => {
          this.showModal.set(false);
          this.loadPolicies();
        },
        error: (err) => console.error('Create failed', err)
      });
    }
  }

  confirmDelete(policy: Policy) {
    this.policyToDelete.set(policy);
    this.showConfirm.set(true);
  }

  deletePolicy() {
    const policy = this.policyToDelete();
    if (!policy) return;

    this.policyService.deletePolicy(policy.id, policy.org_id).subscribe({
      next: () => {
        this.showConfirm.set(false);
        this.loadPolicies();
      },
      error: (err) => console.error('Delete failed', err)
    });
  }

  runTest() {
    const user = this.authService.user();
    if (!user) return;

    this.evaluating.set(true);
    const req = { ...this.playgroundRequest(), org_id: user.org_id };
    this.policyService.evaluate(req).subscribe({
      next: (res) => {
        this.playgroundResponse.set(res);
        this.evaluating.set(false);
      },
      error: (err) => {
        console.error('Evaluation failed', err);
        this.evaluating.set(false);
      }
    });
  }
}
