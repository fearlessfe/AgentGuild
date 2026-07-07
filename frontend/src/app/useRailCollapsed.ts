import { useCallback, useEffect, useState } from "react";

/* Persists whether the navigation rail is collapsed (icon-only) or expanded
   (icon + label). Mirrors the localStorage pattern used by ThemeProvider so the
   choice survives reloads and degrades gracefully when storage is unavailable. */

const STORAGE_KEY = "agentguild-rail-collapsed";

function readStored(): boolean {
  if (typeof window === "undefined") return true;
  try {
    return window.localStorage.getItem(STORAGE_KEY) !== "false";
  } catch {
    // localStorage may be unavailable (private mode); default to collapsed.
    return true;
  }
}

export function useRailCollapsed(): { collapsed: boolean; toggle: () => void } {
  const [collapsed, setCollapsed] = useState<boolean>(readStored);

  useEffect(() => {
    try {
      window.localStorage.setItem(STORAGE_KEY, String(collapsed));
    } catch {
      // Ignore persistence failures; the in-memory state still applies.
    }
  }, [collapsed]);

  const toggle = useCallback(() => setCollapsed((prev) => !prev), []);

  return { collapsed, toggle };
}
