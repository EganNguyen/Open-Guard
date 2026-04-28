import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';
import { Observable } from 'rxjs';
import { HttpParams } from '@angular/common/http';

export interface User {
  id: string;
  org_id: string;
  email: string;
  display_name: string;
  role: string;
  status: string;
  scim_external_id?: string;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface PagedResponse<T> {
  Resources: T[];
  totalResults: number;
  itemsPerPage: number;
  startIndex: number;
}

@Injectable({ providedIn: 'root' })
export class UserService {
  private api = inject(ApiService);

  listUsers(filter?: string): Observable<PagedResponse<User>> {
    let params = new HttpParams();
    if (filter) {
      params = params.set('filter', filter);
    }
    return this.api.get<PagedResponse<User>>('/v1/scim/v2/Users', params);
  }

  getUser(id: string): Observable<User> {
    return this.api.get<User>(`/v1/scim/v2/Users/${id}`);
  }

  suspendUser(id: string): Observable<void> {
    return this.api.post<void>(`/v1/users/${id}/suspend`, {});
  }

  activateUser(id: string): Observable<void> {
    return this.api.post<void>(`/v1/users/${id}/activate`, {});
  }

  reprovisionUser(id: string): Observable<void> {
    return this.api.post<void>(`/v1/users/${id}/reprovision`, {});
  }

  revokeAllSessions(userId: string): Observable<void> {
    return this.api.post<void>(`/v1/users/${userId}/sessions/revoke-all`, {});
  }
}
