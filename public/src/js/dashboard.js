import { escapeHTML } from "./shared/utils.js";

const list = document.getElementById("comps");
const emptyState = document.getElementById("empty");
const statContainer = document.getElementById("dashboard-stats");
const refreshButton = document.getElementById("refresh-dashboard");
const canManage = list?.dataset.canManage === "true";
const containerStates = new Map();
const teamStates = new Map();
const redeployModal = document.getElementById("redeploy-modal");
const redeployStatus = redeployModal?.querySelector("[data-redeploy-status]");
const redeployTarget = redeployModal?.querySelector("[data-redeploy-target]");
const redeployLog = document.getElementById("redeploy-log");
const redeployOverlay = document.getElementById("redeploy-overlay");
const redeployCloseButton = document.getElementById("close-redeploy");
const redeployStartCheckbox = redeployModal?.querySelector("[data-redeploy-start-checkbox]");
const redeployConfirmButton = redeployModal?.querySelector("[data-redeploy-confirm]");
const redeployConfirmDefaultText =
    redeployConfirmButton?.dataset.defaultLabel?.trim() || redeployConfirmButton?.textContent?.trim() || "Redeploy";
let redeployEventSource = null;
let redeployStreamCompID = "";

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

function updateRedeployStatus(text = "Waiting to start", tone = "text-slate-300") {
    if (!redeployStatus) return;
    redeployStatus.textContent = text;
    redeployStatus.className = `text-sm font-semibold ${tone}`;
}

function setRedeployTargetLabel(text = "Awaiting selection") {
    if (!redeployTarget) return;
    redeployTarget.textContent = text;
}

function resetRedeployModalState() {
    if (redeployLog) {
        redeployLog.textContent = "";
        redeployLog.classList.add("hidden");
    }
    updateRedeployStatus();
    closeRedeployStream();
    if (redeployConfirmButton) {
        redeployConfirmButton.disabled = false;
        redeployConfirmButton.textContent = redeployConfirmDefaultText;
    }
    if (redeployStartCheckbox) {
        redeployStartCheckbox.checked = false;
        redeployStartCheckbox.disabled = false;
    }
    if (redeployModal) {
        delete redeployModal.dataset.containerId;
        delete redeployModal.dataset.containerLabel;
        delete redeployModal.dataset.compId;
    }
}

function appendRedeployLog(message) {
    if (!redeployLog) return;
    const timestamp = new Date().toLocaleTimeString();
    redeployLog.classList.remove("hidden");
    redeployLog.textContent += `[${timestamp}] ${message}\n`;
    redeployLog.scrollTop = redeployLog.scrollHeight;
}

function closeRedeployStream() {
    if (!redeployEventSource) return;
    redeployEventSource.close();
    redeployEventSource = null;
    redeployStreamCompID = "";
}

function handleRedeployStatus(message = "") {
    const lower = message.toLowerCase();
    if (!message) return;
    if (lower.includes("redeploy completed")) {
        updateRedeployStatus("Redeploy complete", "text-emerald-400");
        if (redeployStreamCompID) {
            loadCompetitionContainers(redeployStreamCompID);
            redeployStreamCompID = "";
        }
        return;
    }
    if (lower.includes("redeploy failed") || lower.includes("error:")) {
        updateRedeployStatus("Redeploy failed", "text-rose-500");
        return;
    }
    if (lower.includes("redeploy job started")) {
        updateRedeployStatus("Redeploy in progress...", "text-amber-400");
    }
}

function startRedeployLogStream(jobID, compID = "") {
    if (!jobID || typeof EventSource === "undefined") {
        return;
    }

    closeRedeployStream();
    redeployStreamCompID = compID;
    appendRedeployLog(`Connecting to redeploy log (${jobID})...`);

    const source = new EventSource(`/api/containers/redeploy/${encodeURIComponent(jobID)}/stream`);
    redeployEventSource = source;

    source.onmessage = (event) => {
        const message = event.data || "";
        appendRedeployLog(`[redeploy] ${message}`);
        handleRedeployStatus(message);
    };

    source.onerror = () => {
        appendRedeployLog("Log stream disconnected.");
        closeRedeployStream();
    };
}

function openRedeployModal(label, id, compID = "") {
	if (!redeployModal) return;
	resetRedeployModalState();
	redeployModal.dataset.containerId = String(id);
	redeployModal.dataset.containerLabel = label;
	if (compID) {
		redeployModal.dataset.compId = compID;
	}
	setRedeployTargetLabel(`${label} (#${id})`);
	redeployModal.classList.remove("hidden");
}

async function handleRedeployConfirm() {
	if (!redeployModal || !redeployConfirmButton) return;
	const id = Number(redeployModal.dataset.containerId);
	if (!Number.isFinite(id)) return;
	const compID = redeployModal.dataset.compId || "";
	const containerLabel = redeployModal.dataset.containerLabel || `CT-${id}`;
	const startAfter = Boolean(redeployStartCheckbox?.checked);

	redeployConfirmButton.disabled = true;
	redeployConfirmButton.textContent = "Redeploying…";
	if (redeployStartCheckbox) {
		redeployStartCheckbox.disabled = true;
	}

	appendRedeployLog(`Queued redeploy for ${containerLabel}.`);
	updateRedeployStatus("Redeploy in progress...", "text-amber-400");

	try {
		const response = await fetch("/api/containers/redeploy", {
			method: "POST",
			credentials: "include",
			headers: { "Content-Type": "application/json" },
			body: JSON.stringify({ ids: [id], startAfter })
		});
		const payload = await response.json().catch(() => ({}));
		if (!response.ok) {
			throw new Error(payload?.error || payload?.message || "Failed to redeploy container");
		}
		const successMessage = payload?.message || "Container redeployed.";
		appendRedeployLog(successMessage);
		if (payload?.jobID) {
			startRedeployLogStream(payload.jobID, compID);
		} else {
			updateRedeployStatus("Redeploy in progress...", "text-amber-400");
		}
	} catch (error) {
		const message = error.message || "Unable to redeploy container.";
		appendRedeployLog(message);
		closeRedeployStream();
		updateRedeployStatus("Redeploy failed", "text-rose-500");
		window.alert(message);
	} finally {
		redeployConfirmButton.disabled = false;
		redeployConfirmButton.textContent = redeployConfirmDefaultText;
	}
}

function closeRedeployModal() {
	if (!redeployModal) return;
	redeployModal.classList.add("hidden");
	setRedeployTargetLabel("Awaiting selection");
    updateRedeployStatus();
    closeRedeployStream();
}

redeployOverlay?.addEventListener("click", closeRedeployModal);
redeployCloseButton?.addEventListener("click", closeRedeployModal);
redeployConfirmButton?.addEventListener("click", handleRedeployConfirm);

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
            const networkLabel = comp.networkCIDR ? escapeHTML(comp.networkCIDR) : "Not assigned";

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
				${canManage ? `${renderCompetitionContainerPanel(comp)}${renderCompetitionTeamPanel(comp)}` : ""}
			</li>`;
		})
		.join("");

	if (canManage) {
		const activeIDs = new Set(competitions.map((comp) => String(comp?.competitionID || "")));
		for (const key of Array.from(containerStates.keys())) {
			if (!activeIDs.has(key)) {
				containerStates.delete(key);
			}
		}
		for (const key of Array.from(teamStates.keys())) {
			if (!activeIDs.has(key)) {
				teamStates.delete(key);
			}
		}
		competitions.forEach((comp) => {
			if (!comp || !comp.competitionID) return;
			const containerState = initCompetitionContainerState(comp.competitionID);
			renderCompetitionContainers(comp.competitionID);
			if (!containerState.loaded && !containerState.loading) {
				loadCompetitionContainers(comp.competitionID);
			}

			const teamState = initCompetitionTeamState(comp.competitionID);
			renderCompetitionTeams(comp.competitionID);
			if (!teamState.loaded && !teamState.loading) {
				loadCompetitionTeams(comp.competitionID);
			}
		});
	}
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
	if (!list) return null;
	const encoded = encodeURIComponent(String(compID || ""));
	return list.querySelector(`[data-container-panel="${encoded}"]`);
}

function describePowerStatus(status = "") {
	const normalized = String(status || "").toLowerCase();
	switch (normalized) {
		case "running":
			return { label: "Running", tone: "running" };
		case "stopped":
			return { label: "Stopped", tone: "stopped" };
		case "services-down":
			return { label: "Services down", tone: "degraded" };
		default:
			return { label: normalized ? normalized : "Unknown", tone: "unknown" };
	}
}

function formatRelativeTime(timestamp) {
	if (!timestamp) return "—";
	const parsed = new Date(timestamp);
	if (Number.isNaN(parsed.getTime())) {
		return "—";
	}

	const diff = Date.now() - parsed.getTime();
	if (diff < 30 * 1000) return "just now";
	const minutes = Math.floor(diff / (60 * 1000));
	if (minutes < 60) return `${minutes}m ago`;
	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours}h ago`;
	const days = Math.floor(hours / 24);
	return `${days}d ago`;
}

function renderCompetitionContainers(compID) {
	const panel = getContainerPanel(compID);
	if (!panel) return;
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
	if (startBtn) startBtn.disabled = disableActions;
	if (stopBtn) stopBtn.disabled = disableActions;
	if (refreshBtn) refreshBtn.disabled = state.loading;

	if (selectAll) {
		selectAll.disabled = state.loading || !state.loaded || state.data.length === 0;
		selectAll.checked = state.data.length > 0 && selectedCount === state.data.length;
		selectAll.indeterminate = state.data.length > 0 && selectedCount > 0 && selectedCount < state.data.length;
	}

	if (!body) return;

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
			.map((entry) => {
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
	if (!list) return null;
	const encoded = encodeURIComponent(String(compID || ""));
	return list.querySelector(`[data-team-panel="${encoded}"]`);
}

function renderCompetitionTeams(compID) {
	const panel = getTeamPanel(compID);
	if (!panel) return;
	const state = initCompetitionTeamState(compID);
	const selection = panel.querySelector("[data-team-selection]");
	const selectAll = panel.querySelector("[data-team-select-all]");
	const body = panel.querySelector("[data-team-body]");
	const resetBtn = panel.querySelector('[data-team-action="reset"]');
	const adjustBtn = panel.querySelector('[data-team-action="adjust"]');
	const adjustInput = panel.querySelector("[data-team-adjust-value]");
	const refreshBtn = panel.querySelector('[data-team-panel-action="refresh"]');
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
	if (resetBtn) resetBtn.disabled = actionDisabled;
	if (adjustBtn) adjustBtn.disabled = actionDisabled;
	if (adjustInput) adjustInput.disabled = actionDisabled;
	if (refreshBtn) refreshBtn.disabled = state.loading;

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

	if (!body) return;

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
		.map((team) => {
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
	if (!state || state.loading) return;
	state.loading = true;
	state.error = "";
	renderCompetitionTeams(compID);

	try {
		const response = await fetch(`/api/competitions/${encodeURIComponent(compID)}/teams`, {
			credentials: "include"
		});
		const payload = await response.json().catch(() => ({}));
		if (!response.ok) {
			throw new Error(payload?.error || payload?.message || "Failed to load teams");
		}

		const teams = Array.isArray(payload?.teams) ? payload.teams : [];
		const normalized = [];
		for (const entry of teams) {
			if (!entry || !Number.isFinite(Number(entry.id))) {
				continue;
			}
			normalized.push({
				id: Number(entry.id),
				name: entry.name || `Team ${entry.id}`,
				score: Number.isFinite(Number(entry.score)) ? Number(entry.score) : 0,
				lastUpdated: entry.lastUpdated || "",
				network: entry.networkCIDR || ""
			});
		}

		state.teams = normalized;
		const availableIDs = new Set(normalized.map((team) => team.id));
		state.selected = new Set(Array.from(state.selected).filter((id) => availableIDs.has(id)));
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
	if (!panel) return;
	const compID = panel.dataset.compId || "";
	if (!compID) return;
	const state = initCompetitionTeamState(compID);
	if (!state) return;
	const id = Number(checkbox.value);
	if (!Number.isFinite(id)) return;
	if (checkbox.checked) {
		state.selected.add(id);
	} else {
		state.selected.delete(id);
	}
	renderCompetitionTeams(compID);
}

function handleTeamSelectAll(checkbox) {
	const panel = checkbox.closest("[data-team-panel]");
	if (!panel) return;
	const compID = panel.dataset.compId || "";
	if (!compID) return;
	const state = initCompetitionTeamState(compID);
	if (!state) return;
	if (checkbox.checked) {
		state.teams.forEach((team) => state.selected.add(team.id));
	} else {
		state.selected.clear();
	}
	renderCompetitionTeams(compID);
}

async function handleTeamAction(button) {
	const panel = button.closest("[data-team-panel]");
	if (!panel) return;
	const compID = panel.dataset.compId || "";
	if (!compID) return;
	const state = initCompetitionTeamState(compID);
	if (!state) return;

	const selectedIDs = Array.from(state.selected);
	if (!selectedIDs.length) {
		state.feedback = { text: "Select at least one team.", tone: "text-rose-400" };
		renderCompetitionTeams(compID);
		return;
	}

	const action = button.dataset.teamAction;
	if (!action) return;

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
			const result = await response.json().catch(() => ({}));
			if (!response.ok) {
				throw new Error(result?.error || result?.message || "Failed to update team score");
			}

			const team = state.teams.find((entry) => entry.id === teamID);
			if (team) {
				if (Number.isFinite(Number(result?.score))) {
					team.score = Number(result.score);
				}
				team.lastUpdated = result?.lastUpdated || new Date().toISOString();
			}
		}

		state.feedback = {
			text: `${action === "reset" ? "Scores reset" : "Scores updated"} for ${selectedIDs.length} team${
				selectedIDs.length === 1 ? "" : "s"
			}.`,
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

function handleListToggle(event) {
	const toggleTarget = event.target.closest("[data-team-panel]");
	if (!toggleTarget || !toggleTarget.open) return;
	const compID = toggleTarget.dataset.compId || "";
	if (!compID) return;
	const state = initCompetitionTeamState(compID);
	if (!state.loaded && !state.loading) {
		loadCompetitionTeams(compID);
	}
}

async function loadCompetitionContainers(compID) {
	const state = initCompetitionContainerState(compID);
	state.loading = true;
	state.error = "";
	renderCompetitionContainers(compID);

	try {
		const response = await fetch(`/api/containers?competition=${encodeURIComponent(compID)}`, { credentials: "include" });
		const payload = await response.json().catch(() => ({}));
		if (!response.ok) {
			throw new Error(payload?.error || payload?.message || "Failed to load containers");
		}
		const containers = Array.isArray(payload?.containers) ? payload.containers : [];
		state.data = containers;
		state.loaded = true;
		state.selected = new Set(
			Array.from(state.selected).filter((id) => containers.some((entry) => Number(entry?.id) === Number(id)))
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
		const payload = await response.json().catch(() => ({}));
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
	if (!panel) return;
	const compID = panel.dataset.compId || "";
	const id = Number(checkbox.value);
	if (!Number.isFinite(id)) return;
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
	if (!panel) return;
	const compID = panel.dataset.compId || "";
	const state = initCompetitionContainerState(compID);
	state.selected.clear();
	if (checkbox.checked) {
		state.data.forEach((entry) => {
			const id = Number(entry?.id);
			if (Number.isFinite(id)) {
				state.selected.add(id);
			}
		});
	}
	renderCompetitionContainers(compID);
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

function handleContainerRedeploy(button) {
	const panel = button.closest("[data-container-panel]");
	if (!panel) return;
	const compID = panel.dataset.compId || "";
	const id = Number(button.dataset.containerId);
	if (!Number.isFinite(id)) return;

	const containerLabel = button.dataset.containerLabel || `CT-${id}`;
	if (!redeployModal) return;
	openRedeployModal(containerLabel, id, compID);
}

function handleListClick(event) {
	if (!(event.target instanceof Element)) return;
	const teamPanelControl = event.target.closest("[data-team-panel-action]");
	if (teamPanelControl) {
		const panel = teamPanelControl.closest("[data-team-panel]");
		const compID = panel?.dataset.compId || "";
		const action = teamPanelControl.dataset.teamPanelAction;
		if (action === "refresh" && compID) {
			void loadCompetitionTeams(compID);
		}
		return;
	}
	const teamAction = event.target.closest("[data-team-action]");
	if (teamAction && teamAction.dataset.teamAction) {
		void handleTeamAction(teamAction);
		return;
	}
	const redeployButton = event.target.closest("[data-container-redeploy]");
	if (redeployButton) {
		handleContainerRedeploy(redeployButton);
		return;
	}
	const containerButton = event.target.closest("[data-container-action]");
	if (containerButton) {
		const panel = containerButton.closest("[data-container-panel]");
		if (!panel) return;
		const compID = panel.dataset.compId || "";
		const action = containerButton.dataset.containerAction;
		if (action === "refresh") {
			loadCompetitionContainers(compID);
			return;
		}
		if (action === "start" || action === "stop") {
			handleCompetitionBulkPower(compID, action);
			return;
		}
	}
	const toggle = event.target.closest("[data-action='toggle-scoring']");
	if (toggle) {
		toggleScoring(toggle);
		return;
	}
	const teardownTarget = event.target.closest("[data-action='teardown']");
	if (teardownTarget) {
		teardownCompetition(teardownTarget);
	}
}

function handleListChange(event) {
	if (!(event.target instanceof Element)) return;
	const teamSelect = event.target.closest("[data-team-select]");
	if (teamSelect) {
		handleTeamRowSelection(teamSelect);
		return;
	}
	const teamSelectAll = event.target.closest("[data-team-select-all]");
	if (teamSelectAll) {
		handleTeamSelectAll(teamSelectAll);
		return;
	}
	const rowCheckbox = event.target.closest("[data-container-select]");
	if (rowCheckbox) {
		handleContainerRowSelection(rowCheckbox);
		return;
	}
	const selectAll = event.target.closest("[data-container-select-all]");
	if (selectAll) {
		handleContainerSelectAll(selectAll);
	}
}

refreshButton?.addEventListener("click", loadDashboard);

if (canManage && list) {
	list.addEventListener("click", handleListClick);
	list.addEventListener("change", handleListChange);
	list.addEventListener("toggle", handleListToggle);
}

setupCreateCompetitionMenu();
loadDashboard();
