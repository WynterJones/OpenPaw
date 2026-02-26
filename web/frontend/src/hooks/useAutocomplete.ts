import { useState, useRef } from 'react';

export function useAutocomplete() {
  const [mentionOpen, setMentionOpen] = useState(false);
  const [mentionFilter, setMentionFilter] = useState('');
  const [mentionIndex, setMentionIndex] = useState(0);
  const mentionAnchorRef = useRef<number | null>(null);

  const [contextOpen, setContextOpen] = useState(false);
  const [contextFilter, setContextFilter] = useState('');
  const [contextIndex, setContextIndex] = useState(0);
  const contextAnchorRef = useRef<number | null>(null);

  const [mediaOpen, setMediaOpen] = useState(false);
  const [mediaFilter, setMediaFilter] = useState('');
  const [mediaIndex, setMediaIndex] = useState(0);
  const mediaAnchorRef = useRef<number | null>(null);

  return {
    mentionOpen, setMentionOpen,
    mentionFilter, setMentionFilter,
    mentionIndex, setMentionIndex,
    mentionAnchorRef,
    contextOpen, setContextOpen,
    contextFilter, setContextFilter,
    contextIndex, setContextIndex,
    contextAnchorRef,
    mediaOpen, setMediaOpen,
    mediaFilter, setMediaFilter,
    mediaIndex, setMediaIndex,
    mediaAnchorRef,
  };
}
