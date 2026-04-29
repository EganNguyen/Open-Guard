import { Routes } from '@angular/router';
import { HomeComponent } from './home/home';
import { ConnectorsComponent } from './connectors/connectors';
import { UsersComponent } from './users/users';
import { PoliciesComponent } from './policies/policies';
import { AuditLogComponent } from './audit-logs/audit-logs';
import { ThreatsComponent } from './threats/threats';
import { ComplianceComponent } from './compliance/compliance';
import { DlpComponent } from './dlp/dlp';
import { AdminComponent } from './admin/admin';
import { LoginComponent } from './features/login/login';
import { LayoutComponent } from './core/layout/layout';
import { authGuard } from './core/guards/auth.guard';

export const routes: Routes = [
  { path: 'login', component: LoginComponent },
  {
    path: '',
    component: LayoutComponent,
    canActivate: [authGuard],
    children: [
      { path: '', component: HomeComponent },
      { path: 'connectors', component: ConnectorsComponent },
      { path: 'users', component: UsersComponent },
      { path: 'policies', component: PoliciesComponent },
      { path: 'audit', component: AuditLogComponent },
      { path: 'threats', component: ThreatsComponent },
      { path: 'compliance', component: ComplianceComponent },
      { path: 'dlp', component: DlpComponent },
      { path: 'admin', component: AdminComponent },
      { path: 'mfa/totp', loadComponent: () => import('./features/mfa/totp.component').then(m => m.TotpComponent) },
      { path: 'mfa/webauthn', loadComponent: () => import('./features/mfa/webauthn.component').then(m => m.WebauthnComponent) },
      { path: 'org/settings', loadComponent: () => import('./features/org-settings/org-settings.component').then(m => m.OrgSettingsComponent) },
      { path: 'connectors/:id', loadComponent: () => import('./connectors/connector-detail/connector-detail.component').then(m => m.ConnectorDetailComponent) },
    ],
  },
  { path: '**', redirectTo: '' },
];
