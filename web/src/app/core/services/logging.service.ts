import { Injectable, inject } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { environment } from '../../../environments/environment';

export interface LogPayload {
  level: 'info' | 'warn' | 'error' | 'debug';
  message: string;
  context?: Record<string, any>;
}

@Injectable({
  providedIn: 'root'
})
export class LoggingService {
  private http = inject(HttpClient);
  private apiUrl = environment.apiUrl;

  log(payload: LogPayload) {
    // Avoid circular logging if the log request itself fails
    if (payload.context?.['isLoggingRequest']) {
      return;
    }

    this.http.post(`${this.apiUrl}/v1/logs`, payload, {
      headers: { 'X-Skip-Logging': 'true' }
    }).subscribe({
      error: (err) => console.error('Failed to send log to server', err)
    });
  }

  info(message: string, context?: Record<string, any>) {
    this.log({ level: 'info', message, context });
  }

  warn(message: string, context?: Record<string, any>) {
    this.log({ level: 'warn', message, context });
  }

  error(message: string, context?: Record<string, any>) {
    this.log({ level: 'error', message, context });
  }

  debug(message: string, context?: Record<string, any>) {
    this.log({ level: 'debug', message, context });
  }
}
