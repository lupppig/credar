"use client";

import { useEffect, useState } from "react";

type FlowKind =
  "login" | "registration" | "recovery" | "verification" | "settings";

type UiMessage = { id: number; text: string; type: string };
type UiNode = {
  type: string;
  group: string;
  attributes: {
    name?: string;
    type?: string;
    value?: string | number | boolean;
    required?: boolean;
    disabled?: boolean;
    autocomplete?: string;
  };
  messages?: UiMessage[];
  meta?: { label?: { text?: string } };
};
type SelfServiceFlow = {
  id: string;
  ui: {
    action: string;
    method: string;
    messages?: UiMessage[];
    nodes: UiNode[];
  };
};

const kratosBrowser =
  process.env.NEXT_PUBLIC_CREDAR_KRATOS_BROWSER_URL ?? "http://localhost:4433";

const flowCopy: Record<
  FlowKind,
  { eyebrow: string; title: string; detail: string; submit: string }
> = {
  login: {
    eyebrow: "Secure workspace access",
    title: "Welcome back.",
    detail: "Sign in with your verified Credar identity.",
    submit: "Sign in",
  },
  registration: {
    eyebrow: "Create your identity",
    title: "Start with a secure foundation.",
    detail: "Create the identity you will use across Credar workspaces.",
    submit: "Create account",
  },
  recovery: {
    eyebrow: "Account recovery",
    title: "Recover access.",
    detail: "Request a recovery message for your registered email address.",
    submit: "Send recovery link",
  },
  verification: {
    eyebrow: "Identity verification",
    title: "Verify your email.",
    detail: "Confirm your address to keep your identity trusted.",
    submit: "Send verification link",
  },
  settings: {
    eyebrow: "Identity settings",
    title: "Manage your account.",
    detail: "Update the traits and credentials attached to your identity.",
    submit: "Save changes",
  },
};

export function AuthFlow({
  kind,
  flowId,
}: {
  kind: FlowKind;
  flowId?: string;
}) {
  const [flow, setFlow] = useState<SelfServiceFlow | null>(null);
  const [error, setError] = useState<string | null>(null);
  const copy = flowCopy[kind];

  useEffect(() => {
    if (!flowId) {
      const destination = new URL(
        `/self-service/${kind}/browser`,
        kratosBrowser,
      );
      destination.searchParams.set(
        "return_to",
        `${window.location.origin}${kind === "settings" ? "/console" : kind === "login" || kind === "registration" ? "/console" : "/auth/login"}`,
      );
      window.location.replace(destination.toString());
      return;
    }

    const controller = new AbortController();
    fetch(
      `${kratosBrowser}/self-service/${kind}/flows?id=${encodeURIComponent(flowId)}`,
      {
        credentials: "include",
        cache: "no-store",
        signal: controller.signal,
      },
    )
      .then(async (response) => {
        if (!response.ok) {
          throw new Error(
            response.status === 410
              ? "This flow expired. Start again to continue."
              : "The identity service could not load this flow.",
          );
        }
        return (await response.json()) as SelfServiceFlow;
      })
      .then(setFlow)
      .catch((reason: unknown) => {
        if (reason instanceof DOMException && reason.name === "AbortError")
          return;
        setError(
          reason instanceof Error
            ? reason.message
            : "Unable to load this flow.",
        );
      });
    return () => controller.abort();
  }, [flowId, kind]);

  return (
    <section className="auth-shell">
      <div className="auth-context" aria-hidden="true">
        <span className="auth-context-label">CREDAR / IDENTITY</span>
        <div>
          <strong>Access begins with proof.</strong>
          <p>
            Every session is verified before tenant scope and permissions are
            resolved.
          </p>
        </div>
        <div className="auth-proof">
          <span>01 · Verify identity</span>
          <span>02 · Resolve tenant</span>
          <span>03 · Evaluate policy</span>
        </div>
      </div>
      <div className="auth-panel">
        <p className="eyebrow">{copy.eyebrow}</p>
        <h1>{copy.title}</h1>
        <p className="auth-detail">{copy.detail}</p>

        {!flow && !error && (
          <div className="auth-loading" role="status">
            <span /> Connecting to the identity service…
          </div>
        )}

        {error && (
          <div className="auth-error" role="alert">
            <strong>Unable to continue</strong>
            <span>{error}</span>
            <a href={`/auth/${kind}`}>Start a new flow</a>
          </div>
        )}

        {flow && (
          <form
            className="auth-form"
            action={flow.ui.action}
            method={flow.ui.method === "GET" ? "get" : "post"}
          >
            <FlowMessages messages={flow.ui.messages} />
            {flow.ui.nodes.map((node, index) => (
              <FlowNode
                key={`${node.attributes.name ?? node.type}-${index}`}
                node={node}
                fallbackSubmit={copy.submit}
              />
            ))}
          </form>
        )}

        <AuthAlternatives kind={kind} />
      </div>
    </section>
  );
}

function FlowNode({
  node,
  fallbackSubmit,
}: {
  node: UiNode;
  fallbackSubmit: string;
}) {
  const attributes = node.attributes;
  const inputType = attributes.type ?? "text";
  const name = attributes.name;
  const label = node.meta?.label?.text;

  if (node.type !== "input" || !name) return null;

  if (inputType === "hidden") {
    return (
      <input type="hidden" name={name} value={String(attributes.value ?? "")} />
    );
  }

  if (inputType === "submit") {
    return (
      <button
        className="primary-action auth-submit"
        type="submit"
        name={name}
        value={String(attributes.value ?? "")}
        disabled={attributes.disabled}
      >
        {label ?? fallbackSubmit}
      </button>
    );
  }

  return (
    <label className="auth-field">
      <span>{label ?? humanize(name)}</span>
      <input
        name={name}
        type={inputType}
        defaultValue={
          typeof attributes.value === "string" ? attributes.value : undefined
        }
        required={attributes.required}
        disabled={attributes.disabled}
        autoComplete={attributes.autocomplete}
      />
      <FlowMessages messages={node.messages} />
    </label>
  );
}

function FlowMessages({ messages }: { messages?: UiMessage[] }) {
  if (!messages?.length) return null;
  return (
    <div className="flow-messages" role="alert">
      {messages.map((message) => (
        <p key={message.id}>{message.text}</p>
      ))}
    </div>
  );
}

function AuthAlternatives({ kind }: { kind: FlowKind }) {
  if (kind === "login") {
    return (
      <div className="auth-alternatives">
        <a href="/auth/recovery">Forgot password?</a>
        <span>
          New to Credar? <a href="/auth/registration">Create an account</a>
        </span>
      </div>
    );
  }
  if (kind === "registration") {
    return (
      <div className="auth-alternatives">
        <span>
          Already registered? <a href="/auth/login">Sign in</a>
        </span>
      </div>
    );
  }
  return (
    <div className="auth-alternatives">
      <a href="/auth/login">Return to sign in</a>
      <a href="/console">Return to console</a>
    </div>
  );
}

function humanize(value: string) {
  return value
    .replace(/^traits\./, "")
    .replaceAll("_", " ")
    .replace(/^./, (character) => character.toUpperCase());
}
