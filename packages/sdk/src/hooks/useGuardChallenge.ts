import { useState, useCallback } from 'react';
import type { ChallengePayload } from '../types';

export interface UseGuardChallengeResult {
  pendingChallenge: ChallengePayload | null;
  resolve: (token: string) => Promise<boolean>;
  dismiss: () => void;
}

export function useGuardChallenge(): UseGuardChallengeResult {
  const [pendingChallenge, setPendingChallenge] = useState<ChallengePayload | null>(null);

  const resolve = useCallback(async (token: string): Promise<boolean> => {
    if (!pendingChallenge) {
      return false;
    }

    if (pendingChallenge.expiresAt < Date.now()) {
      setPendingChallenge(null);
      return false;
    }

    setPendingChallenge(null);
    return true;
  }, [pendingChallenge]);

  const dismiss = useCallback(() => {
    setPendingChallenge(null);
  }, []);

  return {
    pendingChallenge,
    resolve,
    dismiss,
  };
}

export function triggerChallenge(challenge: ChallengePayload): void {
  if (challenge.type === 'captcha') {
    if (typeof window !== 'undefined' && ' grecaptcha' in window) {
      console.warn('reCAPTCHA not configured');
    }
  }
  console.info('Challenge triggered:', challenge);
}