import type { Metadata } from "next";

import styles from "./homepage.module.css";

export const metadata: Metadata = {
  title: { absolute: "Credar | Credit & Payment Infrastructure" },
  description:
    "Build embedded credit, BNPL, merchant financing, payment, and repayment experiences on Credar's enterprise financial infrastructure.",
};

const structuredData = {
  "@context": "https://schema.org",
  "@type": "SoftwareApplication",
  name: "Credar",
  applicationCategory: "FinanceApplication",
  operatingSystem: "Web",
  description:
    "Enterprise infrastructure for building and operating credit and payment products.",
};

export default function Home() {
  return (
    <div className={styles.homePage}>
      <script
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(structuredData) }}
      />

      <section className={styles.hero}>
        <div className={styles.heroCopy}>
          <p className={styles.kicker}>
            <span /> Credit &amp; payment infrastructure
          </p>
          <h1>
            <span>Launch credit.</span>
            <span>Move payments.</span>
            <span className={styles.heroPromise}>Fund possibility.</span>
          </h1>
          <p className={styles.heroLead}>
            Build, launch, and operate credit products people can trust, from
            embedded finance and BNPL to merchant funding, payments, and
            repayments. One enterprise platform. Every financial moment.
          </p>
          <div className={styles.heroActions}>
            <a className={styles.primaryCta} href="/auth/registration">
              Start building
              <span aria-hidden="true">↗</span>
            </a>
            <a className={styles.secondaryCta} href="#products">
              Explore Credar
              <span aria-hidden="true">↓</span>
            </a>
          </div>
          <p className={styles.heroNote}>
            Built for lenders, fintechs, marketplaces, and ambitious businesses.
          </p>
        </div>

        <div className={styles.heroVisual}>
          <div className={styles.orbitOne} aria-hidden="true" />
          <div className={styles.orbitTwo} aria-hidden="true" />
          <div className={styles.portfolioCard}>
            <div className={styles.cardHeader}>
              <div>
                <span className={styles.previewLabel}>
                  Illustrative workspace
                </span>
                <strong>Portfolio overview</strong>
              </div>
              <span className={styles.livePill}>Live</span>
            </div>
            <div className={styles.portfolioBalance}>
              <span>Credit in motion</span>
              <strong>₦84,200,000</strong>
              <small>Illustrative active portfolio</small>
            </div>
            <div className={styles.portfolioStats}>
              <div>
                <span>On schedule</span>
                <strong>96.4%</strong>
              </div>
              <div>
                <span>Collected</span>
                <strong>₦31.8m</strong>
              </div>
              <div>
                <span>Active plans</span>
                <strong>1,284</strong>
              </div>
            </div>
            <div className={styles.journey}>
              <div className={styles.journeyHead}>
                <span>Customer journey</span>
                <small>Today</small>
              </div>
              <div className={styles.journeyTrack}>
                <span className={styles.journeyDone}>Apply</span>
                <span className={styles.journeyDone}>Decide</span>
                <span className={styles.journeyActive}>Fund</span>
                <span>Repay</span>
              </div>
            </div>
          </div>
          <div className={styles.approvalCard}>
            <span className={styles.approvalIcon}>✓</span>
            <div>
              <small>Credit decision</small>
              <strong>Offer approved</strong>
            </div>
          </div>
          <div className={styles.paymentCard}>
            <span>Payment received</span>
            <strong>+ ₦240,000</strong>
            <small>Repayment · just now</small>
          </div>
        </div>
      </section>

      <section className={styles.philosophyIntro}>
        <p className={styles.sectionLabel}>Why we exist</p>
        <div>
          <h2>
            Money is not the point.
            <br />
            <em>What it unlocks is.</em>
          </h2>
          <p>
            A first storefront. Inventory before the rush. School fees paid on
            time. Growth that does not wait for perfect timing. We believe
            responsible credit is more than debt. It is momentum made
            measurable.
          </p>
        </div>
      </section>

      <section className={styles.products} id="products">
        <div className={styles.sectionHeading}>
          <div>
            <p className={styles.sectionLabel}>One financial operating layer</p>
            <h2>From the first yes to the final payment.</h2>
          </div>
          <p>
            Shape the experience around your customers while Credar coordinates
            the credit and payment journey behind it.
          </p>
        </div>

        <div className={styles.productGrid}>
          <article className={styles.creditProduct}>
            <div className={styles.productTopline}>
              <span>01</span>
              <small>Credit</small>
            </div>
            <h3>Create credit that fits real life.</h3>
            <p>
              Configure products, offers, limits, schedules, and customer
              journeys for the way your market actually moves.
            </p>
            <div
              className={styles.offerVisual}
              aria-label="Illustrative credit offer"
            >
              <div>
                <span>Flexible business credit</span>
                <strong>₦2,500,000</strong>
              </div>
              <div className={styles.offerTerms}>
                <span>6 months</span>
                <span>Monthly</span>
                <span>Auto-collect</span>
              </div>
            </div>
          </article>

          <article className={styles.paymentProduct}>
            <div className={styles.productTopline}>
              <span>02</span>
              <small>Payments</small>
            </div>
            <h3>Make repayment feel like progress.</h3>
            <p>
              Coordinate collections, repayment plans, settlements, and the
              moments that keep customers informed and in control.
            </p>
            <div
              className={styles.paymentVisual}
              aria-label="Illustrative repayment activity"
            >
              <span className={styles.paymentPulse} />
              <div>
                <small>Repayment collected</small>
                <strong>₦185,000</strong>
                <span>Balance updated automatically</span>
              </div>
            </div>
          </article>

          <article className={styles.operationsProduct}>
            <div className={styles.productTopline}>
              <span>03</span>
              <small>Operations</small>
            </div>
            <h3>Know what happened. Know why.</h3>
            <p>
              Give teams the clarity to manage customers, permissions,
              exceptions, and financial history without losing the thread.
            </p>
            <div className={styles.operationVisual}>
              <span>
                <i /> Offer accepted
                <small>09:42</small>
              </span>
              <span>
                <i /> Funds released
                <small>09:44</small>
              </span>
              <span>
                <i /> Customer notified
                <small>09:44</small>
              </span>
            </div>
          </article>
        </div>
      </section>

      <section className={styles.lifecycle}>
        <div className={styles.lifecycleCopy}>
          <p className={styles.sectionLabel}>The whole credit journey</p>
          <h2>One relationship. Not seven disconnected systems.</h2>
          <p>
            Credit is a promise stretched across time. Credar keeps every
            decision, payment, person, and change connected to that promise.
          </p>
          <a href="/auth/registration">
            Build your first product <span aria-hidden="true">→</span>
          </a>
        </div>
        <ol className={styles.lifecycleSteps}>
          <li>
            <span>01</span>
            <div>
              <strong>Onboard</strong>
              <p>Bring customers into a clear, trusted credit journey.</p>
            </div>
          </li>
          <li>
            <span>02</span>
            <div>
              <strong>Decide</strong>
              <p>Turn policy and context into consistent decisions.</p>
            </div>
          </li>
          <li>
            <span>03</span>
            <div>
              <strong>Offer</strong>
              <p>Present transparent terms customers can understand.</p>
            </div>
          </li>
          <li>
            <span>04</span>
            <div>
              <strong>Fund</strong>
              <p>Move value with the control serious finance requires.</p>
            </div>
          </li>
          <li>
            <span>05</span>
            <div>
              <strong>Collect</strong>
              <p>Make every repayment visible, timely, and reconcilable.</p>
            </div>
          </li>
        </ol>
      </section>

      <section className={styles.useCases} id="use-cases">
        <div className={styles.sectionHeading}>
          <div>
            <p className={styles.sectionLabel}>Built around your model</p>
            <h2>Your brand. Your rules. Your credit product.</h2>
          </div>
          <p>
            Start with the experience your customers need, not a template your
            business has to work around.
          </p>
        </div>
        <div className={styles.useCaseGrid}>
          <article>
            <span>For marketplaces</span>
            <h3>Embedded credit</h3>
            <p>
              Put purchasing power inside the journey customers already know.
            </p>
            <strong aria-hidden="true">↗</strong>
          </article>
          <article>
            <span>For merchants</span>
            <h3>Buy now, pay later</h3>
            <p>
              Help customers say yes today and pay on a schedule that works.
            </p>
            <strong aria-hidden="true">↗</strong>
          </article>
          <article>
            <span>For growing businesses</span>
            <h3>Working capital</h3>
            <p>Turn tomorrow&apos;s opportunity into today&apos;s capacity.</p>
            <strong aria-hidden="true">↗</strong>
          </article>
          <article>
            <span>For B2B platforms</span>
            <h3>Merchant financing</h3>
            <p>Give sellers room to stock, sell, and build with confidence.</p>
            <strong aria-hidden="true">↗</strong>
          </article>
        </div>
      </section>

      <section className={styles.enterprise} id="why-credar">
        <div className={styles.enterpriseStatement}>
          <p className={styles.sectionLabel}>Built for consequence</p>
          <h2>
            Growth needs speed.
            <br />
            Trust needs <em>proof.</em>
          </h2>
          <p>
            Credar is designed for the reality of enterprise finance: many
            businesses, many teams, precise authority, and no room for ambiguous
            history.
          </p>
        </div>
        <div className={styles.enterpriseGrid}>
          <article>
            <span>01</span>
            <div>
              <h3>Every business stays its own.</h3>
              <p>
                Explicit tenant boundaries keep customers, teams, and financial
                operations isolated by design.
              </p>
            </div>
          </article>
          <article>
            <span>02</span>
            <div>
              <h3>Access is earned, never assumed.</h3>
              <p>
                Roles and permissions make authority visible and enforce it
                where the decision actually happens.
              </p>
            </div>
          </article>
          <article>
            <span>03</span>
            <div>
              <h3>History keeps its word.</h3>
              <p>
                Material actions leave a durable trail so operators can answer
                what changed, when, and by whom.
              </p>
            </div>
          </article>
        </div>
      </section>

      <section className={styles.manifesto}>
        <div className={styles.manifestoMark} aria-hidden="true">
          C
        </div>
        <p className={styles.sectionLabel}>Our point of view</p>
        <blockquote>
          The future of finance is not more money moving faster.
          <em> It is more people moving forward.</em>
        </blockquote>
        <p>
          We are building Credar for the companies willing to make that future
          responsible, accessible, and real.
        </p>
      </section>

      <section className={styles.finalCta}>
        <div>
          <p className={styles.sectionLabel}>Build what moves people</p>
          <h2>Turn financial possibility into a product.</h2>
        </div>
        <div>
          <p>
            Start with Credar and create credit and payment experiences worthy
            of the ambitions they serve.
          </p>
          <a href="/auth/registration">
            Start building
            <span aria-hidden="true">↗</span>
          </a>
        </div>
      </section>
    </div>
  );
}
