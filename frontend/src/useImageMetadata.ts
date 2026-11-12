import { useEffect, useRef, useState } from "react";
import { GetClipImageMeta } from "./api";
import type { ClipEntry } from "./clipHistory";

export interface ClipImageMeta {
  id: number;
  width: number;
  height: number;
  bytes: number;
}

interface CacheEntry {
  promise?: Promise<void>;
  value?: ClipImageMeta | null;
  timestamp?: number;
  clip?: ClipEntry;
}

export function useImageMetadata(
  clips: ClipEntry[],
  load: (ids: number[]) => Promise<ClipImageMeta[]> = GetClipImageMeta,
) {
  const cache = useRef(new Map<number, CacheEntry>());
  const [metadata, setMetadata] = useState<Record<number, ClipImageMeta>>({});

  useEffect(() => {
    let active = true;
    const ids = clips.filter((clip) => clip.type === "image").map((clip) => clip.id);
    const wanted = new Set(ids);
    for (const id of cache.current.keys()) {
      if (!wanted.has(id)) cache.current.delete(id);
    }
    for (const clip of clips) {
      const cached = cache.current.get(clip.id);
      if (
        cached &&
        !cached.promise &&
        (cached.timestamp !== clip.timestamp || (cached.value === null && cached.clip !== clip))
      )
        cache.current.delete(clip.id);
    }

    const missing = ids.filter((id) => !cache.current.has(id));
    if (missing.length > 0) {
      const entries = new Map(
        missing.map((id) => [
          id,
          {
            timestamp: clips.find((clip) => clip.id === id)?.timestamp,
            clip: clips.find((clip) => clip.id === id),
          } as CacheEntry,
        ]),
      );
      const request = Promise.resolve()
        .then(() => load(missing))
        .then((items) => {
          const results = new Map((items || []).map((item) => [item.id, item]));
          for (const [id, entry] of entries) {
            if (cache.current.get(id) !== entry) continue;
            entry.value = results.get(id) || null;
            delete entry.promise;
          }
        })
        .catch((error) => {
          for (const [id, entry] of entries) {
            if (cache.current.get(id) === entry) cache.current.delete(id);
          }
          console.error(error);
        });
      for (const [id, entry] of entries) {
        entry.promise = request;
        cache.current.set(id, entry);
      }
    }

    const publish = () => {
      if (!active) return;
      const next: Record<number, ClipImageMeta> = {};
      for (const id of ids) {
        const value = cache.current.get(id)?.value;
        if (value) next[id] = value;
      }
      setMetadata((previous) =>
        Object.keys(previous).length === Object.keys(next).length &&
        Object.keys(next).every((id) => previous[Number(id)] === next[Number(id)])
          ? previous
          : next,
      );
    };
    publish();
    const pending = new Set(
      ids.map((id) => cache.current.get(id)?.promise).filter(Boolean) as Promise<void>[],
    );
    // The current effect subscribes to existing requests, including ones that
    // started before a list update or StrictMode's effect cleanup. Completing
    // a request updates the cache independently of any one subscriber.
    for (const request of pending) void request.then(publish);
    return () => {
      active = false;
    };
  }, [clips, load]);

  return metadata;
}
