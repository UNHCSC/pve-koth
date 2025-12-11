import { escapeHTML } from "../shared/utils.js";

const dateFormatter = new Intl.DateTimeFormat(undefined, {
    dateStyle: "medium",
    timeStyle: "short"
});

function computeTeamCheckSummary(team) {
    const containers = Array.isArray(team?.containers) ? team.containers : [];
    let total = 0;
    let passed = 0;
    containers.forEach(function(container) {
        (container.checks || []).forEach(function(check) {
            total += 1;
            if (check?.passed) {
                passed += 1;
            }
        });
    });

    const ratio = total > 0 ? passed / total : 0;
    let statusClasses = "bg-slate-600/40";
    let textColor = "#e2e8f0";
    if (total === 0) {
        statusClasses = "bg-slate-600/40";
        textColor = "#e2e8f0";
    } else if (ratio >= 0.85) {
        statusClasses = "bg-emerald-500/30";
        textColor = "#dcfce7";
    } else if (ratio >= 0.6) {
        statusClasses = "bg-yellow-500/30";
        textColor = "#78350f";
    } else if (ratio >= 0.3) {
        statusClasses = "bg-orange-500/30";
        textColor = "#ffedd5";
    } else {
        statusClasses = "bg-rose-600/40";
        textColor = "#ffe4e6";
    }

    return { passed, total, ratio, statusClasses, textColor };
}

function formatDate(value) {
    if (!value) {
        return "—";
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
        return "—";
    }
    return dateFormatter.format(date);
}

function renderRankingRow(team, index) {
    const rowClass =
        index === 0 ? "bg-white/10" : index % 2 === 0 ? "bg-white/[0.04]" : "bg-transparent";
    const summary = computeTeamCheckSummary(team);
    const teamKey = team.id ?? index;
    const scoreValue = Number.isFinite(Number(team.score)) ? Number(team.score) : 0;
    const networkLabel = team.networkCIDR ? escapeHTML(team.networkCIDR) : "—";
    return `<tr class="${rowClass} score-row">
        <td class="px-2 py-1.5 text-xs font-semibold text-slate-200">#${index + 1}</td>
        <td class="px-2 py-1.5">
            <p class="text-white text-sm font-semibold">${escapeHTML(team.name)}</p>
            <p class="text-[0.65rem] text-slate-400">Updated ${formatDate(team.lastUpdated)}</p>
        </td>
        <td class="px-2 py-1.5 text-xs text-slate-300">${networkLabel}</td>
        <td class="px-2 py-1.5 text-right text-base font-bold text-white">
            <span class="score-value" data-team-id="team-${teamKey}" data-target="${scoreValue}">${scoreValue}</span>
        </td>
        <td class="px-2 py-1.5 text-right">
                <span class="status-pill matrix-status inline-flex items-center rounded-lg px-2 py-0.5 text-[0.65rem] font-semibold ${summary.statusClasses}" style="color: ${summary.textColor};">
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

    function normalizeName(value) {
        return (value || "Container").toLowerCase();
    }

    function ensureContainer(name, sourceChecks = []) {
        const normalized = normalizeName(name);
        if (!containerIndex.has(normalized)) {
            containerIndex.set(normalized, containers.length);
            containers.push({
                name: name || "Container",
                normalized,
                columns: sourceChecks.map(function(check) {
                    return {
                        id: check?.id || check?.name || "",
                        name: check?.name || check?.id || "Service"
                    };
                }),
                rows: []
            });
        } else if (sourceChecks.length) {
            const entry = containers[containerIndex.get(normalized)];
            sourceChecks.forEach(function(check) {
                const id = check?.id || check?.name || "";
                if (!id) {
                    return;
                }
                if (!entry.columns.some(function(column) {
                    return column.id === id;
                })) {
                    entry.columns.push({
                        id,
                        name: check?.name || check?.id || "Service"
                    });
                }
            });
        }
    }

    selected.teams.forEach(function(team) {
        (team.containers || []).forEach(function(container) {
            ensureContainer(container?.name, Array.isArray(container?.checks) ? container.checks : []);
        });
    });

    if (!containers.length) {
        return [];
    }

    const sortedTeams = [...selected.teams].sort(function(a, b) {
        const nameA = (a?.name || "").toLowerCase();
        const nameB = (b?.name || "").toLowerCase();
        if (nameA === nameB) {
            return 0;
        }
        return nameA < nameB ? -1 : 1;
    });

    sortedTeams.forEach(function(team, index) {
        containers.forEach(function(containerEntry) {
            const teamContainer = (team.containers || []).find(function(container) {
                return normalizeName(container?.name) === containerEntry.normalized;
            });
            const statusMap = new Map();
            (teamContainer?.checks || []).forEach(function(check) {
                const id = check?.id || check?.name || "";
                if (!id) {
                    return;
                }
                statusMap.set(id, Boolean(check?.passed));
            });

            containerEntry.rows.push({
                team: team.name || `Team ${index + 1}`,
                statuses: containerEntry.columns.map(function(column) {
                    return statusMap.get(column.id);
                })
            });
        });
    });

    return containers;
}

function renderStatusCell(status) {
    let classes = "bg-white/5 text-slate-200 border-white/15";
    let label = "—";
    if (status === true) {
        classes = "bg-emerald-500/20 text-emerald-100 border-emerald-400/30";
        label = "Up";
    } else if (status === false) {
        classes = "bg-rose-500/25 text-rose-100 border-rose-400/30";
        label = "Down";
    }
    return `<span class="matrix-status inline-flex w-full items-center justify-center rounded-xl border px-3 py-1 text-[0.7rem] font-semibold leading-tight ${classes}">${label}</span>`;
}

function renderMatrixTable(matrix, index = 0) {
    const checks = matrix.columns || [];
    const teams = matrix.rows || [];
    const hasData = checks.length > 0 && teams.length > 0;

    if (!hasData) {
        return `<div class="space-y-1 border-t border-white/5 pt-3">
            <p class="text-sm font-semibold text-white">${escapeHTML(matrix.name)}</p>
            <p class="text-xs text-slate-400">No services or teams reported yet.</p>
        </div>`;
    }

    const gridTemplate = `grid-template-columns: minmax(6rem, 1fr) repeat(${teams.length}, minmax(5rem, 1fr));`;

    const headerRow = `<div class="grid gap-1 text-[0.65rem] uppercase tracking-[0.2em] text-slate-300 pb-1 border-b border-white/10 mb-1" style="${gridTemplate}">
        <span class="text-left">Check</span>
        ${teams
            .map(function(team) {
                return `<span class="text-center whitespace-nowrap text-slate-200 font-semibold">${escapeHTML(team.team)}</span>`;
            })
            .join("")}
    </div>`;

    const body = checks
        .map(function(check, checkIndex) {
            const cells = teams
                .map(function(team) {
                    return renderStatusCell(team.statuses?.[checkIndex]);
                })
                .join("");
            return `<div class="grid items-center gap-1 text-[0.75rem]" style="${gridTemplate}">
                <span class="truncate font-semibold text-white/90">${escapeHTML(check.name)}</span>
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

function renderGridMarkup(selected) {
    if (!selected || !selected.teams.length) {
        return "";
    }

    const matrices = buildContainerMatrices(selected);
    if (!matrices.length) {
        return "";
    }

    return matrices.map(function(matrix, index) {
        return renderMatrixTable(matrix, index);
    }).join("");
}

export function buildTabsMarkup(competitions = [], selected = "") {
    return competitions
        .map(function(comp) {
            const active = selected === comp.competitionID;
            return `<button data-id="${comp.competitionID}" class="shrink-0 px-4 py-2 rounded-2xl text-sm font-semibold border ${
                active
                    ? "bg-blue-500 text-white border-blue-400"
                    : "bg-white/5 text-white/90 border-white/20 hover:bg-white/10"
            }">${escapeHTML(comp.name)}</button>`;
        })
        .join("");
}

export function buildTableMarkup(selected, canManage) {
    if (!selected) {
        return "";
    }

    const rows = selected.teams.length
        ? selected.teams.map(function(team, index) {
              return renderRankingRow(team, index);
          }).join("")
        : `<tr><td class="px-3 py-4 text-center text-slate-300" colspan="5">Scores are not ready yet.</td></tr>`;

    const scoringBadge = selected.scoringActive
        ? "<span class=\"inline-flex items-center rounded-full border border-emerald-400/40 bg-emerald-500/10 px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.2em] text-emerald-200\">Scoring active</span>"
        : "<span class=\"inline-flex items-center rounded-full border border-amber-400/40 bg-amber-500/10 px-2 py-0.5 text-[0.65rem] font-semibold uppercase tracking-[0.2em] text-amber-200\">Scoring paused</span>";

    const scoringNotice = selected.scoringActive
        ? ""
        : "<p class=\"text-[0.7rem] text-amber-200 bg-amber-500/10 border border-amber-500/20 rounded-2xl px-3 py-1\">Scoring is currently paused for this competition. Results will not update until scoring resumes.</p>";

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

    return `<div class="p-3 md:p-4 space-y-3 text-sm">
        <div class="flex flex-col md:flex-row md:items-center md:justify-between gap-3">
            <div>
                <p class="text-xs uppercase tracking-[0.4em] text-slate-400">${selected.isPrivate ? "Private" : "Public"} event</p>
                <h2 class="text-xl font-semibold text-white">${escapeHTML(selected.name)}</h2>
                <p class="text-xs text-slate-300">${escapeHTML(selected.description || "No description provided.")}</p>
                <p class="text-xs text-slate-300">Network: ${escapeHTML(selected.networkCIDR || "Not assigned")}</p>
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
                        <th class="px-2 py-1.5">Network</th>
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
            ${renderGridMarkup(selected)}
        </div>
    </div>`;
}
