import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { diffVersion, getVersion, listVersions } from "./versions.api";
import type { VersionView } from "./versions.types";

function formatStatus(status: string) {
  const map: Record<string, string> = {
    draft: "草稿",
    evaluating: "评测中",
    eligible: "可晋级",
    active: "当前",
    retired: "已退役",
    rejected: "已拒绝",
  };
  return map[status] ?? status;
}

export function VersionTree({ agentId, onSelect }: { agentId: string; onSelect?: (v: VersionView) => void }) {
  const query = useQuery({ queryKey: ["versions", agentId], queryFn: () => listVersions(agentId) });

  if (query.isPending) return <div className="loading">加载版本列表…</div>;
  if (query.isError) return <div className="error">无法读取版本：{query.error.message}</div>;

  const versions = query.data.data.items;
  const active = versions.find((v) => v.status === "active");

  return (
    <section className="version-tree" aria-label="版本谱系">
      <h3>版本谱系</h3>
      {active ? <p>当前版本：<strong>#{active.version_number}</strong></p> : null}
      <ul className="version-list">
        {versions.map((v) => (
          <li key={v.id} className={`version-item ${v.status}`}>
            <button type="button" className="link" onClick={() => onSelect?.(v)}>
              #{v.version_number} {formatStatus(v.status)}
            </button>
            <span className="version-meta">{v.runtime} / {v.model}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function VersionDetail({ agentId, versionId }: { agentId: string; versionId: string }) {
  const query = useQuery({ queryKey: ["version", agentId, versionId], queryFn: () => getVersion(agentId, versionId) });
  const [baseId, setBaseId] = useState("");

  if (query.isPending) return <div className="loading">加载版本详情…</div>;
  if (query.isError) return <div className="error">无法读取版本：{query.error.message}</div>;

  const v = query.data.data;

  return (
    <section className="version-detail" aria-label="版本详情">
      <h3>版本 #{v.version_number}</h3>
      <dl className="meta-grid">
        <div><dt>状态</dt><dd>{formatStatus(v.status)}</dd></div>
        <div><dt>Runtime</dt><dd>{v.runtime}</dd></div>
        <div><dt>Model</dt><dd>{v.model}</dd></div>
        <div><dt>内容哈希</dt><dd>{v.content_hash.slice(0, 16)}…</dd></div>
      </dl>
      <VersionDiffView agentId={agentId} versionId={versionId} baseVersionId={baseId} onBaseChange={setBaseId} />
    </section>
  );
}

function VersionDiffView({
  agentId,
  versionId,
  baseVersionId,
  onBaseChange,
}: {
  agentId: string;
  versionId: string;
  baseVersionId: string;
  onBaseChange: (id: string) => void;
}) {
  const query = useQuery({
    queryKey: ["version-diff", agentId, versionId, baseVersionId],
    queryFn: () => diffVersion(agentId, versionId, baseVersionId || undefined),
    enabled: !!baseVersionId,
  });

  return (
    <div className="version-diff">
      <label>
        对比基线版本 ID：
        <input type="text" value={baseVersionId} onChange={(e) => onBaseChange(e.target.value)} placeholder="父版本或当前版本" />
      </label>
      {query.isPending ? <div className="loading">计算差异…</div> : null}
      {query.isError ? <div className="error">{query.error.message}</div> : null}
      {query.data ? <pre>{JSON.stringify(query.data.data, null, 2)}</pre> : null}
    </div>
  );
}
