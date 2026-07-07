import type { ReactNode } from "react";

export type TimelineEvent = {
  time: ReactNode;
  title: ReactNode;
  note?: ReactNode;
};

export function Timeline({ events }: { events: TimelineEvent[] }) {
  return (
    <ul className="timeline">
      {events.map((event, index) => (
        <li key={index}>
          <div className="tl-time">{event.time}</div>
          <div className="tl-title">{event.title}</div>
          {event.note != null ? <div className="tl-note">{event.note}</div> : null}
        </li>
      ))}
    </ul>
  );
}
