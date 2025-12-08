import { escapeHTML } from "./shared/utils.js";

const list = document.getElementById("comps");
const emptyState = document.getElementById("empty");
const statContainer = document.getElementById("dashboard-stats");
const refreshButton = document.getElementById("refresh-dashboard");
const canManage = list?.dataset.canManage === "true";

function formatBytes(bytes = 0) {
    if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
    const units = ["B", "KB", "MB", "GB"];
    const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    const converted = bytes / 1024 ** index;
    return `${converted.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

async function validateZipFile(file) {
    if (!file) {
        return { ok: false, message: "Select a competition package" };
    }

    if (!/\.zip$/i.test(file.name)) {
        return { ok: false, message: "Package must be a .zip file" };
    }

    if (file.size > 75 * 1024 * 1024) {
        return { ok: false, message: "Package exceeds the 75 MB limit" };
    }

    try {
        const header = new Uint8Array(await file.slice(0, 4).arrayBuffer());
        if (header[0] !== 0x50 || header[1] !== 0x4b) {
            return { ok: false, message: "File is missing a ZIP signature" };
        }
    } catch (error) {
        return { ok: false, message: "Unable to inspect the package" };
    }

    return { ok: true, message: "Package looks like a valid zip" };
}

function setupCreateCompetitionMenu() {
    const modal = document.getElementById("create-modal");
    const openButton = document.getElementById("open-create");
    const closeButton = document.getElementById("close-create");
    const cancelButton = document.getElementById("cancel-create");
    const overlay = document.getElementById("create-overlay");
    const form = document.getElementById("create-comp");
    const browseButton = document.getElementById("package-browse");
    const fileInput = document.getElementById("package-file");
    const dropZone = document.getElementById("package-dropzone");
    const summary = document.getElementById("package-summary");
    const summaryFields = summary
        ? {
              name: summary.querySelector('[data-field="name"]'),
              size: summary.querySelector('[data-field="size"]'),
              status: summary.querySelector('[data-field="status"]')
          }
        : {};
    const logElement = document.getElementById("create-log");
    const uploadButton = document.getElementById("upload-package");
    const state = {
        file: null,
        submitting: false,
        eventSource: null
    };

    if (!modal || !openButton) {
        return;
    }

    function closeStream() {
        if (state.eventSource) {
            state.eventSource.close();
            state.eventSource = null;
        }
    }

    function resetState() {
        state.file = null;
        closeStream();
        if (fileInput) {
            fileInput.value = "";
        }
        if (summary) {
            summary.classList.add("hidden");
        }
        if (logElement) {
            logElement.textContent = "";
            logElement.classList.add("hidden");
        }
    }

    function openModal() {
        modal.classList.remove("hidden");
    }

    function closeModal() {
        modal.classList.add("hidden");
        resetState();
    }

    function appendLog(message) {
        if (!logElement) return;
        logElement.classList.remove("hidden");
        const timestamp = new Date().toLocaleTimeString();
        logElement.textContent += `[${timestamp}] ${message}\n`;
        logElement.scrollTop = logElement.scrollHeight;
    }

    function appendServerLogs(logs = []) {
        if (!Array.isArray(logs) || logs.length === 0) return;
        logs.forEach((entry, index) => {
            setTimeout(() => appendLog(`[server] ${entry}`), index * 150);
        });
    }

    function startLogStream(jobID) {
        if (!jobID || typeof EventSource === "undefined") {
            return;
        }

        closeStream();
        appendLog(`Connecting to provisioning log (${jobID})...`);

        const source = new EventSource(`/api/competitions/upload/${encodeURIComponent(jobID)}/stream`);
        state.eventSource = source;

        source.onmessage = (event) => {
            const message = event.data || "";
            appendLog(`[provisioning] ${message}`);
            handleProvisioningStatus(message);
        };

        source.onerror = () => {
            appendLog("Log stream disconnected.");
            closeStream();
        };
    }

    function handleProvisioningStatus(message = "") {
        const lower = message.toLowerCase();
        const progressMatch = message.match(/^(\S)\s+Provisioning progress:\s*\((\d+)\/(\d+)\)/i);
        if (progressMatch) {
            const spinner = progressMatch[1];
            const done = Number(progressMatch[2]);
            const total = Number(progressMatch[3]);
            updateSummary(`${spinner} Provisioning containers (${done}/${total})`, "text-amber-300");
            if (total > 0 && done >= total) {
                loadDashboard();
            }
            return;
        }
        if (lower.includes("creating new competition") || lower.includes("creating competition record")) {
            updateSummary("Creating competition records...", "text-blue-400");
            return;
        }
        if (lower.includes("generating ssh keypair")) {
            updateSummary("Generating SSH credentials...", "text-blue-400");
            return;
        }
        if (lower.includes("creating container templates") || lower.includes("provisioning container")) {
            updateSummary("Provisioning containers...", "text-amber-400");
            return;
        }
        if (lower.includes("successfully created competition") || lower.includes("provisioning completed successfully")) {
            updateSummary("Provisioning complete", "text-emerald-500");
            loadDashboard();
            return;
        }
        if (lower.includes("error") || lower.includes("failed")) {
            updateSummary("Provisioning encountered errors", "text-rose-500");
        }
    }

    function updateSummary(statusText = "Waiting for upload", statusClass = "text-slate-300") {
        if (!summary || !summaryFields.status) return;
        summary.classList.toggle("hidden", !state.file);
        if (state.file) {
            if (summaryFields.name) {
                summaryFields.name.textContent = state.file.name;
            }
            if (summaryFields.size) {
                summaryFields.size.textContent = formatBytes(state.file.size);
            }
        }
        summaryFields.status.textContent = statusText;
        summaryFields.status.className = `font-semibold ${statusClass}`;
    }

    function setSubmitting(isSubmitting) {
        state.submitting = isSubmitting;
        if (uploadButton) {
            uploadButton.disabled = isSubmitting;
            uploadButton.classList.toggle("opacity-60", isSubmitting);
        }
    }

    openButton.addEventListener("click", openModal);
    closeButton?.addEventListener("click", closeModal);
    cancelButton?.addEventListener("click", closeModal);
    overlay?.addEventListener("click", closeModal);

    browseButton?.addEventListener("click", () => fileInput?.click());

    const handleFileSelection = (file) => {
        if (!file) {
            return;
        }
        state.file = file;
        appendLog(`Selected package ${file.name} (${formatBytes(file.size)})`);
        updateSummary("Ready to validate", "text-blue-600");
    };

    fileInput?.addEventListener("change", (event) => {
        const [file] = event.target.files || [];
        handleFileSelection(file);
    });

    if (dropZone) {
        const setDropActive = (active) => {
            dropZone.classList.toggle("border-blue-400/60", active);
            dropZone.classList.toggle("bg-blue-500/20", active);
            dropZone.classList.toggle("text-white", active);
        };

        dropZone.addEventListener("dragover", (event) => {
            event.preventDefault();
            setDropActive(true);
        });

        dropZone.addEventListener("dragleave", (event) => {
            event.preventDefault();
            setDropActive(false);
        });

        dropZone.addEventListener("drop", (event) => {
            event.preventDefault();
            setDropActive(false);
            const [file] = event.dataTransfer?.files || [];
            if (fileInput && file) {
                const dataTransfer = new DataTransfer();
                dataTransfer.items.add(file);
                fileInput.files = dataTransfer.files;
            }
            handleFileSelection(file);
        });
    }

    form?.addEventListener("submit", async (event) => {
        event.preventDefault();
        if (!state.file || state.submitting) {
            appendLog("Select a package before uploading.");
            updateSummary("Waiting for upload", "text-rose-600");
            return;
        }

        setSubmitting(true);

        try {
            appendLog("Validating package locally...");
            const validation = await validateZipFile(state.file);
            updateSummary(validation.message, validation.ok ? "text-emerald-600" : "text-rose-600");
            if (!validation.ok) {
                appendLog(`Validation failed: ${validation.message}`);
                return;
            }

            appendLog("Uploading package to server...");
            const payload = new FormData();
            payload.append("file", state.file, state.file.name);

            const response = await fetch("/api/competitions/upload", {
                method: "POST",
                body: payload,
                credentials: "include"
            });

            const result = await response.json().catch(() => ({}));

            if (!response.ok) {
                appendServerLogs(result?.logs);
                const message = result?.error || result?.message || "Upload failed";
                updateSummary(message, "text-rose-600");
                appendLog(`Server error: ${message}`);
                if (result?.detail) {
                    appendLog(`Details: ${result.detail}`);
                }
                return;
            }

            const successMessage = result?.message || "Server parsed the competition package.";
            appendLog(successMessage);
            if (result?.competitionID) {
                appendLog(`Parsed ID: ${result.competitionID}`);
            }
            if (result?.competitionName) {
                appendLog(`Parsed name: ${result.competitionName}`);
            }
            if (typeof result?.packageID !== "undefined") {
                appendLog(`Stored package ID: ${result.packageID}`);
            }
            updateSummary("Provisioning will begin shortly...", "text-blue-400");
            if (result?.jobID) {
                startLogStream(result.jobID);
            } else if (Array.isArray(result?.logs)) {
                appendServerLogs(result.logs);
                loadDashboard();
            }
        } catch (error) {
            appendLog(`Error: ${error.message}`);
            updateSummary(error.message, "text-rose-600");
        } finally {
            setSubmitting(false);
        }
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
            const scoringBadge = comp.scoringActive
                ? '<span class="ml-2 rounded-full bg-emerald-500/20 text-emerald-200 text-xs px-2 py-0.5">Scoring active</span>'
                : '<span class="ml-2 rounded-full bg-amber-500/20 text-amber-100 text-xs px-2 py-0.5">Scoring paused</span>';

            const actions = `
                <div class="flex flex-col items-end gap-2 mt-2">
                    <a class="text-blue-300 hover:text-blue-200" href="/scoreboard/${encodeURIComponent(
                        comp.competitionID
                    )}">Open scoreboard</a>
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

            return `<li class="rounded-2xl border border-white/10 bg-white/5 p-5 flex flex-col gap-4 md:flex-row md:items-center md:justify-between">
                <div>
                    <p class="text-lg font-semibold text-white">${escapeHTML(comp.name)}${badge}${scoringBadge}</p>
                    <p class="text-sm text-slate-300">${escapeHTML(comp.description || "No description")}</p>
                    <p class="text-xs text-slate-400 mt-1">Hosted by ${escapeHTML(comp.host || "Unknown")}</p>
                </div>
                <div class="text-sm text-right text-slate-300">
                    <p>${comp.teamCount} teams · ${comp.containerCount} containers</p>
                    ${actions}
                </div>
            </li>`;
        })
        .join("");
}

async function teardownCompetition(button) {
    if (!button) return;
    const compID = button.dataset.id;
    const compName = button.dataset.name || compID;
    if (!compID) return;

    if (!window.confirm(`Tear down competition "${compName}"? This cannot be undone.`)) {
        return;
    }

    const originalText = button.textContent;
    button.disabled = true;
    button.textContent = "Destroying…";

    try {
        const response = await fetch(`/api/competitions/${encodeURIComponent(compID)}/teardown`, {
            method: "POST",
            credentials: "include"
        });
        const result = await response.json().catch(() => ({}));

        if (!response.ok) {
            throw new Error(result?.error || result?.message || "Failed to tear down competition");
        }

        window.alert(result?.message || `Competition ${compName} destroyed.`);
        await loadDashboard();
    } catch (error) {
        console.error(error);
        window.alert(error.message || "Unable to tear down competition.");
    } finally {
        button.disabled = false;
        button.textContent = originalText;
    }
}

async function toggleScoring(button) {
    if (!button) return;
    const compID = button.dataset.id;
    const currentlyActive = button.dataset.active === "true";
    if (!compID) return;

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

        const result = await response.json().catch(() => ({}));
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
if (canManage && list) {
    list.addEventListener("click", (event) => {
        const toggle = event.target.closest("[data-action='toggle-scoring']");
        if (toggle) {
            toggleScoring(toggle);
            return;
        }
        const target = event.target.closest("[data-action='teardown']");
        if (target) {
            teardownCompetition(target);
        }
    });
}
setupCreateCompetitionMenu();
loadDashboard();
