import { buildTabsMarkup, buildTableMarkup } from "./scoreboard/rendering.js";
import { animateScoreCells } from "./scoreboard/animations.js";

const root = document.getElementById("scoreboard-root");
const tabs = document.getElementById("scoreboard-tabs");
const table = document.getElementById("scoreboard-table");
const empty = document.getElementById("scoreboard-empty");
const canManage = root?.dataset.canManage === "true";
const REFRESH_INTERVAL = 30_000;

const state = {
    competitions: [],
    selected: root?.dataset.selected || ""
};

const scoreHistory = new Map();
const scoreAnimationTokens = new Map();

function renderTabs() {
    if (!tabs) {
        return;
    }
    tabs.innerHTML = buildTabsMarkup(state.competitions, state.selected);
    tabs.querySelectorAll("button").forEach(function(button) {
        button.addEventListener("click", function() {
            state.selected = button.dataset.id;
            renderTabs();
            renderTable();
        });
    });
}

function renderTable() {
    if (!table) {
        return;
    }

    const selected = state.competitions.find(function(c) {
        return c.competitionID === state.selected;
    });

    if (!selected) {
        table.classList.add("hidden");
        empty?.classList.remove("hidden");
        return;
    }

    empty?.classList.add("hidden");
    table.classList.remove("hidden");

    table.innerHTML = buildTableMarkup(selected, canManage);
    animateScoreCells(table, selected.teams, scoreHistory, scoreAnimationTokens);
}

async function loadScoreboard() {
    try {
        const response = await fetch("/api/scoreboard", { credentials: "include" });
        if (!response.ok) {
            throw new Error("Failed to load scoreboard");
        }
        const data = await response.json();
        state.competitions = data.competitions || [];

        if (!state.competitions.length) {
            state.selected = "";
            renderTabs();
            renderTable();
            return;
        }

        if (!state.selected || !state.competitions.some(function(c) {
            return c.competitionID === state.selected;
        })) {
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
        table.addEventListener("click", function(event) {
            const button = event.target.closest("[data-action=\"scoreboard-toggle\"]");
            if (!button) {
                return;
            }
            const compID = button.dataset.id;
            const currentlyActive = button.dataset.active === "true";
            if (!compID) {
                return;
            }
            toggleScoringState(compID, !currentlyActive, button);
        });
    }
}

async function toggleScoringState(compID, nextState, button) {
    if (!compID || !button) {
        return;
    }
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
            const result = await response.json().catch(function() {
                return {};
            });
            throw new Error(result?.error || result?.message || "Failed to toggle scoring");
        }

        await loadScoreboard();
    } catch (error) {
        console.error(error);
        window.alert(error.message || "Unable to update scoring state.");
    } finally {
        button.disabled = false;
        button.textContent = originalText;
    }
}
