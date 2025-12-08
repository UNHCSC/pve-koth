import { escapeHTML } from "./shared/utils.js";

const root = document.getElementById("scoreboard-root");
const tabs = document.getElementById("scoreboard-tabs");
const table = document.getElementById("scoreboard-table");
const empty = document.getElementById("scoreboard-empty");
const gridSection = document.getElementById("scoreboard-grid");
const gridBody = gridSection?.querySelector("[data-grid]");
const canManage = root?.dataset.canManage === "true";
const REFRESH_INTERVAL = 30_000;
const dateFormatter = new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
});

const state = {
    competitions: [],
    selected: root?.dataset.selected || "",
};

function computeTeamCheckSummary(team) {
    const containers = Array.isArray(team?.containers) ? team.containers : [];
    let total = 0;
    let passed = 0;
    containers.forEach((container) => {
        (container.checks || []).forEach((check) => {
            total += 1;
            if (check?.passed) {
                passed += 1;
            }
        });
    });

    const ratio = total > 0 ? passed / total : 0;
    let status = "bg-slate-600/40 text-slate-200";
    if (total === 0) {
        status = "bg-slate-600/40 text-slate-200";
    } else if (ratio >= 0.85) {
        status = "bg-emerald-500/30 text-emerald-100";
    } else if (ratio >= 0.6) {
        status = "bg-yellow-500/30 text-yellow-900";
    } else if (ratio >= 0.3) {
        status = "bg-orange-500/30 text-orange-100";
    } else {
        status = "bg-rose-600/40 text-rose-100";
    }

    return { passed, total, ratio, status };
}

function renderRankingRow(team, index) {
    const rowClass =
        index === 0 ? "bg-white/10" : index % 2 === 0 ? "bg-white/[0.04]" : "bg-transparent";
    const summary = computeTeamCheckSummary(team);
    return `<tr class="${rowClass}">
        <td class="px-2 py-1.5 text-xs font-semibold text-slate-200">#${index + 1}</td>
        <td class="px-2 py-1.5">
            <p class="text-white text-sm font-semibold">${escapeHTML(team.name)}</p>
            <p class="text-[0.65rem] text-slate-400">Updated ${formatDate(team.lastUpdated)}</p>
        </td>
        <td class="px-2 py-1.5 text-right text-base font-bold text-white">${team.score}</td>
        <td class="px-2 py-1.5 text-right">
            <span class="inline-flex items-center rounded-lg px-2 py-0.5 text-[0.65rem] font-semibold ${summary.status}">
                ${summary.passed}/${summary.total || 0} up
            </span>
        </td>
    </tr>`;
}

function buildContainerMatrices(selected) {
    if (!selected || !Array.isArray(selected.teams)) {
        return [];
    }

    const containers = [];
    const containerIndex = new Map();

    const normalizeName = (value) => (value || "Container").toLowerCase();

    const ensureContainer = (name, sourceChecks = []) => {
        const normalized = normalizeName(name);
        if (!containerIndex.has(normalized)) {
            containerIndex.set(normalized, containers.length);
            containers.push({
                name: name || "Container",
                normalized,
                columns: sourceChecks.map((check) => ({
                    id: check?.id || check?.name || "",
                    name: check?.name || check?.id || "Service",
                })),
                rows: [],
            });
        } else if (sourceChecks.length) {
            const entry = containers[containerIndex.get(normalized)];
            sourceChecks.forEach((check) => {
                const id = check?.id || check?.name || "";
                if (!id) return;
                if (!entry.columns.some((column) => column.id === id)) {
                    entry.columns.push({
                        id,
                        name: check?.name || check?.id || "Service",
                    });
                }
            });
        }
    };

    selected.teams.forEach((team) => {
        (team.containers || []).forEach((container) => {
            ensureContainer(container?.name, Array.isArray(container?.checks) ? container.checks : []);
        });
    });

    if (!containers.length) {
        return [];
    }

    const sortedTeams = [...selected.teams].sort((a, b) => {
        const nameA = (a?.name || "").toLowerCase();
        const nameB = (b?.name || "").toLowerCase();
        if (nameA === nameB) {
            return 0;
        }
        return nameA < nameB ? -1 : 1;
    });

    sortedTeams.forEach((team, index) => {
        containers.forEach((containerEntry) => {
            const teamContainer = (team.containers || []).find(
                (container) => normalizeName(container?.name) === containerEntry.normalized
            );
            const statusMap = new Map();
            (teamContainer?.checks || []).forEach((check) => {
                const id = check?.id || check?.name || "";
                if (!id) return;
                statusMap.set(id, Boolean(check?.passed));
            });

            containerEntry.rows.push({
                team: team.name || `Team ${index + 1}`,
                statuses: containerEntry.columns.map((column) => statusMap.get(column.id)),
            });
        });
    });

    return containers;
}

function renderMatrixTable(matrix, index = 0) {
    const columns = matrix.columns;
    const rows = matrix.rows;
    const hasData = columns.length > 0;

    if (!hasData) {
        return `<div class="space-y-1 border-t border-white/5 pt-3">
            <p class="text-sm font-semibold text-white">${escapeHTML(matrix.name)}</p>
            <p class="text-xs text-slate-400">No services reported yet.</p>
        </div>`;
    }

    const gridTemplate = `grid-template-columns: auto repeat(${columns.length}, minmax(5rem, 1fr));`;

    const headerRow = `<div class="grid gap-1 text-[0.7rem] uppercase tracking-[0.2em] text-slate-300 pb-1 border-b border-white/10 mb-1"
        style="${gridTemplate}">
        ${index === 0 ? '<span>Team</span>' : '<span></span>'}
        ${columns
            .map(
                (column) =>
                    `<span class="text-center whitespace-nowrap">${escapeHTML(column.name)}</span>`
            )
            .join("")}
    </div>`;

    const body = rows
        .map((row) => {
            const cells = row.statuses
                .map((status) => {
                    let classes = "bg-white/5 text-slate-200 border-white/15";
                    let label = "—";
                    if (status === true) {
                        classes = "bg-emerald-500/20 text-emerald-100 border-emerald-400/30";
                        label = "Up";
                    } else if (status === false) {
                        classes = "bg-rose-500/25 text-rose-100 border-rose-400/30";
                        label = "Down";
                    }
                    return `<span class="inline-flex w-full items-center justify-center rounded-xl border px-3 py-1 text-[0.7rem] font-semibold leading-tight ${classes}">${label}</span>`;
                })
                .join("");
            return `<div class="grid items-center gap-1 text-[0.75rem]" style="${gridTemplate}">
                <span class="truncate font-semibold text-white/90">${escapeHTML(row.team)}</span>
                ${cells}
            </div>`;
        })
        .join("");

    const containerClass = index === 0 ? "pt-0" : "border-t border-white/5 pt-3";

    return `<div class="space-y-1.5 ${containerClass}">
        <div class="flex flex-wrap items-baseline gap-2 text-slate-300">
            <p class="text-sm font-semibold text-white/90">${escapeHTML(matrix.name)}</p>
        </div>
        <div class="space-y-1">
            ${headerRow}
            <div class="space-y-1">${body}</div>
        </div>
    </div>`;
}

function formatDate(value) {
    if (!value) return "—";
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "—";
    return dateFormatter.format(date);
}

function renderTabs() {
    if (!tabs) return;
    tabs.innerHTML = state.competitions
        .map(
            (comp) => `<button data-id="${comp.competitionID}" class="shrink-0 px-4 py-2 rounded-2xl text-sm font-semibold border ${
                state.selected === comp.competitionID
                    ? "bg-blue-500 text-white border-blue-400"
                    : "bg-white/5 text-white/90 border-white/20 hover:bg-white/10"
            }">${escapeHTML(comp.name)}</button>`
        )
        .join("");

    tabs.querySelectorAll("button").forEach((button) => {
        button.addEventListener("click", () => {
            state.selected = button.dataset.id;
            renderTabs();
            renderTable();
        });
    });
}

function renderTable() {
    if (!table) return;

    const selected = state.competitions.find((c) => c.competitionID === state.selected);

    if (!selected) {
        table.classList.add("hidden");
        empty?.classList.remove("hidden");
        return;
    }

    empty?.classList.add("hidden");
    table.classList.remove("hidden");

    const rows = selected.teams.length
        ? selected.teams.map((team, index) => renderRankingRow(team, index)).join("")
        : `<tr><td class="px-3 py-4 text-center text-slate-300" colspan="4">Scores are not ready yet.</td></tr>`;

    const scoringBadge = selected.scoringActive
        ? '<span class="inline-flex items-center rounded-full border border-emerald-400/40 bg-emerald-500/10 px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.2em] text-emerald-200">Scoring active</span>'
        : '<span class="inline-flex items-center rounded-full border border-amber-400/40 bg-amber-500/10 px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.2em] text-amber-200">Scoring paused</span>';

    const scoringNotice = selected.scoringActive
        ? ""
        : '<p class="text-[0.7rem] text-amber-200 bg-amber-500/10 border border-amber-500/20 rounded-2xl px-3 py-1">Scoring is currently paused for this competition. Results will not update until scoring resumes.</p>';

    const scoringControls =
        canManage && selected
            ? `<div class="flex items-center gap-2 justify-end">
                    <button class="text-xs rounded-full border px-3 py-1 font-semibold ${
                        selected.scoringActive
                            ? "border-amber-400/50 text-amber-200 hover:bg-amber-500/10"
                            : "border-emerald-400/60 text-emerald-200 hover:bg-emerald-500/10"
                    }" data-action="scoreboard-toggle" data-id="${escapeHTML(selected.competitionID)}" data-active="${
                  selected.scoringActive ? "true" : "false"
              }">${selected.scoringActive ? "Pause scoring" : "Start scoring"}</button>
                </div>`
            : "";

    table.innerHTML = `<div class="p-3 md:p-4 space-y-3 text-sm">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
            <div>
                <p class="text-xs uppercase tracking-[0.4em] text-slate-400">${selected.isPrivate ? "Private" : "Public"} event</p>
                <h2 class="text-xl font-semibold text-white">${escapeHTML(selected.name)}</h2>
                <p class="text-xs text-slate-300">${escapeHTML(selected.description || "No description provided.")}</p>
            </div>
            <div class="text-xs text-slate-300 text-right space-y-1">
                <p>${selected.teamCount} teams · ${selected.containerCount} containers</p>
                <p>Host: ${escapeHTML(selected.host || "Unknown")}</p>
                ${scoringBadge}
                ${scoringControls}
            </div>
        </div>
        ${scoringNotice}
        <div class="overflow-x-auto">
            <table class="min-w-full text-left">
                <thead>
                    <tr class="text-xs uppercase tracking-[0.3em] text-slate-400">
                        <th class="px-2 py-1.5">Rank</th>
                        <th class="px-2 py-1.5">Team</th>
                        <th class="px-2 py-1.5 text-right">Score</th>
                        <th class="px-2 py-1.5 text-right">Checks</th>
                    </tr>
                </thead>
                <tbody>
                    ${rows}
                </tbody>
            </table>
        </div>
        <div class="space-y-2 mt-3">
            ${renderGrid(selected)}
        </div>
    </div>`;
}

function renderGrid(selected) {
    if (!selected || !selected.teams.length) {
        return "";
    }

    const matrices = buildContainerMatrices(selected);
    if (!matrices.length) {
        return "";
    }

    return matrices.map((matrix, index) => renderMatrixTable(matrix, index)).join("");
}

async function loadScoreboard() {
    try {
        const response = await fetch("/api/scoreboard", { credentials: "include" });
        if (!response.ok) throw new Error("Failed to load scoreboard");
        const data = await response.json();
        state.competitions = data.competitions || [];

        if (!state.competitions.length) {
            state.selected = "";
            renderTabs();
            renderTable();
            return;
        }

        if (!state.selected || !state.competitions.some((c) => c.competitionID === state.selected)) {
            state.selected = state.competitions[0].competitionID;
        }

        renderTabs();
        renderTable();
    } catch (error) {
        console.error(error);
        if (empty) {
            empty.textContent = "Unable to load scoreboard right now.";
            empty.classList.remove("hidden");
        }
    }
}

if (root) {
    loadScoreboard();
    setInterval(loadScoreboard, REFRESH_INTERVAL);
    if (canManage && table) {
        table.addEventListener("click", (event) => {
            const button = event.target.closest("[data-action='scoreboard-toggle']");
            if (!button) return;
            const compID = button.dataset.id;
            const currentlyActive = button.dataset.active === "true";
            if (!compID) return;
            toggleScoringState(compID, !currentlyActive, button);
        });
    }
}

async function toggleScoringState(compID, nextState, button) {
    if (!compID) return;
    const originalText = button.textContent;
    button.disabled = true;
    button.textContent = nextState ? "Starting…" : "Pausing…";

    try {
        const response = await fetch(`/api/competitions/${encodeURIComponent(compID)}/scoring`, {
            method: "POST",
            credentials: "include",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({ active: nextState })
        });

        if (!response.ok) {
            const result = await response.json().catch(() => ({}));
            throw new Error(result?.error || result?.message || "Failed to toggle scoring");
        }

        await loadScoreboard();
    } catch (error) {
        console.error(error);
        alert(error.message || "Unable to update scoring state.");
    } finally {
        button.disabled = false;
        button.textContent = originalText;
    }
}
