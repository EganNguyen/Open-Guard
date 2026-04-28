import { ErrorHandler, Injectable, inject } from '@angular/core';
import { LoggingService } from './services/logging.service';

@Injectable()
export class GlobalErrorHandler implements ErrorHandler {
  private loggingService = inject(LoggingService);

  handleError(error: any): void {
    const message = error.message ? error.message : error.toString();

    // Log to console as well
    console.error('Global Error Handler:', error);

    // Send to Loki via our LoggingService
    this.loggingService.error(message, {
      stack: error.stack,
      url: window.location.href,
      userAgent: navigator.userAgent,
      timestamp: new Date().toISOString(),
    });
  }
}
