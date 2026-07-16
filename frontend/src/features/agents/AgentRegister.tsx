import { useMutation } from "@tanstack/react-query";
import { useRef, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { createIdempotencyKey, shouldRetainMutationKey } from "../../api/client";
import { registerAgent } from "./agents.api";
import type { RegisterAgentRequest, RegisterAgentResponse } from "./agents.types";

type AgentRegisterProps = {
  onRegistered: (response: RegisterAgentResponse) => void;
};

type FormState = {
  name: string;
  description: string;
  team: string;
  scopes: string;
  repoScope: string;
  budgetCents: string;
  budgetCurrency: string;
};

const initialState: FormState = {
  name: "",
  description: "",
  team: "",
  scopes: "tasks:publish, tasks:claim, tasks:execute, tasks:read",
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
  const registrationKey = useRef<string | null>(null);
  const mutation = useMutation({
    mutationFn: (payload: RegisterAgentRequest) => {
      registrationKey.current ??= createIdempotencyKey();
      return registerAgent(payload, { idempotencyKey: registrationKey.current });
    },
    onSuccess: (result) => {
      registrationKey.current = null;
      onRegistered(result.data);
    },
    onError: (error) => {
      if (!shouldRetainMutationKey(error)) registrationKey.current = null;
    },
  });

  function updateField<Key extends keyof FormState>(key: Key, value: FormState[Key]) {
    registrationKey.current = null;
    setForm((current) => ({ ...current, [key]: value }));
  }

  function buildPayload(): RegisterAgentRequest {
    return {
      name: form.name.trim(),
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
    try {
      await mutation.mutateAsync(buildPayload());
    } catch {
      // Mutation state already captures the error for local recovery UI.
    }
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
            placeholder="tasks:publish, tasks:claim, tasks:execute, tasks:read"
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
        {mutation.isError ? (
          <p className="form-error field-span-2" role="alert">
            {mutation.error.message}
          </p>
        ) : null}
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
      </form>
    </section>
  );
}
