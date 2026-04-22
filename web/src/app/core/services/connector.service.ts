import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';
import { Observable, map } from 'rxjs';

export interface Connector {
  id: string;
  name: string;
  redirect_uris: string[];
  client_secret?: string;
  status?: string;
  scopes?: string[];
  createdDate?: string;
  eventVolume?: string;
}

@Injectable({
  providedIn: 'root'
})
export class ConnectorService {
  private api = inject(ApiService);

  listConnectors(): Observable<Connector[]> {
    return this.api.get<any[]>('/mgmt/connectors').pipe(
      map(data => Array.isArray(data) ? data.map(c => ({
        id: c.id,
        name: c.name,
        redirect_uris: c.redirect_uris,
        status: 'Active',
        scopes: ['openid', 'profile', 'email'],
        createdDate: 'Apr 17, 2026',
        eventVolume: '0'
      })) : [])
    );
  }

  createConnector(connector: any): Observable<any> {
    return this.api.post('/mgmt/connectors', connector);
  }

  updateConnector(id: string, connector: any): Observable<any> {
    return this.api.put(`/mgmt/connectors/${id}`, connector);
  }

  deleteConnector(id: string): Observable<any> {
    return this.api.delete(`/mgmt/connectors/${id}`);
  }
}
