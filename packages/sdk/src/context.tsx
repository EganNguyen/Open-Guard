import React, { createContext, useContext, ReactNode } from 'react';
import { OpenGuardClient } from './client';

interface GuardContextValue {
  client: OpenGuardClient;
}

const GuardContext = createContext<GuardContextValue | null>(null);

export interface GuardProviderProps {
  client: OpenGuardClient;
  children: ReactNode;
}

export function GuardProvider({ client, children }: GuardProviderProps): JSX.Element {
  return (
    <GuardContext.Provider value={{ client }}>
      {children}
    </GuardContext.Provider>
  );
}

export function useGuard(): OpenGuardClient {
  const context = useContext(GuardContext);
  if (!context) {
    throw new Error('useGuard must be used within a GuardProvider');
  }
  return context.client;
}

export { GuardContext };