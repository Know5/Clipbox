import { useEffect, useRef } from "react";
import { SetUIActivity } from "./api";

let activityQueue = Promise.resolve();
export function useUIActivity(busy: boolean) {
  const token = useRef(`draft-${Date.now()}-${Math.random().toString(36).slice(2)}`);
  useEffect(() => {
    const update = (value: boolean) => {
      activityQueue = activityQueue
        .catch(() => {})
        .then(() => SetUIActivity(token.current, value))
        .catch(() => {});
    };
    update(busy);
    return () => update(false);
  }, [busy]);
}
