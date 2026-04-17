import { Routes } from '@angular/router';
import { HomeComponent } from './home/home';
import { ConnectorsComponent } from './connectors/connectors';

export const routes: Routes = [
  { path: '', component: HomeComponent },
  { path: 'connectors', component: ConnectorsComponent },
  { path: '**', redirectTo: '' }
];
