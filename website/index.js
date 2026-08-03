// Small progressive enhancement for the proctop page: nothing here is
// required for the content to render.

document.addEventListener("DOMContentLoaded", () => {
  const year = document.getElementById("year")
  if (year) {
    year.textContent = String(new Date().getFullYear())
  }

  // Highlight the nav link of the section currently in view.
  const links = [...document.querySelectorAll(".nav-links a[href^='#']")]
  const targets = links
    .map((a) => document.querySelector(a.getAttribute("href")))
    .filter(Boolean)

  if (targets.length === 0 || !("IntersectionObserver" in window)) {
    return
  }

  const seen = new IntersectionObserver(
    (entries) => {
      for (const entry of entries) {
        if (!entry.isIntersecting) {
          continue
        }
        for (const a of links) {
          const active = a.getAttribute("href") === "#" + entry.target.id
          a.style.color = active ? "var(--ink)" : ""
        }
      }
    },
    { rootMargin: "-45% 0px -50% 0px" },
  )

  for (const t of targets) {
    seen.observe(t)
  }
})
