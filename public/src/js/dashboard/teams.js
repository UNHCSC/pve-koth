import { escapeHTML } from "../shared/utils.js";
import { formatRelativeTime } from "./helpers.js";

export function createTeamManager({ list, teamStates }) {
    function initCompetitionTeamState(compID) {
        const key = String(compID || "");
        if (!teamStates.has(key)) {
            teamStates.set(key, {
                actionLoading: false,
                error: "",
                feedback: { text: "", tone: "text-slate-400" },
                loaded: false,
                loading: false,
                selected: new Set(),
                teams: []
            });
        }
        return teamStates.get(key);
    }

    function getTeamPanel(compID) {
        if (!list) {
            return null;
        }
        const encoded = encodeURIComponent(String(compID || ""));
        return list.querySelector(`[data-team-panel="${encoded}"]`);
    }

    function renderCompetitionTeamPanel(comp) {
        const compID = String(comp.competitionID || "");
        const encoded = encodeURIComponent(compID);
        const escapedID = escapeHTML(compID);
        return `
    <details class="group rounded-2xl border border-white/10 bg-slate-900/70" data-team-panel="${encoded}" data-comp-id="${escapedID}">
        <summary class="flex flex-col gap-1 px-4 py-3 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400">
            <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <div>
                    <p class="text-sm font-semibold text-white">Team control</p>
                    <p class="text-xs text-slate-400">Adjust scores for ${escapeHTML(comp.name)}</p>
                </div>
                <span class="chevron-icon text-white/80" aria-hidden="true">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
                        <path d="M6 9l6 6 6-6"></path>
                    </svg>
                </span>
            </div>
        </summary>
        <div class="panel-content">
            <div class="panel-body space-y-3 border-t border-white/10 px-4 pb-4 pt-3">
                <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <p class="text-xs uppercase tracking-[0.3em] text-slate-400" data-team-selection>&nbsp;</p>
                    <label class="inline-flex items-center gap-2 text-xs text-slate-300">
                        <input type="checkbox" data-team-select-all class="h-4 w-4 rounded border-white/30 bg-slate-800/80" disabled>
                        Select all
                    </label>
                </div>
                <div class="flex flex-wrap gap-2">
                    <button class="inline-flex items-center rounded-2xl border border-white/30 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] text-white/90 hover:bg-white/10 disabled:opacity-40" type="button" data-team-panel-action="refresh" disabled>Refresh teams</button>
                    <button class="inline-flex items-center justify-center rounded-2xl border border-white/30 px-3 py-2 text-xs font-semibold uppercase tracking-[0.3em] text-white/90 hover:bg-white/10 disabled:opacity-50" type="button" data-team-action="reset" disabled>Reset score</button>
                    <input type="number" step="1" class="flex-1 min-w-[120px] rounded-2xl border border-white/10 bg-slate-900/80 px-3 py-2 text-sm text-white" placeholder="+10 / -5" data-team-adjust-value disabled>
                    <button class="inline-flex items-center justify-center rounded-2xl bg-blue-600/80 px-3 py-2 text-xs font-semibold uppercase tracking-[0.3em] text-white hover:bg-blue-500 disabled:opacity-50" type="button" data-team-action="adjust" disabled>Modify score</button>
                </div>
                <p class="text-sm text-slate-400" data-team-feedback>Use the controls above to adjust team scores.</p>
                <p class="hidden text-sm text-rose-400" data-team-error></p>
                <div class="text-slate-400 text-sm py-4 text-center border border-dashed border-white/10 rounded-2xl" data-team-empty>Not loaded yet.</div>
                <div class="overflow-x-auto">
                    <table class="min-w-full text-sm">
                        <thead>
                            <tr class="text-xs uppercase tracking-[0.2em] text-slate-400 border-b border-white/10">
                                <th class="py-2 pr-3 text-left w-10"></th>
                                <th class="py-2 pr-3 text-left">Team</th>
                                <th class="py-2 pr-3 text-left">Network</th>
                                <th class="py-2 pr-3 text-left">Score</th>
                                <th class="py-2 text-left">Updated</th>
                            </tr>
                        </thead>
                        <tbody data-team-body class="divide-y divide-white/5"></tbody>
                    </table>
                </div>
            </div>
        </div>
    </details>`;
    }

    function renderCompetitionTeams(compID) {
        const panel = getTeamPanel(compID);
        if (!panel) {
            return;
        }
        const state = initCompetitionTeamState(compID);
        const selection = panel.querySelector("[data-team-selection]");
        const selectAll = panel.querySelector("[data-team-select-all]");
        const body = panel.querySelector("[data-team-body]");
        const resetBtn = panel.querySelector("[data-team-action=\"reset\"]");
        const adjustBtn = panel.querySelector("[data-team-action=\"adjust\"]");
        const adjustInput = panel.querySelector("[data-team-adjust-value]");
        const refreshBtn = panel.querySelector("[data-team-panel-action=\"refresh\"]");
        const feedbackEl = panel.querySelector("[data-team-feedback]");
        const errorEl = panel.querySelector("[data-team-error]");
        const emptyEl = panel.querySelector("[data-team-empty]");

        const selectedCount = state.selected.size;
        const selectionLabel = state.loading
            ? "Loading teams..."
            : state.loaded
            ? state.teams.length > 0
                ? selectedCount > 0
                    ? `${selectedCount} team${selectedCount === 1 ? "" : "s"} selected`
                    : "Select teams to adjust"
                : "No teams available"
            : "Expand to load teams.";

        if (selection) {
            selection.textContent = selectionLabel;
        }

        if (errorEl) {
            if (state.error) {
                errorEl.textContent = state.error;
                errorEl.classList.remove("hidden");
            } else {
                errorEl.textContent = "";
                errorEl.classList.add("hidden");
            }
        }

        const actionDisabled = state.loading || state.actionLoading || selectedCount === 0;
        if (resetBtn) {
            resetBtn.disabled = actionDisabled;
        }
        if (adjustBtn) {
            adjustBtn.disabled = actionDisabled;
        }
        if (adjustInput) {
            adjustInput.disabled = actionDisabled;
        }
        if (refreshBtn) {
            refreshBtn.disabled = state.loading;
        }

        if (selectAll) {
            selectAll.disabled = state.loading || state.teams.length === 0;
            selectAll.checked = state.teams.length > 0 && selectedCount === state.teams.length;
            selectAll.indeterminate = selectedCount > 0 && selectedCount < state.teams.length;
        }

        if (feedbackEl) {
            if (state.error) {
                feedbackEl.textContent = state.error;
                feedbackEl.className = "text-sm text-rose-400";
            } else if (state.feedback?.text) {
                feedbackEl.textContent = state.feedback.text;
                feedbackEl.className = `text-sm ${state.feedback.tone}`;
            } else {
                feedbackEl.textContent = "Use the controls above to adjust team scores.";
                feedbackEl.className = "text-sm text-slate-400";
            }
        }

        if (!body) {
            return;
        }

        if (!state.loaded) {
            body.innerHTML = "";
            if (emptyEl) {
                emptyEl.textContent = "Expand the panel to load teams.";
                emptyEl.classList.remove("hidden");
            }
            return;
        }

        if (!state.teams.length) {
            body.innerHTML = "";
            if (emptyEl) {
                emptyEl.textContent = "No teams available.";
                emptyEl.classList.remove("hidden");
            }
            return;
        }

        if (emptyEl) {
            emptyEl.classList.add("hidden");
        }

        const rows = state.teams
            .map(function(team) {
                const checked = state.selected.has(team.id);
                const name = escapeHTML(team.name || `Team ${team.id}`);
                const score = Number.isFinite(Number(team.score)) ? Number(team.score) : 0;
                const updated = formatRelativeTime(team.lastUpdated);
                const networkLabel = team.network ? escapeHTML(team.network) : "—";
                return `<tr class="border-b border-white/5 last:border-b-0">
                <td class="py-3 pr-3 align-top">
                    <input type="checkbox" class="h-4 w-4 rounded border-white/30 bg-slate-800/80" data-team-select value="${team.id}" ${checked ? "checked" : ""}>
                </td>
                <td class="py-3 pr-3 align-top">
                    <p class="text-slate-100 font-semibold">${name}</p>
                    <p class="text-xs text-slate-400">ID ${team.id}</p>
                </td>
                <td class="py-3 pr-3 align-top">
                    <p class="text-slate-100 font-semibold">${networkLabel}</p>
                </td>
                <td class="py-3 pr-3 align-top">
                    <p class="text-slate-100 font-semibold">${score}</p>
                </td>
                <td class="py-3 align-top text-slate-300">${updated}</td>
            </tr>`;
            })
            .join("");
        body.innerHTML = rows;
    }

    async function loadCompetitionTeams(compID) {
        const state = initCompetitionTeamState(compID);
        if (!state || state.loading) {
            return;
        }
        state.loading = true;
        state.error = "";
        renderCompetitionTeams(compID);

        try {
            const response = await fetch(`/api/competitions/${encodeURIComponent(compID)}/teams`, {
                credentials: "include"
            });
            const payload = await response.json().catch(function() {
                return {};
            });
            if (!response.ok) {
                throw new Error(payload?.error || payload?.message || "Failed to load teams");
            }

            const teams = Array.isArray(payload?.teams) ? payload.teams : [];
            const normalized = [];
            teams.forEach(function(entry) {
                if (!entry || !Number.isFinite(Number(entry.id))) {
                    return;
                }
                normalized.push({
                    id: Number(entry.id),
                    name: entry.name || `Team ${entry.id}`,
                    score: Number.isFinite(Number(entry.score)) ? Number(entry.score) : 0,
                    lastUpdated: entry.lastUpdated || "",
                    network: entry.networkCIDR || ""
                });
            });

            state.teams = normalized;
            const availableIDs = new Set(normalized.map(function(team) {
                return team.id;
            }));
            state.selected = new Set(Array.from(state.selected).filter(function(id) {
                return availableIDs.has(id);
            }));
            state.error = "";
            state.loaded = true;
        } catch (error) {
            state.error = error.message || "Unable to load teams.";
            state.teams = [];
            state.selected.clear();
            state.loaded = true;
        } finally {
            state.loading = false;
            renderCompetitionTeams(compID);
        }
    }

    function handleTeamRowSelection(checkbox) {
        const panel = checkbox.closest("[data-team-panel]");
        if (!panel) {
            return;
        }
        const compID = panel.dataset.compId || "";
        if (!compID) {
            return;
        }
        const state = initCompetitionTeamState(compID);
        if (!state) {
            return;
        }
        const id = Number(checkbox.value);
        if (!Number.isFinite(id)) {
            return;
        }
        if (checkbox.checked) {
            state.selected.add(id);
        } else {
            state.selected.delete(id);
        }
        renderCompetitionTeams(compID);
    }

    function handleTeamSelectAll(checkbox) {
        const panel = checkbox.closest("[data-team-panel]");
        if (!panel) {
            return;
        }
        const compID = panel.dataset.compId || "";
        if (!compID) {
            return;
        }
        const state = initCompetitionTeamState(compID);
        if (!state) {
            return;
        }
        if (checkbox.checked) {
            state.teams.forEach(function(team) {
                state.selected.add(team.id);
            });
        } else {
            state.selected.clear();
        }
        renderCompetitionTeams(compID);
    }

    async function handleTeamAction(button) {
        const panel = button.closest("[data-team-panel]");
        if (!panel) {
            return;
        }
        const compID = panel.dataset.compId || "";
        if (!compID) {
            return;
        }
        const state = initCompetitionTeamState(compID);
        if (!state) {
            return;
        }

        const selectedIDs = Array.from(state.selected);
        if (!selectedIDs.length) {
            state.feedback = { text: "Select at least one team.", tone: "text-rose-400" };
            renderCompetitionTeams(compID);
            return;
        }

        const action = button.dataset.teamAction;
        if (!action) {
            return;
        }

        let amount;
        if (action === "adjust") {
            const input = panel.querySelector("[data-team-adjust-value]");
            const rawValue = (input?.value || "").trim();
            if (rawValue === "") {
                state.feedback = { text: "Enter a value to adjust the score.", tone: "text-rose-400" };
                renderCompetitionTeams(compID);
                return;
            }
            amount = Number(rawValue);
            if (!Number.isFinite(amount) || !Number.isInteger(amount)) {
                state.feedback = { text: "Adjustment must be a whole number.", tone: "text-rose-400" };
                renderCompetitionTeams(compID);
                return;
            }
            if (amount === 0) {
                state.feedback = { text: "Amount must be non-zero.", tone: "text-rose-400" };
                renderCompetitionTeams(compID);
                return;
            }
        }

        const payload = { action };
        if (action === "adjust") {
            payload.amount = amount;
        }

        state.actionLoading = true;
        state.error = "";
        state.feedback = { text: "", tone: "text-slate-400" };
        renderCompetitionTeams(compID);

        try {
            for (const teamID of selectedIDs) {
                const response = await fetch(`/api/competitions/${encodeURIComponent(compID)}/teams/${teamID}/score`, {
                    method: "POST",
                    credentials: "include",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(payload)
                });
                const result = await response.json().catch(function() {
                    return {};
                });
                if (!response.ok) {
                    throw new Error(result?.error || result?.message || "Failed to update team score");
                }

                const team = state.teams.find(function(entry) {
                    return entry.id === teamID;
                });
                if (team) {
                    if (Number.isFinite(Number(result?.score))) {
                        team.score = Number(result.score);
                    }
                    team.lastUpdated = result?.lastUpdated || new Date().toISOString();
                }
            }

            state.feedback = {
                text: `${action === "reset" ? "Scores reset" : "Scores updated"} for ${selectedIDs.length} team${selectedIDs.length === 1 ? "" : "s"}.`,
                tone: "text-emerald-400"
            };

            if (action === "adjust") {
                const input = panel.querySelector("[data-team-adjust-value]");
                if (input) {
                    input.value = "";
                }
            }
        } catch (error) {
            state.error = error.message || "Unable to update team score.";
            state.feedback = { text: "", tone: "text-slate-400" };
        } finally {
            state.actionLoading = false;
            renderCompetitionTeams(compID);
        }
    }

    return {
        initCompetitionTeamState,
        renderCompetitionTeamPanel,
        renderCompetitionTeams,
        loadCompetitionTeams,
        handleTeamAction,
        handleTeamRowSelection,
        handleTeamSelectAll
    };
}
