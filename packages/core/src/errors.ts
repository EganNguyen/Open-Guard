export class OpenGuardError extends Error {
  code: string;
  phase: number;
  detectorId?: string;

  constructor(message: string, code: string, phase: number, detectorId?: string) {
    super(message);
    this.name = 'OpenGuardError';
    this.code = code;
    this.phase = phase;
    this.detectorId = detectorId;
  }
}

export class DetectorTimeoutError extends OpenGuardError {
  constructor(detectorId: string, timeoutMs: number) {
    super(
      `Detector ${detectorId} timed out after ${timeoutMs}ms`,
      'DETECTOR_TIMEOUT',
      2,
      detectorId
    );
    this.name = 'DetectorTimeoutError';
  }
}

export class StoreConnectionError extends OpenGuardError {
  constructor(storeType: string, cause?: Error) {
    const message = cause
      ? `Failed to connect to ${storeType} store: ${cause.message}`
      : `Failed to connect to ${storeType} store`;
    super(message, 'STORE_CONNECTION_FAILED', 2);
    this.name = 'StoreConnectionError';
    if (cause) this.cause = cause;
  }
}