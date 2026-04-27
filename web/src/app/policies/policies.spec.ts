import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ReactiveFormsModule, FormArray } from '@angular/forms';
import { PoliciesComponent } from './policies';
import { PolicyService } from '../core/services/policy.service';
import { AuthService } from '../core/services/auth.service';
import { of } from 'rxjs';
import { signal } from '@angular/core';

describe('PoliciesComponent', () => {
  let component: PoliciesComponent;
  let fixture: ComponentFixture<PoliciesComponent>;
  let mockPolicyService: any;
  let mockAuthService: any;

  beforeEach(async () => {
    mockPolicyService = {
      listPolicies: jasmine.createSpy('listPolicies').and.returnValue(of({ policies: [] })),
      createPolicy: jasmine.createSpy('createPolicy').and.returnValue(of({})),
    };

    mockAuthService = {
      user: signal({ org_id: 'org-123' })
    };

    await TestBed.configureTestingModule({
      imports: [PoliciesComponent, ReactiveFormsModule],
      providers: [
        { provide: PolicyService, useValue: mockPolicyService },
        { provide: AuthService, useValue: mockAuthService }
      ]
    }).compileComponents();

    fixture = TestBed.createComponent(PoliciesComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should initialize with default RBAC values when opening create modal', () => {
    component.openCreateModal();
    
    expect(component.showModal()).toBeTrue();
    expect(component.policyForm.get('logic.type')?.value).toBe('rbac');
    expect(component.subjects.length).toBe(1);
    expect(component.subjects.at(0).value).toBe('*');
  });

  it('should toggle expression validator when switching to CEL', () => {
    const logicGroup = component.policyForm.get('logic');
    
    logicGroup?.get('type')?.setValue('cel');
    expect(logicGroup?.get('expression')?.validator).toBeTruthy();

    logicGroup?.get('type')?.setValue('rbac');
    expect(logicGroup?.get('expression')?.validator).toBeNull();
  });

  it('should correctly format policy data for saving', () => {
    component.openCreateModal();
    component.policyForm.patchValue({
      name: 'Test Policy',
      description: 'Test Desc'
    });
    
    component.savePolicy();
    
    expect(mockPolicyService.createPolicy).toHaveBeenCalledWith(jasmine.objectContaining({
      name: 'Test Policy',
      org_id: 'org-123'
    }));
  });
});
