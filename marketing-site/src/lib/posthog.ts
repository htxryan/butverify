/**
 * PostHog browser snippet builder.
 *
 * Server-side: Astro injects the snippet into the document `<head>` only when
 * `PUBLIC_POSTHOG_KEY` is set, so previews and local dev don't ship analytics.
 *
 * Returned string is the official PostHog snippet adapted to fail closed if
 * the SDK fails to load (no exceptions thrown into page JS).
 */
export function posthogSnippet(opts: { key: string; host: string }): string {
  // Escape `</` so a malicious or fat-fingered key can never close the inline
  // <script> we render this string into. JSON.stringify alone doesn't.
  const safe = (s: string) => JSON.stringify(s).replace(/<\//g, '<\\/');
  const k = safe(opts.key);
  const h = safe(opts.host);
  return `!(function (t, e) {
    var o, n, p, r;
    e.__SV ||
      ((window.posthog = e),
      (e._i = []),
      (e.init = function (i, s, a) {
        function g(t, e) {
          var o = e.split(".");
          2 == o.length && ((t = t[o[0]]), (e = o[1])),
            (t[e] = function () {
              t.push([e].concat(Array.prototype.slice.call(arguments, 0)));
            });
        }
        ((p = t.createElement("script")).type = "text/javascript"),
          (p.async = !0),
          (p.src = s.api_host + "/static/array.js"),
          (r = t.getElementsByTagName("script")[0]).parentNode.insertBefore(p, r);
        var u = e;
        for (
          void 0 !== a ? (u = e[a] = []) : (a = "posthog"),
            u.people = u.people || [],
            u.toString = function (t) {
              var e = "posthog";
              return "posthog" !== a && (e += "." + a), t || (e += " (stub)"), e;
            },
            u.people.toString = function () {
              return u.toString(1) + ".people (stub)";
            },
            o =
              "init me ws ys ps bs capture je Di ks register register_once register_for_session unregister unregister_for_session Ps getFeatureFlag getFeatureFlagPayload isFeatureEnabled reloadFeatureFlags updateEarlyAccessFeatureEnrollment getEarlyAccessFeatures on onFeatureFlags onSessionId getSurveys getActiveMatchingSurveys renderSurvey canRenderSurvey identify setPersonProperties group resetGroups setPersonPropertiesForFlags resetPersonPropertiesForFlags setGroupPropertiesForFlags resetGroupPropertiesForFlags reset get_distinct_id getGroups get_session_id get_session_replay_url alias set_config startSessionRecording stopSessionRecording sessionRecordingStarted captureException loadToolbar get_property getSessionProperty Es zs createPersonProfile Is opt_in_capturing opt_out_capturing has_opted_in_capturing has_opted_out_capturing clear_opt_in_out_capturing Ss debug xs getPageViewId captureTraceFeedback captureTraceMetric".split(
                " ",
              ),
            n = 0;
          n < o.length;
          n++
        )
          g(u, o[n]);
        e._i.push([i, s, a]);
      }),
      (e.__SV = 1));
  })(document, window.posthog || []);
  posthog.init(${k}, { api_host: ${h}, capture_pageview: true });`;
}

export type CtaEvent = {
  name: 'cta_click';
  cta: string;
  location: string;
};

/**
 * Helper used by client islands to fire CTA events without crashing if the
 * PostHog SDK is missing (e.g. local dev with no key).
 */
export function captureCta(event: CtaEvent): void {
  if (typeof window === 'undefined') return;
  const ph = (window as { posthog?: { capture?: (n: string, p: unknown) => void } }).posthog;
  ph?.capture?.(event.name, { cta: event.cta, location: event.location });
}
