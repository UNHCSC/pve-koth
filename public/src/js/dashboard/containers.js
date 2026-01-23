import { escapeHTML } from "../shared/utils.js";
import { describePowerStatus, formatRelativeTime } from "./helpers.js";

export function createContainerManager({ list, containerStates }) {
    let redeployHandler = null;

    function setRedeployHandler(handler) {
        redeployHandler = typeof handler === "function" ? handler : null;
    }

    function initCompetitionContainerState(compID) {
        const key = String(compID || "");
        if (!containerStates.has(key)) {
            containerStates.set(key, {
                selected: new Set(),
                data: [],
                loading: false,
                loaded: false,
                error: ""
            });
        }
        return containerStates.get(key);
    }

    function getContainerPanel(compID) {
        if (!list) {
            return null;
        }
        const encoded = encodeURIComponent(String(compID || ""));
        return list.querySelector(`[data-container-panel="${encoded}"]`);
    }

    function renderCompetitionContainerPanel(comp) {
        const compID = String(comp.competitionID || "");
        const encoded = encodeURIComponent(compID);
        const escapedID = escapeHTML(compID);
        return `
    <details class="group rounded-2xl border border-white/10 bg-slate-900/70" data-container-panel="${encoded}" data-comp-id="${escapedID}">
        <summary class="flex flex-col gap-1 px-4 py-3 cursor-pointer focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-blue-400">
            <div class="flex flex-col gap-2 lg:flex-row lg:items-center lg:justify-between">
                <div>
                    <p class="text-sm font-semibold text-white">Container control</p>
                    <p class="text-xs text-slate-400">Manage power state for ${escapeHTML(comp.name)}</p>
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
                <div class="flex flex-wrap gap-2">
                    <button type="button" data-container-action="refresh"
                        class="inline-flex items-center rounded-2xl border border-white/30 px-3 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] text-white/90 hover:bg-white/10 disabled:opacity-40">Refresh</button>
                    <button type="button" data-container-action="start"
                        class="inline-flex items-center rounded-2xl bg-emerald-500/20 px-3 py-1.5 text-xs font-semibold text-emerald-100 hover:bg-emerald-500/30 disabled:opacity-40">Start selected</button>
                    <button type="button" data-container-action="stop"
                        class="inline-flex items-center rounded-2xl bg-rose-500/20 px-3 py-1.5 text-xs font-semibold text-rose-100 hover:bg-rose-500/30 disabled:opacity-40">Stop selected</button>
                </div>
                <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                    <p class="text-xs uppercase tracking-[0.3em] text-slate-400" data-container-selection>Load the list to manage containers</p>
                    <label class="inline-flex items-center gap-2 text-xs text-slate-300">
                        <input type="checkbox" data-container-select-all class="h-4 w-4 rounded border-white/30 bg-slate-800/80" disabled>
                        Select all
                    </label>
                </div>
                <p class="hidden text-sm text-rose-400" data-container-error></p>
                <div class="text-slate-400 text-sm py-4 text-center border border-dashed border-white/10 rounded-2xl" data-container-empty>Not loaded yet.</div>
                <div class="overflow-x-auto">
                    <table class="min-w-full text-sm">
                <thead>
                    <tr class="text-xs uppercase tracking-[0.2em] text-slate-400 border-b border-white/10">
                        <th class="py-2 pr-3 text-left w-10"></th>
                        <th class="py-2 pr-3 text-left">Container</th>
                        <th class="py-2 pr-3 text-left">Network</th>
                        <th class="py-2 pr-3 text-left">Team</th>
                        <th class="py-2 pr-3 text-left">Power</th>
                        <th class="py-2 pr-3 text-left">Updated</th>
                        <th class="py-2 text-left">Actions</th>
                    </tr>
                </thead>
                        <tbody data-container-body class="divide-y divide-white/5"></tbody>
                    </table>
                </div>
            </div>
        </div>
    </details>`;
    }

    function renderCompetitionContainers(compID) {
        const panel = getContainerPanel(compID);
        if (!panel) {
            return;
        }
        const state = initCompetitionContainerState(compID);
        const selection = panel.querySelector("[data-container-selection]");
        const errorEl = panel.querySelector("[data-container-error]");
        const emptyEl = panel.querySelector("[data-container-empty]");
        const body = panel.querySelector("[data-container-body]");
        const selectAll = panel.querySelector("[data-container-select-all]");
        const startBtn = panel.querySelector('[data-container-action="start"]');
        const stopBtn = panel.querySelector('[data-container-action="stop"]');
        const refreshBtn = panel.querySelector('[data-container-action="refresh"]');

        const selectedCount = state.selected.size;
        const selectionLabel = state.loading
            ? "Loading containers..."
            : selectedCount > 0
            ? `${selectedCount} container${selectedCount === 1 ? "" : "s"} selected`
            : state.loaded
            ? state.data.length > 0
                ? "No containers selected"
                : "No containers found"
            : "Load the list to manage containers";
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

        const disableActions = state.loading || selectedCount === 0;
        if (startBtn) {
            startBtn.disabled = disableActions;
        }
        if (stopBtn) {
            stopBtn.disabled = disableActions;
        }
        if (refreshBtn) {
            refreshBtn.disabled = state.loading;
        }

        if (selectAll) {
            selectAll.disabled = state.loading || !state.loaded || state.data.length === 0;
            selectAll.checked = state.data.length > 0 && selectedCount === state.data.length;
            selectAll.indeterminate = state.data.length > 0 && selectedCount > 0 && selectedCount < state.data.length;
        }

        if (!body) {
            return;
        }

        if (!state.loaded) {
            body.innerHTML = "";
            if (emptyEl) {
                emptyEl.textContent = "Use refresh to load containers.";
                emptyEl.classList.remove("hidden");
            }
            return;
        }

        if (!Array.isArray(state.data) || state.data.length === 0) {
            body.innerHTML = "";
            if (emptyEl) {
                emptyEl.textContent = "No containers available.";
                emptyEl.classList.remove("hidden");
            }
            return;
        }

        if (emptyEl) {
            emptyEl.classList.add("hidden");
        }

        const rows = state.data
            .map(function(entry) {
                if (!entry || typeof entry.id === "undefined") {
                    return "";
                }
                const id = Number(entry.id);
                if (!Number.isFinite(id)) {
                    return "";
                }

                const checked = state.selected.has(id);
                const teamName = entry.team ? escapeHTML(entry.team.name || `Team ${entry.team.id}`) : "Unassigned";
                const teamMeta = entry.team ? `<p class="text-xs text-slate-400">ID ${entry.team.id}</p>` : "";
                const nodeInfo = entry.node ? `<p class="text-xs text-slate-400">Node ${escapeHTML(entry.node)}</p>` : "";
                const ip = entry.ipAddress ? escapeHTML(entry.ipAddress) : "—";
                const status = describePowerStatus(entry.status);
                const containerName = entry.name || `CT-${id}`;
                const label = escapeHTML(containerName);
                const configName = entry.containerConfigName ? escapeHTML(entry.containerConfigName) : "";
                const redeployDisabled = state.loading;

                return `<tr class="border-b border-white/5 last:border-b-0">
                    <td class="py-3 pr-3 align-top">
                        <input type="checkbox" class="h-4 w-4 rounded border-white/30 bg-slate-800/80" data-container-select value="${id}" ${checked ? "checked" : ""}>
                    </td>
                <td class="py-3 pr-3 align-top">
                    <p class="font-semibold text-white">${label} <span class="text-slate-400 text-xs">(${id})</span></p>
                </td>
                <td class="py-3 pr-3 align-top">
                    <p class="font-mono text-slate-100">${ip}</p>
                    ${nodeInfo}
                </td>
                <td class="py-3 pr-3 align-top">
                    <p class="text-slate-100">${teamName}</p>
                    ${teamMeta}
                </td>
                    <td class="py-3 pr-3 align-top">
                        <span class="status-pill ${status.tone}">${escapeHTML(status.label)}</span>
                    </td>
                    <td class="py-3 pr-3 align-top text-slate-300">${formatRelativeTime(entry.lastUpdated)}</td>
                    <td class="py-3 align-top">
                        <button type="button" data-container-redeploy data-container-id="${id}" data-container-label="${label}" class="text-xs font-semibold text-blue-200 hover:text-white disabled:opacity-40" ${redeployDisabled ? "disabled" : ""}>
                            Redeploy container
                        </button>
                        ${configName ? `<p class="text-[10px] uppercase tracking-[0.2em] text-slate-400 mt-1">Config: ${configName}</p>` : "<p class=\"text-[10px] uppercase tracking-[0.2em] text-slate-500 mt-1\">Config: unspecified</p>"}
                    </td>
                </tr>`;
            })
            .join("");

        body.innerHTML = rows;
    }

    async function loadCompetitionContainers(compID) {
        const state = initCompetitionContainerState(compID);
        state.loading = true;
        state.error = "";
        renderCompetitionContainers(compID);

        try {
            const response = await fetch(`/api/containers?competition=${encodeURIComponent(compID)}`, { credentials: "include" });
            const payload = await response.json().catch(function() {
                return {};
            });
            if (!response.ok) {
                throw new Error(payload?.error || payload?.message || "Failed to load containers");
            }
            const containers = Array.isArray(payload?.containers) ? payload.containers : [];
            state.data = containers;
            state.loaded = true;
            state.selected = new Set(
                Array.from(state.selected).filter(function(id) {
                    return containers.some(function(entry) {
                        return Number(entry?.id) === Number(id);
                    });
                })
            );
        } catch (error) {
            state.error = error.message || "Unable to load containers.";
            state.data = [];
            state.loaded = true;
        } finally {
            state.loading = false;
            renderCompetitionContainers(compID);
        }
    }

    async function handleCompetitionBulkPower(compID, action) {
        const state = initCompetitionContainerState(compID);
        if (state.selected.size === 0) {
            return;
        }
        state.loading = true;
        state.error = "";
        renderCompetitionContainers(compID);

        try {
            const response = await fetch("/api/containers/power", {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ ids: Array.from(state.selected), action })
            });
            const payload = await response.json().catch(function() {
                return {};
            });
            if (!response.ok) {
                throw new Error(payload?.error || payload?.message || `Failed to ${action} containers`);
            }
            window.alert(payload?.message || `Containers queued to ${action}.`);
            state.loading = false;
            renderCompetitionContainers(compID);
            await loadCompetitionContainers(compID);
        } catch (error) {
            state.loading = false;
            state.error = error.message || `Unable to ${action} containers.`;
            renderCompetitionContainers(compID);
        }
    }

    function handleContainerRowSelection(checkbox) {
        const panel = checkbox.closest("[data-container-panel]");
        if (!panel) {
            return;
        }
        const compID = panel.dataset.compId || "";
        const id = Number(checkbox.value);
        if (!Number.isFinite(id)) {
            return;
        }
        const state = initCompetitionContainerState(compID);
        if (checkbox.checked) {
            state.selected.add(id);
        } else {
            state.selected.delete(id);
        }
        renderCompetitionContainers(compID);
    }

    function handleContainerSelectAll(checkbox) {
        const panel = checkbox.closest("[data-container-panel]");
        if (!panel) {
            return;
        }
        const compID = panel.dataset.compId || "";
        const state = initCompetitionContainerState(compID);
        state.selected.clear();
        if (checkbox.checked) {
            state.data.forEach(function(entry) {
                const id = Number(entry?.id);
                if (Number.isFinite(id)) {
                    state.selected.add(id);
                }
            });
        }
        renderCompetitionContainers(compID);
    }

    function handleContainerRedeploy(button) {
        const panel = button.closest("[data-container-panel]");
        if (!panel) {
            return;
        }
        const compID = panel.dataset.compId || "";
        const id = Number(button.dataset.containerId);
        if (!Number.isFinite(id)) {
            return;
        }

        const containerLabel = button.dataset.containerLabel || `CT-${id}`;
        if (typeof redeployHandler === "function") {
            redeployHandler(containerLabel, id, compID);
        }
    }

    return {
        setRedeployHandler,
        initCompetitionContainerState,
        renderCompetitionContainerPanel,
        renderCompetitionContainers,
        loadCompetitionContainers,
        handleCompetitionBulkPower,
        handleContainerRowSelection,
        handleContainerSelectAll,
        handleContainerRedeploy
    };
}
