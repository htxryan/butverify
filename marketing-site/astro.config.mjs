import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import svelte from '@astrojs/svelte';
import tailwind from '@astrojs/tailwind';
import sitemap from '@astrojs/sitemap';
import mdx from '@astrojs/mdx';

const SITE_URL = process.env.SITE_URL ?? 'https://butverify.dev';

export default defineConfig({
  site: SITE_URL,
  trailingSlash: 'ignore',
  output: 'static',
  integrations: [
    svelte(),
    starlight({
      title: 'butverify docs',
      description:
        'Quickstarts, agent integration recipes, and reference for the butverify CLI and API.',
      // Mirror the BaseLayout OG/Twitter card setup so docs pages share the
      // same hero image when linked in chat or social. Starlight emits its
      // own <head>, so meta authored here is what crawlers see for /docs/*.
      head: [
        { tag: 'meta', attrs: { property: 'og:image', content: `${SITE_URL}/og/default.png` } },
        {
          tag: 'meta',
          attrs: { property: 'og:image:secure_url', content: `${SITE_URL}/og/default.png` },
        },
        { tag: 'meta', attrs: { property: 'og:image:type', content: 'image/png' } },
        { tag: 'meta', attrs: { property: 'og:image:width', content: '1200' } },
        { tag: 'meta', attrs: { property: 'og:image:height', content: '630' } },
        { tag: 'meta', attrs: { property: 'og:image:alt', content: 'butverify docs' } },
        { tag: 'meta', attrs: { name: 'twitter:card', content: 'summary_large_image' } },
        { tag: 'meta', attrs: { name: 'twitter:image', content: `${SITE_URL}/og/default.png` } },
        { tag: 'meta', attrs: { name: 'twitter:image:alt', content: 'butverify docs' } },
      ],
      sidebar: [
        {
          label: 'Getting started',
          items: [
            { label: 'Install the CLI', slug: 'docs/quickstart/install' },
            { label: 'Push your first site', slug: 'docs/quickstart/first-push' },
            { label: 'View it in a browser', slug: 'docs/quickstart/view' },
            { label: 'JSON output for agents', slug: 'docs/quickstart/json-output' },
          ],
        },
        {
          label: 'Agent integration recipes',
          items: [
            { label: 'Claude Code', slug: 'docs/agents/claude-code' },
            { label: 'Cursor', slug: 'docs/agents/cursor' },
            { label: 'Codex', slug: 'docs/agents/codex' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'CLI commands', slug: 'docs/reference/cli' },
            { label: 'bv evidence', slug: 'docs/reference/evidence' },
            { label: 'bv install-skill claude', slug: 'docs/reference/install-skill' },
            { label: 'Error codes', slug: 'docs/reference/error-codes' },
            { label: 'Site lifecycle', slug: 'docs/reference/site-lifecycle' },
          ],
        },
        {
          label: 'Troubleshooting',
          slug: 'docs/troubleshooting',
        },
      ],
      customCss: ['./src/styles/starlight.css'],
      components: {
        Header: './src/components/starlight/Header.astro',
        ThemeProvider: './src/components/starlight/ThemeProvider.astro',
        ThemeSelect: './src/components/starlight/ThemeSelect.astro',
      },
      disable404Route: true,
    }),
    tailwind({ applyBaseStyles: false }),
    mdx(),
    sitemap(),
  ],
  vite: {
    define: {
      'import.meta.env.PUBLIC_POSTHOG_KEY': JSON.stringify(process.env.PUBLIC_POSTHOG_KEY ?? ''),
      'import.meta.env.PUBLIC_POSTHOG_HOST': JSON.stringify(
        process.env.PUBLIC_POSTHOG_HOST ?? 'https://us.i.posthog.com',
      ),
      'import.meta.env.PUBLIC_GH_APP_INSTALL_URL': JSON.stringify(
        process.env.PUBLIC_GH_APP_INSTALL_URL ??
          'https://github.com/apps/butverify/installations/new',
      ),
    },
  },
});
