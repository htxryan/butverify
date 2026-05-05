// @ts-check
(function () {
  var stacked = document.getElementById("ev-layout-stacked");
  var carousel = document.getElementById("ev-layout-carousel");
  var viewport = document.querySelector("[data-ev-gallery-viewport]");
  var track = document.querySelector(".ev-track");
  var slides = Array.prototype.slice.call(document.querySelectorAll("[data-ev-slide]"));
  var pageButtons = Array.prototype.slice.call(document.querySelectorAll("[data-ev-page]"));
  var outlineLinks = Array.prototype.slice.call(document.querySelectorAll("[data-ev-outline-link]"));
  var prev = document.querySelector("[data-ev-prev]");
  var next = document.querySelector("[data-ev-next]");
  var outline = document.querySelector("[data-ev-outline]");
  var outlineToggle = document.querySelector("[data-ev-outline-toggle]");
  var metaPanel = document.getElementById("ev-meta-panel");
  var metaToggle = document.querySelector("[data-ev-meta-toggle]");
  var metaClose = document.querySelector("[data-ev-meta-close]");
  var themeToggle = document.querySelector("[data-ev-theme-toggle]");
  var themeLabel = document.querySelector("[data-ev-theme-label]");
  var publishedTimes = Array.prototype.slice.call(document.querySelectorAll("[data-ev-published-at]"));
  var lightbox = document.querySelector("[data-ev-lightbox]");
  var lightboxImg = document.querySelector("[data-ev-lightbox-img]");
  var lightboxTitle = document.querySelector("[data-ev-lightbox-title]");
  var lightboxTriggers = Array.prototype.slice.call(document.querySelectorAll("[data-ev-lightbox-trigger]"));
  var lightboxClose = document.querySelector("[data-ev-lightbox-close]");
  var lightboxZoomIn = document.querySelector("[data-ev-lightbox-zoom-in]");
  var lightboxZoomOut = document.querySelector("[data-ev-lightbox-zoom-out]");
  var lightboxReset = document.querySelector("[data-ev-lightbox-reset]");
  var lightboxFullscreen = document.querySelector("[data-ev-lightbox-fullscreen]");
  var currentIndex = 0;
  var scrollTimer = 0;
  var lightboxScale = 1;
  var lastLightboxTrigger = null;
  var themeStorageKey = "butverify:theme";
  var systemThemeQuery = window.matchMedia ? window.matchMedia("(prefers-color-scheme: dark)") : null;
  var mobileQuery = window.matchMedia ? window.matchMedia("(max-width: 48rem)") : null;

  function isCarousel() {
    return Boolean(carousel && carousel.checked);
  }

  function clampIndex(index) {
    return Math.max(0, Math.min(slides.length - 1, index));
  }

  function setActive(index) {
    currentIndex = clampIndex(index);
    pageButtons.forEach(function (button, i) {
      if (i === currentIndex) {
        button.setAttribute("aria-current", "true");
      } else {
        button.removeAttribute("aria-current");
      }
    });
    outlineLinks.forEach(function (link, i) {
      if (i === currentIndex) {
        link.setAttribute("aria-current", "true");
      } else {
        link.removeAttribute("aria-current");
      }
    });
    if (prev) {
      prev.toggleAttribute("disabled", currentIndex <= 0 || slides.length < 2);
    }
    if (next) {
      next.toggleAttribute("disabled", currentIndex >= slides.length - 1 || slides.length < 2);
    }
  }

  function goTo(index) {
    var target = slides[clampIndex(index)];
    if (!target) {
      return;
    }
    window.clearTimeout(scrollTimer);
    setActive(Number(target.getAttribute("data-ev-index")) || 0);
    target.scrollIntoView({
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth",
      block: "nearest",
      inline: "start"
    });
  }

  function stepBy(delta) {
    var base = nearestIndex();
    setActive(base);
    goTo(base + delta);
  }

  function nearestIndex() {
    var scroller = isCarousel() && track ? track : viewport;
    if (!scroller || slides.length === 0) {
      return 0;
    }
    var viewRect = scroller.getBoundingClientRect();
    var best = 0;
    var bestDistance = Infinity;
    slides.forEach(function (slide, i) {
      var rect = slide.getBoundingClientRect();
      var distance = isCarousel() ? Math.abs(rect.left - viewRect.left) : Math.abs(rect.top - viewRect.top);
      if (distance < bestDistance) {
        bestDistance = distance;
        best = i;
      }
    });
    return best;
  }

  function queueActiveUpdate() {
    window.clearTimeout(scrollTimer);
    scrollTimer = window.setTimeout(function () {
      setActive(nearestIndex());
    }, 80);
  }

  function toggleMetadata(open) {
    if (!metaPanel || !metaToggle) {
      return;
    }
    metaPanel.hidden = !open;
    metaToggle.setAttribute("aria-expanded", String(open));
    if (open) {
      metaPanel.focus();
    } else {
      metaToggle.focus();
    }
  }

  function setOutlineCollapsed(collapsed) {
    if (!outline || !outlineToggle) {
      return;
    }
    outline.setAttribute("data-collapsed", String(collapsed));
    outlineToggle.setAttribute("aria-expanded", String(!collapsed));
  }

  function syncMobileOutline() {
    if (mobileQuery && mobileQuery.matches) {
      setOutlineCollapsed(true);
    }
  }

  function storedTheme() {
    var theme;
    try {
      theme = localStorage.getItem(themeStorageKey);
      return theme === "light" || theme === "dark" ? theme : "";
    } catch (e) {
      return "";
    }
  }

  function systemTheme() {
    return systemThemeQuery && systemThemeQuery.matches ? "dark" : "light";
  }

  function currentTheme() {
    return storedTheme() || systemTheme();
  }

  function syncThemeToggle(theme) {
    if (!themeToggle) {
      return;
    }
    var dark = theme === "dark";
    themeToggle.setAttribute("aria-pressed", String(dark));
    themeToggle.setAttribute("aria-label", dark ? "Switch to light mode" : "Switch to dark mode");
    if (themeLabel) {
      themeLabel.textContent = dark ? "Dark" : "Light";
    }
  }

  function setTheme(theme) {
    if (theme !== "light" && theme !== "dark") {
      return;
    }
    document.documentElement.setAttribute("data-ev-theme", theme);
    try {
      localStorage.setItem(themeStorageKey, theme);
    } catch (e) {}
    syncThemeToggle(theme);
  }

  if (themeToggle) {
    syncThemeToggle(currentTheme());
    themeToggle.addEventListener("click", function () {
      setTheme(currentTheme() === "dark" ? "light" : "dark");
    });
  }
  if (systemThemeQuery && systemThemeQuery.addEventListener) {
    systemThemeQuery.addEventListener("change", function () {
      if (!storedTheme()) {
        syncThemeToggle(systemTheme());
      }
    });
  }

  function syncPublishedTimes() {
    publishedTimes.forEach(function (node) {
      var raw = node.getAttribute("datetime") || node.textContent || "";
      var date = new Date(raw);
      if (Number.isNaN(date.getTime())) {
        return;
      }
      node.textContent = date.toLocaleString();
    });
  }

  function isLightboxOpen() {
    return Boolean(lightbox && !lightbox.hidden);
  }

  function setLightboxZoom(scale) {
    if (!lightboxImg) {
      return;
    }
    lightboxScale = Math.max(1, Math.min(4, scale));
    lightboxImg.style.transform = "scale(" + lightboxScale + ")";
    lightboxImg.setAttribute("data-zoomed", String(lightboxScale > 1));
    if (lightboxZoomOut) {
      lightboxZoomOut.toggleAttribute("disabled", lightboxScale <= 1);
    }
    if (lightboxReset) {
      lightboxReset.toggleAttribute("disabled", lightboxScale <= 1);
    }
    if (lightboxZoomIn) {
      lightboxZoomIn.toggleAttribute("disabled", lightboxScale >= 4);
    }
  }

  function openLightbox(trigger) {
    if (!lightbox || !lightboxImg) {
      return;
    }
    lastLightboxTrigger = trigger;
    lightboxImg.setAttribute("src", trigger.getAttribute("data-ev-lightbox-src") || "");
    lightboxImg.setAttribute("alt", trigger.getAttribute("data-ev-lightbox-alt") || "");
    if (lightboxTitle) {
      lightboxTitle.textContent = trigger.getAttribute("data-ev-lightbox-title") || "Image preview";
    }
    lightbox.hidden = false;
    setLightboxZoom(1);
    if (lightboxClose) {
      lightboxClose.focus();
    } else {
      lightbox.focus();
    }
  }

  function closeLightbox() {
    if (!lightbox || !lightboxImg) {
      return;
    }
    if (document.fullscreenElement === lightbox && document.exitFullscreen) {
      document.exitFullscreen().catch(function () {});
    }
    lightbox.hidden = true;
    lightboxImg.removeAttribute("src");
    setLightboxZoom(1);
    if (lastLightboxTrigger) {
      lastLightboxTrigger.focus();
    }
  }

  function toggleLightboxFullscreen() {
    if (!lightbox || !document.fullscreenEnabled) {
      return;
    }
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(function () {});
      return;
    }
    lightbox.requestFullscreen().catch(function () {});
  }

  lightboxTriggers.forEach(function (trigger) {
    trigger.addEventListener("click", function () {
      openLightbox(trigger);
    });
  });

  if (lightbox) {
    lightbox.setAttribute("tabindex", "-1");
    lightbox.addEventListener("click", function (event) {
      if (event.target === lightbox) {
        closeLightbox();
      }
    });
  }
  if (lightboxClose) {
    lightboxClose.addEventListener("click", closeLightbox);
  }
  if (lightboxZoomIn) {
    lightboxZoomIn.addEventListener("click", function () {
      setLightboxZoom(lightboxScale + 0.25);
    });
  }
  if (lightboxZoomOut) {
    lightboxZoomOut.addEventListener("click", function () {
      setLightboxZoom(lightboxScale - 0.25);
    });
  }
  if (lightboxReset) {
    lightboxReset.addEventListener("click", function () {
      setLightboxZoom(1);
    });
  }
  if (lightboxFullscreen) {
    if (!document.fullscreenEnabled || !lightbox || !lightbox.requestFullscreen) {
      lightboxFullscreen.setAttribute("disabled", "");
    }
    lightboxFullscreen.addEventListener("click", toggleLightboxFullscreen);
  }

  if (metaPanel) {
    metaPanel.setAttribute("tabindex", "-1");
  }
  if (metaToggle) {
    metaToggle.addEventListener("click", function () {
      toggleMetadata(Boolean(metaPanel && metaPanel.hidden));
    });
  }
  if (metaClose) {
    metaClose.addEventListener("click", function () {
      toggleMetadata(false);
    });
  }

  if (outlineToggle && outline) {
    syncMobileOutline();
    outlineToggle.addEventListener("click", function () {
      var collapsed = outline.getAttribute("data-collapsed") === "true";
      setOutlineCollapsed(!collapsed);
    });
  }
  if (mobileQuery && mobileQuery.addEventListener) {
    mobileQuery.addEventListener("change", syncMobileOutline);
  } else if (mobileQuery && mobileQuery.addListener) {
    mobileQuery.addListener(syncMobileOutline);
  }

  outlineLinks.forEach(function (link) {
    link.addEventListener("click", function () {
      var index = Number(link.getAttribute("data-ev-index")) || 0;
      setActive(index);
      if (mobileQuery && mobileQuery.matches) {
        setOutlineCollapsed(true);
      }
    });
  });

  pageButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      goTo(Number(button.getAttribute("data-ev-index")) || 0);
    });
  });

  if (prev) {
    prev.addEventListener("click", function () {
      stepBy(-1);
    });
  }
  if (next) {
    next.addEventListener("click", function () {
      stepBy(1);
    });
  }

  [stacked, carousel].forEach(function (input) {
    if (!input) {
      return;
    }
    input.addEventListener("change", function () {
      window.clearTimeout(scrollTimer);
      window.requestAnimationFrame(function () {
        goTo(currentIndex);
        if (viewport) {
          viewport.focus();
        }
      });
    });
  });

  if (viewport) {
    viewport.addEventListener("scroll", queueActiveUpdate, { passive: true });
  }
  if (track) {
    track.addEventListener("scroll", queueActiveUpdate, { passive: true });
  }

  document.addEventListener("keydown", function (event) {
    var target = event.target;
    var tag = target && target.tagName ? target.tagName.toLowerCase() : "";
    if (tag === "input" || tag === "textarea" || tag === "select" || (target && target.isContentEditable)) {
      return;
    }
    if (isLightboxOpen()) {
      if (event.key === "Escape") {
        event.preventDefault();
        closeLightbox();
      } else if (event.key === "+" || event.key === "=") {
        event.preventDefault();
        setLightboxZoom(lightboxScale + 0.25);
      } else if (event.key === "-" || event.key === "_") {
        event.preventDefault();
        setLightboxZoom(lightboxScale - 0.25);
      } else if (event.key === "0") {
        event.preventDefault();
        setLightboxZoom(1);
      }
      return;
    }
    if (event.key === "Escape" && metaPanel && !metaPanel.hidden) {
      event.preventDefault();
      toggleMetadata(false);
      return;
    }
    if (!isCarousel()) {
      return;
    }
    if (event.key === "ArrowLeft") {
      event.preventDefault();
      stepBy(-1);
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      stepBy(1);
    }
  });

  syncPublishedTimes();
  setActive(0);
})();
