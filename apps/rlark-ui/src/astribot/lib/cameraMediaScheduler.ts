export type StreamPriority = "dialog" | "pinned" | "card";

type Entry = {
  id: string;
  priority: StreamPriority;
  order: number;
  onChange: (granted: boolean) => void;
  active: boolean;
};

const priorityRank: Record<StreamPriority, number> = { card: 0, pinned: 1, dialog: 2 };

export function createCameraMediaScheduler(options: { maxLiveStreams?: number } = {}) {
  let maxLiveStreams = Math.max(0, Math.floor(options.maxLiveStreams ?? 3));
  let sequence = 0;
  const entries = new Map<string, Entry>();

  const assign = () => {
    const candidates = [...entries.values()].sort((a, b) => priorityRank[b.priority] - priorityRank[a.priority] || a.order - b.order);
    const winners = new Set(candidates.slice(0, maxLiveStreams));
    for (const entry of entries.values()) {
      const active = winners.has(entry);
      if (active !== entry.active) {
        entry.active = active;
        entry.onChange(active);
      }
    }
  };

  return {
    acquire(id: string, priority: StreamPriority, onChange: (granted: boolean) => void) {
      const existing = entries.get(id);
      const entry = { id, priority, order: existing?.order ?? sequence++, onChange, active: false };
      entries.set(id, entry);
      if (existing?.active) {
        existing.active = false;
        existing.onChange(false);
      }
      assign();
      return () => {
        if (entries.get(id) !== entry) return;
        entries.delete(id);
        if (entry.active) {
          entry.active = false;
          entry.onChange(false);
        }
        assign();
      };
    },
    promote(id: string, priority: StreamPriority) {
      const entry = entries.get(id);
      if (entry) { entry.priority = priority; assign(); }
    },
    setMaxLiveStreams(limit: number) {
      maxLiveStreams = Math.max(0, Math.floor(limit));
      assign();
    },
    getStats() {
      const active = [...entries.values()].filter((entry) => entry.active);
      return { activeCount: active.length, maxLiveStreams, queuedCount: entries.size - active.length, activeIds: active.map((entry) => entry.id) };
    },
    nextSnapshotDelayMs(baseMs = 5000, jitterMs = 1500, randomFn = Math.random) {
      return Math.max(1500, Math.round(baseMs + (randomFn() * 2 - 1) * jitterMs));
    },
  };
}

export const cameraMediaScheduler = createCameraMediaScheduler();
