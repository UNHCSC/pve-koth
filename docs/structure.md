# Project Structure

The repository is intentionally organized so the Go service, frontend build, and documentation live side-by-side:

- `app/` contains the Fiber HTTP server, authentication helpers, and SSE job wiring for redeploys/teardowns.
- `koth/` implements competition lifecycle behaviors (provisioning, scoring, redeploy, teardown) and exposes the environment variables your container scripts will see.
- `public/src/` houses the dashboard JavaScript/CSS layers and modal implementations.
- `public/views/` renders the dashboard/landing templates that consume the built assets under `public/static/`.
- `tests/` includes runnable Go suites; the new job-stream tests live alongside the existing DB helpers.
- `docs/` is where you will find narrative guides (architecture, teardown, testing, competition creation, etc.).
- `examples/competition_config/` is the reference competition bundle you can zip up and upload through the dashboard.
