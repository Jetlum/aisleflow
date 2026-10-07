# Pyck functionality animation

Run `node docs/animation/serve.cjs` from the project root and open **http://localhost:4173**. Alternatively, open `index.html` directly in a modern browser. No build, installation, network connection, or running backend is required. The 54-second walkthrough starts automatically, except with reduced motion enabled. Use pause, the five chapter buttons, timeline, replay, and speed selector to present it at your own pace. Playback pauses when the tab is hidden. Chapter buttons also provide a static step-through.

The story shows the implemented AisleFlow feedback loop: BPMN compilation, Temporal movement activities, movement telemetry, congestion detection, transactional outbox delivery, an approved A01-to-A02 alternative, and the express packing branch. It is an independent concept presentation, not an official Pyck integration or a connected operational dashboard.

The route and synthetic movement samples follow `../../TECHNICAL_GUIDE.md` and `../../tests/closedloop/closedloop_test.go`: four A01 tasks, four A02 tasks, and express packing. The fourth A01 sample gives `(10 + 11 + 30) / 3 = 17 s`, above the configured 15-second threshold. Illustration coordinates and presentation timing are invented for clarity. The live stack can apply the signal at a later task boundary.

- `index.html`: accessible page structure and chapter controls.
- `style.css`: responsive presentation design.
- `story.js`: deterministic, seekable demonstration state.
- `animation.js`: warehouse drawing and playback.

Check the story and playback controls with `node --test --test-isolation=none docs/animation/*.test.cjs` from the project root. The animation does not change or execute the backend.

Visual reference: [Pyck homepage](https://pyck.ai/), inspected 24 September 2026. Its charcoal (`#1B1B21`), aqua (`#19FFF6`), cerulean, and violet palette, Montserrat typography, and isometric warehouse informed the design. The public logo and font are saved under `assets/` from the homepage's `assets/logos/squirrel-white.svg` and `assets/fonts/montserrat-latin.woff2`. The animated warehouse is drawn specifically for this demo. Pyck branding belongs to its respective owner.
