import { useState, useRef, useCallback, useEffect } from 'react';

export function useBufferedStreaming() {
  const [displayText, setDisplayText] = useState('');
  const textBufferRef = useRef('');
  const rafIdRef = useRef<number>(0);

  const flush = useCallback(() => {
    setDisplayText(textBufferRef.current);
    rafIdRef.current = 0;
  }, []);

  const appendText = useCallback((text: string, needsSep: boolean) => {
    if (needsSep && textBufferRef.current.length > 0) {
      textBufferRef.current += '\n\n' + text;
    } else {
      textBufferRef.current += text;
    }
    if (!rafIdRef.current) {
      rafIdRef.current = requestAnimationFrame(flush);
    }
  }, [flush]);

  const setTextDirect = useCallback((text: string) => {
    textBufferRef.current = text;
    setDisplayText(text);
  }, []);

  const reset = useCallback(() => {
    if (rafIdRef.current) {
      cancelAnimationFrame(rafIdRef.current);
      rafIdRef.current = 0;
    }
    textBufferRef.current = '';
    setDisplayText('');
  }, []);

  useEffect(() => {
    return () => {
      if (rafIdRef.current) {
        cancelAnimationFrame(rafIdRef.current);
      }
    };
  }, []);

  return { displayText, appendText, setTextDirect, reset };
}
