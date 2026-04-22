import { Routes } from '@angular/router';
import { HomeComponent } from './home/home';
import { ConnectorsComponent } from './connectors/connectors';
import { UsersComponent } from './users/users';
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
    ]
  },
  { path: '**', redirectTo: '' }
];
