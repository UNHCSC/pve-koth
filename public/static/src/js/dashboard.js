import { escapeHTML } from "./shared/utils.js";

const list = document.getElementById("comps");
const emptyState = document.getElementById("empty");
const statContainer = document.getElementById("dashboard-stats");
const refreshButton = document.getElementById("refresh-dashboard");

function setupCreateCompetitionMenu() {
    const modal = document.getElementById("create-modal");
    const openButton = document.getElementById("open-create");
    const closeButton = document.getElementById("close-create");
    const cancelButton = document.getElementById("cancel-create");
    const overlay = document.getElementById("create-overlay");
    const form = document.getElementById("create-comp");
    const privateSelect = document.getElementById("comp-private");
    const groupsDiv = document.getElementById("ldap-groups-div");
    const groupsInput = document.getElementById("comp-groups");

    if (!modal || !openButton) {
        return;
    }

    function openModal() {
        modal.classList.remove("hidden");
    }

    function closeModal() {
        modal.classList.add("hidden");
    }

    openButton.addEventListener("click", openModal);
    closeButton?.addEventListener("click", closeModal);
    cancelButton?.addEventListener("click", closeModal);
    overlay?.addEventListener("click", closeModal);

    privateSelect?.addEventListener("change", (event) => {
        const isPrivate = event.target.value === "1";
        groupsDiv?.classList.toggle("hidden", !isPrivate);
        if (!isPrivate && groupsInput) {
            groupsInput.value = "";
        }
        if (groupsInput) {
            groupsInput.required = isPrivate;
        }
    });

    form?.addEventListener("submit", (event) => {
        event.preventDefault();
        const formData = new FormData(form);
        console.info("Competition payload ready", Object.fromEntries(formData.entries()));
        closeModal();
    });
}

function setStats({ total = 0, publicCount = 0, privateCount = 0 } = {}) {
    if (!statContainer) return;
    statContainer.querySelector('[data-stat="total"]').textContent = total;
    statContainer.querySelector('[data-stat="public"]').textContent = publicCount;
    statContainer.querySelector('[data-stat="private"]').textContent = privateCount;
}

function renderCompetitions(competitions = []) {
    if (!list) return;

    const publicCount = competitions.filter((c) => !c.isPrivate).length;
    const privateCount = competitions.length - publicCount;
    setStats({ total: competitions.length, publicCount, privateCount });

    if (!competitions.length) {
        emptyState?.classList.remove("hidden");
        list.innerHTML = "";
        return;
    }

    emptyState?.classList.add("hidden");
    list.innerHTML = competitions
        .map((comp) => {
            const badge = comp.isPrivate
                ? '<span class="ml-2 rounded-full bg-rose-500/20 text-rose-200 text-xs px-2 py-0.5">Private</span>'
                : '<span class="ml-2 rounded-full bg-emerald-500/20 text-emerald-200 text-xs px-2 py-0.5">Public</span>';

            return `<li class="rounded-2xl border border-white/10 bg-white/5 p-5 flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                <div>
                    <p class="text-lg font-semibold text-white">${escapeHTML(comp.name)}${badge}</p>
                    <p class="text-sm text-slate-300">${escapeHTML(comp.description || "No description")}</p>
                    <p class="text-xs text-slate-400 mt-1">Hosted by ${escapeHTML(comp.host || "Unknown")}</p>
                </div>
                <div class="text-sm text-right text-slate-300">
                    <p>${comp.teamCount} teams · ${comp.containerCount} containers</p>
                    <a class="text-blue-300 hover:text-blue-200" href="/scoreboard/${encodeURIComponent(
                        comp.competitionID
                    )}">Open scoreboard</a>
                </div>
            </li>`;
        })
        .join("");
}

async function loadDashboard() {
    try {
        const response = await fetch("/api/competitions", { credentials: "include" });
        if (!response.ok) throw new Error("Failed to load competitions");
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

refreshButton?.addEventListener("click", loadDashboard);
setupCreateCompetitionMenu();
loadDashboard();
