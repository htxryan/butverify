/**
 * Centralized copy/links so quickstarts, agent recipes, and the landing page
 * stay synchronized. Tests assert on the values exported here.
 */

export const SITE = {
  name: 'butverify',
  tagline: 'Static site hosting your AI agents can publish to.',
  description:
    "An AI agent shouldn't need DevOps to show its work. butverify previews locally by default, publishes private links on demand, and disappears.",
  domain: 'butverify.dev',
  appHost: 'app.butverify.dev',
  apiHost: 'api.butverify.dev',
  docsBase: '/docs',
  cliBinary: 'bv',
  cliInstallTap: 'butverify/tap',
  installCmd: 'brew install butverify/tap/bv',
  ghAppInstallEnv: 'PUBLIC_GH_APP_INSTALL_URL',
} as const;

export const NAV = [
  { label: 'Pricing', href: '/pricing' },
  { label: 'Agents', href: '/agents' },
  { label: 'Docs', href: '/docs/quickstart/install' },
  { label: 'Changelog', href: '/changelog' },
] as const;

export type Plan = {
  id: 'free' | 'pro' | 'team';
  name: string;
  price: string;
  cadence: string;
  highlight?: boolean;
  blurb: string;
  features: string[];
  ctaLabel: string;
  ctaHref: string;
};

export const PLANS: ReadonlyArray<Plan> = [
  {
    id: 'free',
    name: 'Free',
    price: '$0',
    cadence: '/forever',
    blurb: 'For solo devs trying things out.',
    features: [
      '5 sites',
      '100 MB total storage',
      '30-day site lifetime',
      'Public sites',
      'JSON CLI output',
    ],
    ctaLabel: 'Install the GitHub App',
    ctaHref: '{{INSTALL_URL}}',
  },
  {
    id: 'pro',
    name: 'Pro',
    price: '$10',
    cadence: '/month',
    highlight: true,
    blurb: 'For developers shipping agent work daily.',
    features: [
      '100 sites',
      '5 GB storage',
      'Unlimited site lifetime (until you delete)',
      'Per-site GitHub access policies',
      'Priority email support',
    ],
    ctaLabel: 'Start Pro trial',
    ctaHref: '{{INSTALL_URL}}',
  },
  {
    id: 'team',
    name: 'Team',
    price: '$30',
    cadence: '/user / month',
    blurb: 'For agencies and orgs running fleets of agents.',
    features: [
      'Org tenants (GitHub Organizations)',
      'Shared site quota across the org',
      'Audit log export to R2',
      'SSO via GitHub Enterprise (preview)',
    ],
    ctaLabel: 'Talk to us',
    ctaHref: 'mailto:hello@butverify.dev?subject=Team plan',
  },
];

export type AgentSlug = 'claude-code' | 'cursor' | 'codex' | 'copilot' | 'aider' | 'continue';

export type AgentRecipe = {
  slug: 'claude-code' | 'cursor' | 'codex';
  name: string;
  blurb: string;
  docsHref: string;
};

export const AGENTS: ReadonlyArray<AgentRecipe> = [
  {
    slug: 'claude-code',
    name: 'Claude Code',
    blurb: "Add a hook that publishes the project's preview directory remotely after a successful build.",
    docsHref: '/docs/agents/claude-code',
  },
  {
    slug: 'cursor',
    name: 'Cursor',
    blurb: 'Wire `bv push --mode remote` into a Cursor command and surface the URL in the agent transcript.',
    docsHref: '/docs/agents/cursor',
  },
  {
    slug: 'codex',
    name: 'OpenAI Codex CLI',
    blurb:
      'Drop a `~/.codex/post-task.sh` that publishes the workspace remotely and prints the URL as JSON.',
    docsHref: '/docs/agents/codex',
  },
];

/**
 * Agents/IDEs the trust strip lists as compatible with `bv push`.
 *
 * The bv CLI is a plain shell binary that emits JSON, so any agent that can
 * shell out works. The three with `recipeHref` ship dedicated docs; the rest
 * are listed because they can drive the CLI through their generic
 * shell/terminal/run-command surfaces — no fake testimonials, no logos for
 * products that can't actually run a command.
 */
export type CompatibleAgent = {
  slug: AgentSlug;
  name: string;
  /** Set when this agent has a dedicated recipe under /docs/agents. */
  recipeHref?: string;
};

export const COMPATIBLE_AGENTS: ReadonlyArray<CompatibleAgent> = [
  { slug: 'claude-code', name: 'Claude Code', recipeHref: '/docs/agents/claude-code' },
  { slug: 'cursor', name: 'Cursor', recipeHref: '/docs/agents/cursor' },
  { slug: 'codex', name: 'Codex CLI', recipeHref: '/docs/agents/codex' },
  { slug: 'copilot', name: 'GitHub Copilot' },
  { slug: 'aider', name: 'Aider' },
  { slug: 'continue', name: 'Continue' },
];

export type TrustClaim = {
  /** Short label rendered in the security row. */
  label: string;
  /** One-sentence supporting fact for the visible tooltip-ish line. */
  detail: string;
  /** Anchor on /security so the link lands the user near the relevant copy. */
  href: string;
};

export const TRUST_CLAIMS: ReadonlyArray<TrustClaim> = [
  {
    label: 'GitHub OAuth',
    detail: 'Sites authenticate against your GitHub identity — no separate password.',
    href: '/security#github-oauth',
  },
  {
    label: 'Cloudflare Access',
    detail: 'Private sites are gated at the edge before any request reaches the Worker.',
    href: '/security#cloudflare-access',
  },
  {
    label: 'cosign-signed releases (planned for v1.0)',
    detail:
      'The bv release pipeline is configured to publish cosign signatures and a Homebrew tap that verifies them; not yet wired into CI.',
    href: '/security#cosign-signed-releases',
  },
];

export type ChangelogEntry = {
  date: string;
  title: string;
  body: string;
};

export const CHANGELOG: ReadonlyArray<ChangelogEntry> = [
  {
    date: '2026-04-27',
    title: 'Marketing + docs surface live',
    body: 'Public site at butverify.dev with quickstarts, agent integration recipes, and reference docs at /docs.',
  },
  {
    date: '2026-04-27',
    title: 'Dashboard preview at app.butverify.dev',
    body: 'SvelteKit dashboard for site list, billing entry, and access policies (paid tier).',
  },
  {
    date: '2026-04-27',
    title: 'CLI alpha — local previews and remote publishing',
    body: 'Go binary with local-first `bv push`, remote publishing mode, JSON output, and Homebrew tap support.',
  },
];

export function resolveInstallUrl(href: string): string {
  if (!href.includes('{{INSTALL_URL}}')) return href;
  const env = (
    typeof import.meta !== 'undefined' && import.meta.env
      ? import.meta.env.PUBLIC_GH_APP_INSTALL_URL
      : undefined
  ) as string | undefined;
  const fallback = 'https://github.com/apps/butverify/installations/new';
  // Defense-in-depth: a misconfigured CI variable mustn't redirect users to
  // an attacker-controlled domain. Only honor PUBLIC_GH_APP_INSTALL_URL when
  // it points at github.com; otherwise fall back to the canonical install URL.
  const candidate = env && env.length > 0 ? env : fallback;
  let chosen = fallback;
  try {
    const u = new URL(candidate);
    if (u.protocol === 'https:' && (u.host === 'github.com' || u.host.endsWith('.github.com'))) {
      chosen = candidate;
    }
  } catch {
    chosen = fallback;
  }
  return href.replaceAll('{{INSTALL_URL}}', chosen);
}
