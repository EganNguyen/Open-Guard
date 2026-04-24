export class OpenGuardError extends Error {
  public readonly code: string;
  public readonly phase: number;
  public readonly detectorId?: string;

  constructor(
    message: string,
    code: string,
    phase: number,
    detectorId?: string
  ) {
    super(message);
    this.name = 'OpenGuardError';
    this.code = code;
    this.phase = phase;
    this.detectorId = detectorId;
    Error.captureStackTrace(this, this.constructor);
  }
}

export class DetectorTimeoutError extends OpenGuardError {
  constructor(detectorId: string, timeoutMs: number) {
    super(
      `Detector ${detectorId} timed out after ${timeoutMs}ms`,
      'DETECTOR_TIMEOUT',
      5,
      detectorId
    );
    this.name = 'DetectorTimeoutError';
  }
}

export class StoreConnectionError extends OpenGuardError {
  constructor(storeName: string, originalError?: Error) {
    super(
      `Failed to connect to ${storeName}: ${originalError?.message || 'Unknown error'}`,
      'STORE_CONNECTION_FAILED',
      2,
    );
    this.name = 'StoreConnectionError';
  }
}