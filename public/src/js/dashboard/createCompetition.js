import { formatBytes } from "./helpers.js";

export async function validateZipFile(file) {
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

export function setupCreateCompetitionMenu({ loadDashboard } = {}) {
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
    const advancedLoggingCheckbox = document.getElementById("enable-advanced-logging");
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

        if (advancedLoggingCheckbox) {
            advancedLoggingCheckbox.checked = false;
            advancedLoggingCheckbox.disabled = false;
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
        if (!logElement) {
            return;
        }
        logElement.classList.remove("hidden");
        const timestamp = new Date().toLocaleTimeString();
        logElement.textContent += `[${timestamp}] ${message}\n`;
        logElement.scrollTop = logElement.scrollHeight;
    }

    function appendServerLogs(logs = []) {
        if (!Array.isArray(logs) || logs.length === 0) {
            return;
        }
        logs.forEach(function(entry, index) {
            setTimeout(function() {
                appendLog(`[server] ${entry}`);
            }, index * 150);
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

        source.onmessage = function(event) {
            const message = event.data || "";
            appendLog(`[provisioning] ${message}`);
            handleProvisioningStatus(message);
        };

        source.onerror = function() {
            appendLog("Log stream disconnected.");
            closeStream();
        };
    }

    function handleProvisioningStatus(message = "") {
        const lower = message.toLowerCase();
        const progressMatch = message.match(/^\S\s+Provisioning progress:\s*\((\d+)\/(\d+)\)/i);
        if (progressMatch) {
            const spinner = progressMatch[1];
            const done = Number(progressMatch[2]);
            const total = Number(progressMatch[3]);
            updateSummary(`${spinner} Provisioning containers (${done}/${total})`, "text-amber-300");
            if (total > 0 && done >= total && typeof loadDashboard === "function") {
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
            if (typeof loadDashboard === "function") {
                loadDashboard();
            }
            return;
        }
        if (lower.includes("error") || lower.includes("failed")) {
            updateSummary("Provisioning encountered errors", "text-rose-500");
        }
    }

    function updateSummary(statusText = "Waiting for upload", statusClass = "text-slate-300") {
        if (!summary || !summaryFields.status) {
            return;
        }
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

        if (advancedLoggingCheckbox) {
            advancedLoggingCheckbox.disabled = isSubmitting;
        }
    }

    openButton.addEventListener("click", openModal);
    closeButton?.addEventListener("click", closeModal);
    cancelButton?.addEventListener("click", closeModal);
    overlay?.addEventListener("click", closeModal);

    browseButton?.addEventListener("click", function() {
        fileInput?.click();
    });

    function handleFileSelection(file) {
        if (!file) {
            return;
        }
        state.file = file;
        appendLog(`Selected package ${file.name} (${formatBytes(file.size)})`);
        updateSummary("Ready to validate", "text-blue-600");
    }

    fileInput?.addEventListener("change", function(event) {
        const [file] = event.target.files || [];
        handleFileSelection(file);
    });

    if (dropZone) {
        function setDropActive(active) {
            dropZone.classList.toggle("border-blue-400/60", active);
            dropZone.classList.toggle("bg-blue-500/20", active);
            dropZone.classList.toggle("text-white", active);
        }

        dropZone.addEventListener("dragover", function(event) {
            event.preventDefault();
            setDropActive(true);
        });

        dropZone.addEventListener("dragleave", function(event) {
            event.preventDefault();
            setDropActive(false);
        });

        dropZone.addEventListener("drop", function(event) {
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

    form?.addEventListener("submit", async function(event) {
        event.preventDefault();
        if (!state.file || state.submitting) {
            appendLog("Select a package before uploading.");
            updateSummary("Waiting for upload", "text-rose-600");
            return;
        }

        const enableAdvancedLogging = Boolean(advancedLoggingCheckbox?.checked);
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
            payload.append("enableAdvancedLogging", enableAdvancedLogging ? "true" : "false");

            const response = await fetch("/api/competitions/upload", {
                method: "POST",
                body: payload,
                credentials: "include"
            });

            const result = await response.json().catch(function() {
                return {};
            });

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
                if (typeof loadDashboard === "function") {
                    loadDashboard();
                }
            }
        } catch (error) {
            appendLog(`Error: ${error.message}`);
            updateSummary(error.message, "text-rose-600");
        } finally {
            setSubmitting(false);
        }
    });
}
