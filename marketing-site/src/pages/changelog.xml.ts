import type { APIRoute } from 'astro';
import { CHANGELOG, SITE } from '../lib/site';

export const GET: APIRoute = ({ site }) => {
  const baseUrl = (site ?? new URL(`https://${SITE.domain}/`)).toString().replace(/\/$/, '');
  const items = CHANGELOG.map((entry) => {
    const dateIso = new Date(`${entry.date}T00:00:00Z`).toUTCString();
    const guid = `${baseUrl}/changelog#${entry.date}-${slug(entry.title)}`;
    return [
      '<item>',
      `  <title>${esc(entry.title)}</title>`,
      `  <link>${guid}</link>`,
      `  <guid isPermaLink="false">${guid}</guid>`,
      `  <pubDate>${dateIso}</pubDate>`,
      `  <description>${esc(entry.body)}</description>`,
      '</item>',
    ].join('\n');
  }).join('\n');

  const xml = [
    '<?xml version="1.0" encoding="UTF-8"?>',
    '<rss version="2.0">',
    '  <channel>',
    `    <title>${SITE.name} — changelog</title>`,
    `    <link>${baseUrl}/changelog</link>`,
    `    <description>Recent updates to ${SITE.name}.</description>`,
    `    <language>en-us</language>`,
    items,
    '  </channel>',
    '</rss>',
  ].join('\n');

  return new Response(xml, {
    headers: { 'content-type': 'application/rss+xml; charset=utf-8' },
  });
};

function esc(s: string): string {
  return s
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&apos;');
}

function slug(s: string): string {
  return s
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-|-$/g, '');
}
