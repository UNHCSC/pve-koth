import { escapeHTML } from "./shared/utils.js";
import { setupCreateCompetitionMenu } from "./dashboard/createCompetition.js";
import { createContainerManager } from "./dashboard/containers.js";
import { createTeamManager } from "./dashboard/teams.js";
import { createRedeployController } from "./dashboard/redeploy.js";
import { createTeardownController } from "./dashboard/teardown.js";

const list = document.getElementById("comps");
const emptyState = document.getElementById("empty");
const statContainer = document.getElementById("dashboard-stats");
const refreshButton = document.getElementById("refresh-dashboard");
const canManage = list?.dataset.canManage === "true";

const containerStates = new Map();
const teamStates = new Map();

const containerManager = createContainerManager({ list, containerStates });
const teamManager = createTeamManager({ list, teamStates });
const redeployController = createRedeployController({ loadCompetitionContainers: containerManager.loadCompetitionContainers });
containerManager.setRedeployHandler(redeployController.openRedeployModal);

let teardownController;

function setStats({ total = 0, publicCount = 0, privateCount = 0 } = {}) {
    if (!statContainer) {
        return;
    }
    statContainer.querySelector("[data-stat=\"total\"]").textContent = total;
    statContainer.querySelector("[data-stat=\"public\"]").textContent = publicCount;
    statContainer.querySelector("[data-stat=\"private\"]").textContent = privateCount;
}

function renderCompetitions(competitions = []) {
    if (!list) {
        return;
    }

    const publicCount = competitions.filter(function(comp) {
        return !comp.isPrivate;
    }).length;
    const privateCount = competitions.length - publicCount;
    setStats({ total: competitions.length, publicCount, privateCount });

    if (!competitions.length) {
        emptyState?.classList.remove("hidden");
        list.innerHTML = "";
        return;
    }

    emptyState?.classList.add("hidden");
    list.innerHTML = competitions
        .map(function(comp) {
            const badge = comp.isPrivate
                ? "<span class=\"ml-2 rounded-full bg-rose-500/20 text-rose-200 text-xs px-2 py-0.5\">Private</span>"
                : "<span class=\"ml-2 rounded-full bg-emerald-500/20 text-emerald-200 text-xs px-2 py-0.5\">Public</span>";
            const scoringBadge = comp.scoringActive
                ? "<span class=\"ml-2 rounded-full bg-emerald-500/20 text-emerald-200 text-xs px-2 py-0.5\">Scoring active</span>"
                : "<span class=\"ml-2 rounded-full bg-amber-500/20 text-amber-100 text-xs px-2 py-0.5\">Scoring paused</span>";
            const networkLabel = comp.networkCIDR ? escapeHTML(comp.networkCIDR) : "Not assigned";
            const containerMarkup = canManage ? containerManager.renderCompetitionContainerPanel(comp) : "";
            const teamMarkup = canManage ? teamManager.renderCompetitionTeamPanel(comp) : "";
            const actions = `
                <div class="flex flex-col items-end gap-2 mt-2">
                    <a class="text-blue-300 hover:text-blue-200" href="/scoreboard/${encodeURIComponent(comp.competitionID)}">Open scoreboard</a>
                    ${
                        canManage
                            ? `<div class="flex flex-col gap-2 items-end">
                                <button class="inline-flex items-center rounded-xl border border-white/40 px-3 py-1 text-xs font-semibold text-white/90 hover:bg-white/10 focus:outline-none focus:ring-2 focus:ring-blue-400 disabled:opacity-60"
                                    data-action="toggle-scoring"
                                    data-active="${comp.scoringActive ? "true" : "false"}"
                                    data-id="${escapeHTML(comp.competitionID)}"
                                >${comp.scoringActive ? "Stop scoring" : "Start scoring"}</button>
                                <button class="inline-flex items-center rounded-xl border border-rose-500/60 px-3 py-1 text-xs font-semibold text-rose-200 hover:bg-rose-500/10 focus:outline-none focus:ring-2 focus:ring-rose-400 disabled:opacity-60"
                                data-action="teardown"
                                data-id="${escapeHTML(comp.competitionID)}"
                                data-name="${escapeHTML(comp.name)}"
                            >Tear down</button></div>`
                            : ""
                    }
                </div>`;

            return `<li class="rounded-2xl border border-white/10 bg-white/5 p-5 flex flex-col gap-4">
                <div class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                <div>
                    <p class="text-lg font-semibold text-white">${escapeHTML(comp.name)}${badge}${scoringBadge}</p>
                    <p class="text-sm text-slate-300">${escapeHTML(comp.description || "No description")}</p>
                    <p class="text-xs text-slate-400 mt-1">Hosted by ${escapeHTML(comp.host || "Unknown")}</p>
                    <p class="text-xs text-slate-400 mt-1">Network: ${networkLabel}</p>
                </div>
                <div class="text-sm text-right text-slate-300">
                    <p>${comp.teamCount} teams · ${comp.containerCount} containers</p>
                    ${actions}
                </div>
                </div>
                ${canManage ? `${containerMarkup}${teamMarkup}` : ""}
            </li>`;
        })
        .join("");

    if (canManage) {
        const activeIDs = new Set(competitions.map(function(comp) {
            return String(comp?.competitionID || "");
        }));
        Array.from(containerStates.keys()).forEach(function(key) {
            if (!activeIDs.has(key)) {
                containerStates.delete(key);
            }
        });
        Array.from(teamStates.keys()).forEach(function(key) {
            if (!activeIDs.has(key)) {
                teamStates.delete(key);
            }
        });
        competitions.forEach(function(comp) {
            if (!comp || !comp.competitionID) {
                return;
            }
            const compID = comp.competitionID;
            const containerState = containerManager.initCompetitionContainerState(compID);
            containerManager.renderCompetitionContainers(compID);
            if (!containerState.loaded && !containerState.loading) {
                containerManager.loadCompetitionContainers(compID);
            }

            const teamState = teamManager.initCompetitionTeamState(compID);
            teamManager.renderCompetitionTeams(compID);
            if (!teamState.loaded && !teamState.loading) {
                teamManager.loadCompetitionTeams(compID);
            }
        });
    }
}

function teardownCompetition(button) {
    if (!button || !teardownController) {
        return;
    }
    const compID = button.dataset.id;
    if (!compID) {
        return;
    }
    const compName = button.dataset.name || compID;

    teardownController.openTeardownModal(compName, compID);
}

async function toggleScoring(button) {
    if (!button) {
        return;
    }
    const compID = button.dataset.id;
    const currentlyActive = button.dataset.active === "true";
    if (!compID) {
        return;
    }

    const nextState = !currentlyActive;
    const originalText = button.textContent;
    button.disabled = true;
    button.textContent = nextState ? "Starting…" : "Stopping…";

    try {
        const response = await fetch(`/api/competitions/${encodeURIComponent(compID)}/scoring`, {
            method: "POST",
            credentials: "include",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify({ active: nextState })
        });

        const result = await response.json().catch(function() {
            return {};
        });
        if (!response.ok) {
            throw new Error(result?.error || result?.message || "Failed to update scoring");
        }

        await loadDashboard();
    } catch (error) {
        console.error(error);
        window.alert(error.message || "Unable to update scoring state.");
    } finally {
        button.disabled = false;
        button.textContent = originalText;
    }
}

function handleListClick(event) {
    if (!(event.target instanceof Element)) {
        return;
    }
    const teamPanelControl = event.target.closest("[data-team-panel-action]");
    if (teamPanelControl) {
        const panel = teamPanelControl.closest("[data-team-panel]");
        const compID = panel?.dataset.compId || "";
        const action = teamPanelControl.dataset.teamPanelAction;
        if (action === "refresh" && compID) {
            teamManager.loadCompetitionTeams(compID);
        }
        return;
    }
    const teamAction = event.target.closest("[data-team-action]");
    if (teamAction && teamAction.dataset.teamAction) {
        teamManager.handleTeamAction(teamAction);
        return;
    }
    const redeployButton = event.target.closest("[data-container-redeploy]");
    if (redeployButton) {
        containerManager.handleContainerRedeploy(redeployButton);
        return;
    }
    const containerButton = event.target.closest("[data-container-action]");
    if (containerButton) {
        const panel = containerButton.closest("[data-container-panel]");
        if (!panel) {
            return;
        }
        const compID = panel.dataset.compId || "";
        const action = containerButton.dataset.containerAction;
        if (action === "refresh") {
            containerManager.loadCompetitionContainers(compID);
            return;
        }
        if (action === "start" || action === "stop") {
            containerManager.handleCompetitionBulkPower(compID, action);
            return;
        }
    }
    const toggle = event.target.closest("[data-action=\"toggle-scoring\"]");
    if (toggle) {
        toggleScoring(toggle);
        return;
    }
    const teardownTarget = event.target.closest("[data-action=\"teardown\"]");
    if (teardownTarget) {
        teardownCompetition(teardownTarget);
    }
}

function handleListChange(event) {
    if (!(event.target instanceof Element)) {
        return;
    }
    const teamSelect = event.target.closest("[data-team-select]");
    if (teamSelect) {
        teamManager.handleTeamRowSelection(teamSelect);
        return;
    }
    const teamSelectAll = event.target.closest("[data-team-select-all]");
    if (teamSelectAll) {
        teamManager.handleTeamSelectAll(teamSelectAll);
        return;
    }
    const rowCheckbox = event.target.closest("[data-container-select]");
    if (rowCheckbox) {
        containerManager.handleContainerRowSelection(rowCheckbox);
        return;
    }
    const selectAll = event.target.closest("[data-container-select-all]");
    if (selectAll) {
        containerManager.handleContainerSelectAll(selectAll);
    }
}

function handleListToggle(event) {
    if (!(event.target instanceof Element)) {
        return;
    }
    const toggleTarget = event.target.closest("[data-team-panel]");
    if (!toggleTarget || !toggleTarget.open) {
        return;
    }
    const compID = toggleTarget.dataset.compId || "";
    if (!compID) {
        return;
    }
    const state = teamManager.initCompetitionTeamState(compID);
    if (!state.loaded && !state.loading) {
        teamManager.loadCompetitionTeams(compID);
    }
}

async function loadDashboard() {
    try {
        const response = await fetch("/api/competitions", { credentials: "include" });
        if (!response.ok) {
            throw new Error("Failed to load competitions");
        }
        const data = await response.json();
        renderCompetitions(data.competitions || []);
    } catch (error) {
        console.error(error);
        if (emptyState) {
            emptyState.textContent = "We couldn't load competitions right now.";
            emptyState.classList.remove("hidden");
        }
    }
}

teardownController = createTeardownController({ loadDashboard });

refreshButton?.addEventListener("click", loadDashboard);

if (canManage && list) {
    list.addEventListener("click", handleListClick);
    list.addEventListener("change", handleListChange);
    list.addEventListener("toggle", handleListToggle);
}

setupCreateCompetitionMenu({ loadDashboard });
loadDashboard();
