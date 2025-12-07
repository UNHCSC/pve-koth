import { escapeHTML } from "./shared/utils.js";

const root = document.getElementById("scoreboard-root");
const tabs = document.getElementById("scoreboard-tabs");
const table = document.getElementById("scoreboard-table");
const empty = document.getElementById("scoreboard-empty");
const REFRESH_INTERVAL = 30_000;
const dateFormatter = new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short",
});

const state = {
    competitions: [],
    selected: root?.dataset.selected || "",
};

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
        ? selected.teams
              .map(
                  (team, index) => `<tr class="${
                      index === 0
                          ? "bg-white/10"
                          : index % 2 === 0
                          ? "bg-white/[0.04]"
                          : "bg-transparent"
                  }">
                    <td class="px-4 py-3 text-sm font-semibold text-slate-200">#${index + 1}</td>
                    <td class="px-4 py-3">
                        <p class="text-white font-semibold">${escapeHTML(team.name)}</p>
                        <p class="text-xs text-slate-400">Updated ${formatDate(team.lastUpdated)}</p>
                    </td>
                    <td class="px-4 py-3 text-right text-lg font-bold text-white">${team.score}</td>
                </tr>`
              )
              .join("")
        : `<tr><td class="px-4 py-6 text-center text-slate-300" colspan="3">Scores are not ready yet.</td></tr>`;

    table.innerHTML = `<div class="p-4 md:p-6 space-y-4">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
            <div>
                <p class="text-xs uppercase tracking-[0.4em] text-slate-400">${selected.isPrivate ? "Private" : "Public"} event</p>
                <h2 class="text-2xl font-semibold text-white">${escapeHTML(selected.name)}</h2>
                <p class="text-sm text-slate-300">${escapeHTML(selected.description || "No description provided.")}</p>
            </div>
            <div class="text-sm text-slate-300 text-right">
                <p>${selected.teamCount} teams · ${selected.containerCount} containers</p>
                <p>Host: ${escapeHTML(selected.host || "Unknown")}</p>
            </div>
        </div>
        <div class="overflow-x-auto">
            <table class="min-w-full text-left">
                <thead>
                    <tr class="text-xs uppercase tracking-[0.3em] text-slate-400">
                        <th class="px-4 py-2">Rank</th>
                        <th class="px-4 py-2">Team</th>
                        <th class="px-4 py-2 text-right">Score</th>
                    </tr>
                </thead>
                <tbody>
                    ${rows}
                </tbody>
            </table>
        </div>
    </div>`;
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
}
