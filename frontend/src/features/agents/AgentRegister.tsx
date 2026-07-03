import { useMutation } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { registerAgent } from "./agents.api";
import type { AccessTokenView, RegisterAgentRequest } from "./agents.types";

type AgentRegisterProps = {
  onRegistered: (tokenView: AccessTokenView) => void;
};

type FormState = {
  name: string;
  ownerEmail: string;
  description: string;
  team: string;
  scopes: string;
  repoScope: string;
  budgetCents: string;
  budgetCurrency: string;
};

const initialState: FormState = {
  name: "",
  ownerEmail: "",
  description: "",
  team: "",
  scopes: "tasks:read, tasks:write",
  repoScope: "",
  budgetCents: "",
  budgetCurrency: "USD",
};

function parseList(value: string) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

export function AgentRegister({ onRegistered }: AgentRegisterProps) {
  const [form, setForm] = useState<FormState>(initialState);
  const mutation = useMutation({
    mutationFn: (payload: RegisterAgentRequest) => registerAgent(payload),
    onSuccess: (result) => {
      onRegistered(result.data);
    },
  });

  function updateField<Key extends keyof FormState>(key: Key, value: FormState[Key]) {
    setForm((current) => ({ ...current, [key]: value }));
  }

  function buildPayload(): RegisterAgentRequest {
    return {
      name: form.name.trim(),
      owner_email: form.ownerEmail.trim(),
      description: form.description.trim() || undefined,
      team: form.team.trim() || undefined,
      scopes: parseList(form.scopes),
      repo_scope: parseList(form.repoScope),
      budget_cents: form.budgetCents ? Number(form.budgetCents) : undefined,
      budget_currency: form.budgetCurrency.trim() || undefined,
    };
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    await mutation.mutateAsync(buildPayload());
  }

  const createdAgent = mutation.data?.data.agent;

  return (
    <section className="agent-register-panel" aria-label="注册 Agent">
      <div className="agent-register-copy">
        <h2>注册新 Agent</h2>
        <p>创建新的执行身份，签发一次性 activation token，并控制可访问的仓库与预算。</p>
      </div>
      <form className="agent-form" onSubmit={handleSubmit}>
        <label>
          名称
          <input
            value={form.name}
            onChange={(event) => updateField("name", event.target.value)}
            required
            placeholder="Code Review Bot"
          />
        </label>
        <label>
          Owner Email
          <input
            type="email"
            value={form.ownerEmail}
            onChange={(event) => updateField("ownerEmail", event.target.value)}
            required
            placeholder="review@example.com"
          />
        </label>
        <label className="field-span-2">
          描述
          <textarea
            rows={3}
            value={form.description}
            onChange={(event) => updateField("description", event.target.value)}
            placeholder="描述这个 Agent 的职责边界"
          />
        </label>
        <label>
          Team
          <input value={form.team} onChange={(event) => updateField("team", event.target.value)} placeholder="Platform" />
        </label>
        <label>
          Scopes
          <input
            value={form.scopes}
            onChange={(event) => updateField("scopes", event.target.value)}
            placeholder="tasks:read, tasks:write"
          />
        </label>
        <label className="field-span-2">
          Repo Scope
          <input
            value={form.repoScope}
            onChange={(event) => updateField("repoScope", event.target.value)}
            placeholder="billing-service, frontend"
          />
        </label>
        <label>
          预算上限（分）
          <input
            type="number"
            min="0"
            value={form.budgetCents}
            onChange={(event) => updateField("budgetCents", event.target.value)}
            placeholder="2000"
          />
        </label>
        <label>
          币种
          <input
            value={form.budgetCurrency}
            onChange={(event) => updateField("budgetCurrency", event.target.value)}
            placeholder="USD"
          />
        </label>
        <div className="agent-form-actions field-span-2">
          <button type="submit" className="primary-action" disabled={mutation.isPending}>
            {mutation.isPending ? "注册中…" : "注册 Agent"}
          </button>
          {createdAgent ? (
            <Link className="secondary-action" to={`/agents/${createdAgent.id}`}>
              查看详情
            </Link>
          ) : null}
        </div>
        {mutation.isError ? <p className="error">{mutation.error.message}</p> : null}
      </form>
    </section>
  );
}
