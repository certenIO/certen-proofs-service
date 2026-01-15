// Async data fetching hook (adapted from Accumulate Explorer)
import { useState, useEffect, useCallback } from 'react';

export interface AsyncState<T> {
  data?: T;
  loading: boolean;
  error?: Error;
  reload: () => void;
}

export function useAsync<T>(
  asyncFn: () => Promise<T>,
  deps: unknown[] = []
): AsyncState<T> {
  const [state, setState] = useState<{
    data?: T;
    loading: boolean;
    error?: Error;
  }>({
    loading: true,
  });

  const execute = useCallback(async () => {
    setState((prev) => ({ ...prev, loading: true, error: undefined }));
    try {
      const data = await asyncFn();
      setState({ data, loading: false });
    } catch (error) {
      setState({ loading: false, error: error as Error });
    }
  }, deps);

  useEffect(() => {
    execute();
  }, [execute]);

  return {
    ...state,
    reload: execute,
  };
}

export function useAsyncCallback<T, A extends unknown[]>(
  asyncFn: (...args: A) => Promise<T>
): {
  execute: (...args: A) => Promise<T | undefined>;
  data?: T;
  loading: boolean;
  error?: Error;
} {
  const [state, setState] = useState<{
    data?: T;
    loading: boolean;
    error?: Error;
  }>({
    loading: false,
  });

  const execute = useCallback(
    async (...args: A) => {
      setState((prev) => ({ ...prev, loading: true, error: undefined }));
      try {
        const data = await asyncFn(...args);
        setState({ data, loading: false });
        return data;
      } catch (error) {
        setState({ loading: false, error: error as Error });
        return undefined;
      }
    },
    [asyncFn]
  );

  return {
    execute,
    ...state,
  };
}

export default useAsync;
