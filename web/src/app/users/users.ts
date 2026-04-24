import { Component, OnInit, signal, inject } from '@angular/core';
import { CommonModule } from '@angular/common';
import { HttpClient } from '@angular/common/http';
import { FormsModule, ReactiveFormsModule, FormBuilder, FormGroup, Validators } from '@angular/forms';
import { forkJoin, of, catchError } from 'rxjs';
import { environment } from '../../environments/environment';
import { User } from '../core/models/user.model';
import { Connector } from '../core/models/connector.model';


@Component({
  selector: 'app-users',
  standalone: true,
  imports: [CommonModule, FormsModule, ReactiveFormsModule],
  templateUrl: './users.html',
  styleUrl: './users.css'
})
export class UsersComponent implements OnInit {
  private http = inject(HttpClient);
  private fb = inject(FormBuilder);

  connectors = signal<Connector[]>([]);
  users = signal<User[]>([]);
  groupedData = signal<{ connector: Partial<Connector>, users: User[] }[]>([]);
  loading = signal(true);
  error = signal('');
  showModal = signal(false);
  submitting = signal(false);
  
  userForm: FormGroup = this.fb.group({
    org_id: ['', Validators.required],
    email: ['', [Validators.required, Validators.email]],
    password: ['', [Validators.required, Validators.minLength(6)]],
    display_name: ['', Validators.required],
    role: ['user', Validators.required]
  });

  ngOnInit() {
    this.fetchData();
  }

  fetchData() {
    this.loading.set(true);
    this.error.set('');
    
    forkJoin({
      connectors: this.http.get<Connector[]>(`${environment.apiUrl}/mgmt/connectors`).pipe(catchError(() => of([]))),
      users: this.http.get<User[]>(`${environment.apiUrl}/mgmt/users`).pipe(catchError(() => of([])))
    }).subscribe({
      next: (res) => {
        this.connectors.set(res.connectors || []);
        this.users.set(res.users || []);
        this.groupUsers();
        this.loading.set(false);
      },
      error: (err) => {
        this.error.set('Failed to load data');
        this.loading.set(false);
        console.error(err);
      }
    });
  }

  groupUsers() {
    const connectors = this.connectors();
    const users = this.users();

    const grouped = connectors.map(conn => ({
      connector: conn,
      users: users.filter(u => u.org_id === conn.org_id)
    }));

    // Add users without a matching connector organization
    const matchedUserIds = new Set(grouped.flatMap(g => g.users.map(u => u.id)));
    const remainingUsers = users.filter(u => !matchedUserIds.has(u.id));

    if (remainingUsers.length > 0) {
      grouped.unshift({
        connector: { name: 'System Administration', id: 'system', description: 'Internal OpenGuard control plane users' },
        users: remainingUsers
      });
    }

    this.groupedData.set(grouped);
  }

  getRoleClass(role: string) {
    if (!role) return 'bg-gray-100 text-gray-700';
    switch (role.toLowerCase()) {
      case 'admin': return 'bg-purple-100 text-purple-700';
      case 'editor': return 'bg-blue-100 text-blue-700';
      default: return 'bg-gray-100 text-gray-700';
    }
  }

  openModal() {
    this.showModal.set(true);
    this.userForm.reset({ role: 'user', org_id: '' });
  }

  closeModal() {
    this.showModal.set(false);
  }

  onCreateUser() {
    if (this.userForm.invalid) return;

    this.submitting.set(true);
    this.http.post(`${environment.apiUrl}/mgmt/users`, this.userForm.value).subscribe({
      next: () => {
        this.submitting.set(false);
        this.closeModal();
        this.fetchData();
      },
      error: (err) => {
        this.submitting.set(false);
        console.error('Failed to create user', err);
        alert('Failed to create user: ' + (err.error || err.message));
      }
    });
  }
}
