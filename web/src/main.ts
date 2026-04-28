import { bootstrapApplication } from '@angular/platform-browser';
import { appConfig } from './app/app.config';
import { App } from './app/app';
import { LoggingService } from './app/core/services/logging.service';

bootstrapApplication(App, appConfig)
  .then((appRef) => {
    // Log page load time after a short delay to ensure navigation entry is complete
    setTimeout(() => {
      const navigation = performance.getEntriesByType(
        'navigation',
      )[0] as PerformanceNavigationTiming;
      if (navigation) {
        const loadTime = navigation.duration / 1000; // seconds
        const loggingService = appRef.injector.get(LoggingService);
        loggingService.info(`Page loaded: ${window.location.pathname}`, {
          load_time: loadTime,
          page: window.location.pathname,
        });
      }
    }, 100);
  })
  .catch((err) => console.error(err));
