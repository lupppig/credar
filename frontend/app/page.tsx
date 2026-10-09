export default function Home() {
  return (
    <div className="page-stack">
      <section className="hero" aria-labelledby="page-title">
        <p className="eyebrow">Platform foundation</p>
        <h1 id="page-title">
          Operational foundations, ready for product work.
        </h1>
        <p className="summary">
          Credar is one multi-tenant credit platform for businesses that borrow,
          finance their customers, or do both. This console currently exposes
          the runtime foundation without implying unfinished product workflows.
        </p>
      </section>

      <section aria-labelledby="foundation-heading">
        <div className="section-heading">
          <div>
            <p className="eyebrow">Current capability</p>
            <h2 id="foundation-heading">A coherent local platform spine</h2>
          </div>
          <span className="status-chip">Runtime spine</span>
        </div>
        <div className="capability-grid">
          <article className="capability-card">
            <span className="card-index">01</span>
            <h3>Runtime health</h3>
            <p>
              Process liveness and dependency-aware readiness stay distinct,
              with bounded shutdown and correlated requests.
            </p>
          </article>
          <article className="capability-card">
            <span className="card-index">02</span>
            <h3>Committed events</h3>
            <p>
              PostgreSQL transactions publish through a leased outbox into
              durable, idempotent JetStream consumers.
            </p>
          </article>
          <article className="capability-card">
            <span className="card-index">03</span>
            <h3>Bounded adapters</h3>
            <p>
              Redis remains ephemeral and object bytes remain behind an opaque,
              application-owned storage boundary.
            </p>
          </article>
        </div>
      </section>

      <aside className="next-step" aria-labelledby="next-step-heading">
        <p className="eyebrow">Extension point</p>
        <h2 id="next-step-heading">Workspaces begin with identity.</h2>
        <p>
          Authentication, tenant selection, permissions, and product navigation
          are intentionally absent until their contracts are implemented.
        </p>
      </aside>
    </div>
  );
}
