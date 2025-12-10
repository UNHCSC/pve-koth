# Creating Your First Competition

If you want to know what a ready-to-upload competition looks like, start with `examples/competition_config/`. That folder already mirrors the archive structure the dashboard expects:

```
competition.zip
├── config.json           # central competition metadata and container definitions
├── scripts/              # setup/scoring helpers run inside each container
├── public/               # optional static artifacts served via /static/<competition>
├── writeup.md            # deliverables you share with teams
└── writeup.pdf           # (or any other artifact you prefer to stash)
```

The example archive ships with two container configs (`website` and `grafana`) and demonstrates how to combine shared/global scripts (`scripts/setup_global.sh`/`scripts/score_global.sh`) with container-specific ones. Look at `examples/competition_config/README.md` for a narrative you can copy when teaching teams about the competition.

### Populating `config.json`

Use the sample `config.json` as a template. Important keys:

- `competitionID`, `competitionName`, `competitionDescription`, and `competitionHost` describe the competition itself.
- `numTeams` controls how many team slots are created.
- `privacy.public` toggles visibility; `ldapAllowedGroupsFilter` can limit access to specific groups.
- `containerSpecs` defines defaults for every container (template, storage pool, resources, gateway, DNS, etc.).
- `teamContainerConfigs` contains an array of container definitions with:
  - `name` (human label used in the dashboard),
  - `lastOctetValue` (the octet offset used when allocating IPs in the competition block),
  - `setupScript`/`scoringScript` arrays that reference files inside `scripts/`,
  - `scoringSchema`, the checks the scoring loops execute.
- `setupPublicFolder` points to a subdirectory (like `public`) that will be served to containers when they download static assets.
- `writeupFilePath` can reference a Markdown or PDF file to share with participants after provisioning.

When you're ready to upload, zip the folder so that `config.json` is at the archive root and upload via the dashboard's create competition modal.

### Available Environment Variables

Scripts executed inside each container receive the following environment variables:

- `KOTH_COMP_ID` — the competition system ID (e.g., `exampleComp`).
- `KOTH_TEAM_ID` — the numeric team ID in the database.
- `KOTH_HOSTNAME` — the container hostname assigned by the provisioning logic.
- `KOTH_IP` — the actual IPv4 assigned to the container.
- `KOTH_PUBLIC_FOLDER` — the HTTP base URL where `setupPublicFolder` contents are served; combine with `KOTH_ACCESS_TOKEN` for authenticated fetches.
- `KOTH_ACCESS_TOKEN` — a time-limited bearer token (30 minutes) that scripts include when downloading artifacts from the admin server.
- `KOTH_CONTAINER_IPS` — a comma-separated list of every IP in this team's subnet block.
- `KOTH_CONTAINER_IPS_<name>` — single env vars for each container, derived from the container configuration names (e.g., `KOTH_CONTAINER_IPS_website`).

Use these env vars in your `scripts/` helpers to discover peer IPs, verify services, download scoring scripts, or fetch public assets. The `examples/competition_config/scripts` directory already shows how to leverage `KOTH_PUBLIC_FOLDER`, `KOTH_ACCESS_TOKEN`, `KOTH_IP`, and the `KOTH_CONTAINER_IPS` list for both setup and scoring.
