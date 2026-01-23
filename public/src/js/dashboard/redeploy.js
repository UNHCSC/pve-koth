export function createRedeployController({ loadCompetitionContainers } = {}) {
    const redeployModal = document.getElementById("redeploy-modal");
    const redeployStatus = redeployModal?.querySelector("[data-redeploy-status]");
    const redeployTarget = redeployModal?.querySelector("[data-redeploy-target]");
    const redeployLog = document.getElementById("redeploy-log");
    const redeployOverlay = document.getElementById("redeploy-overlay");
    const redeployCloseButton = document.getElementById("close-redeploy");
    const redeployStartCheckbox = redeployModal?.querySelector("[data-redeploy-start-checkbox]");
    const redeployAdvancedLoggingCheckbox = redeployModal?.querySelector("[data-redeploy-advanced-logging]");
    const redeployConfirmButton = redeployModal?.querySelector("[data-redeploy-confirm]");
    const redeployConfirmDefaultText =
        redeployConfirmButton?.dataset.defaultLabel?.trim() || redeployConfirmButton?.textContent?.trim() || "Redeploy";
    let redeployEventSource = null;
    let redeployStreamCompID = "";
    let redeployInProgress = false;

    function updateRedeployStatus(text = "Waiting to start", tone = "text-slate-300") {
        if (!redeployStatus) {
            return;
        }
        redeployStatus.textContent = text;
        redeployStatus.className = `text-sm font-semibold ${tone}`;
    }

    function setRedeployTargetLabel(text = "Awaiting selection") {
        if (!redeployTarget) {
            return;
        }
        redeployTarget.textContent = text;
    }

    function updateRedeployControls() {
        if (redeployConfirmButton) {
            redeployConfirmButton.disabled = redeployInProgress;
            redeployConfirmButton.textContent = redeployInProgress ? "Redeploying…" : redeployConfirmDefaultText;
        }
        if (redeployStartCheckbox) {
            redeployStartCheckbox.disabled = redeployInProgress;
        }
        if (redeployAdvancedLoggingCheckbox) {
            redeployAdvancedLoggingCheckbox.disabled = redeployInProgress;
        }
    }

    function setRedeployBusy(isBusy) {
        redeployInProgress = Boolean(isBusy);
        updateRedeployControls();
    }

    function closeRedeployStream() {
        if (!redeployEventSource) {
            return;
        }
        redeployEventSource.close();
        redeployEventSource = null;
        redeployStreamCompID = "";
    }

    function resetRedeployModalState() {
        if (redeployLog) {
            redeployLog.textContent = "";
            redeployLog.classList.add("hidden");
        }
        updateRedeployStatus();
        closeRedeployStream();
        if (redeployConfirmButton) {
            redeployConfirmButton.textContent = redeployConfirmDefaultText;
        }
        if (redeployStartCheckbox) {
            redeployStartCheckbox.checked = false;
        }
        if (redeployAdvancedLoggingCheckbox) {
            redeployAdvancedLoggingCheckbox.checked = false;
        }
        if (redeployModal) {
            delete redeployModal.dataset.containerId;
            delete redeployModal.dataset.containerLabel;
            delete redeployModal.dataset.compId;
        }
        updateRedeployControls();
    }

    function appendRedeployLog(message) {
        if (!redeployLog) {
            return;
        }
        const timestamp = new Date().toLocaleTimeString();
        redeployLog.classList.remove("hidden");
        redeployLog.textContent += `[${timestamp}] ${message}\n`;
        redeployLog.scrollTop = redeployLog.scrollHeight;
    }

    function handleRedeployStatus(message = "") {
        const lower = message.toLowerCase();
        if (!message) {
            return;
        }
        if (lower.includes("redeploy completed")) {
            updateRedeployStatus("Redeploy complete", "text-emerald-400");
            if (redeployStreamCompID && typeof loadCompetitionContainers === "function") {
                loadCompetitionContainers(redeployStreamCompID);
                redeployStreamCompID = "";
            }
            setRedeployBusy(false);
            return;
        }
        if (lower.includes("redeploy failed") || lower.includes("error:")) {
            updateRedeployStatus("Redeploy failed", "text-rose-500");
            if (lower.includes("redeploy failed")) {
                setRedeployBusy(false);
            }
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

        source.onmessage = function(event) {
            const message = event.data || "";
            appendRedeployLog(`[redeploy] ${message}`);
            handleRedeployStatus(message);
        };

        source.onerror = function() {
            appendRedeployLog("Log stream disconnected.");
            closeRedeployStream();
            setRedeployBusy(false);
        };
    }

    function openRedeployModal(label, id, compID = "") {
        if (!redeployModal) {
            return;
        }
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
        if (!redeployModal || !redeployConfirmButton || redeployInProgress || redeployConfirmButton.disabled) {
            return;
        }
        const id = Number(redeployModal.dataset.containerId);
        if (!Number.isFinite(id)) {
            return;
        }
        const compID = redeployModal.dataset.compId || "";
        const containerLabel = redeployModal.dataset.containerLabel || `CT-${id}`;
        const startAfter = Boolean(redeployStartCheckbox?.checked);
        const advancedLogging = Boolean(redeployAdvancedLoggingCheckbox?.checked);

        setRedeployBusy(true);

        appendRedeployLog(`Queued redeploy for ${containerLabel}.`);
        updateRedeployStatus("Redeploy in progress...", "text-amber-400");

        try {
            const response = await fetch("/api/containers/redeploy", {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ ids: [id], startAfter, enableAdvancedLogging: advancedLogging })
            });
            const payload = await response.json().catch(function() {
                return {};
            });
            if (!response.ok) {
                throw new Error(payload?.error || payload?.message || "Failed to redeploy container");
            }
            const successMessage = payload?.message || "Container redeployed.";
            appendRedeployLog(successMessage);
            if (payload?.jobID) {
                startRedeployLogStream(payload.jobID, compID);
            } else {
                setRedeployBusy(false);
                updateRedeployStatus("Redeploy in progress...", "text-amber-400");
            }
        } catch (error) {
            const message = error.message || "Unable to redeploy container.";
            appendRedeployLog(message);
            closeRedeployStream();
            updateRedeployStatus("Redeploy failed", "text-rose-500");
            setRedeployBusy(false);
            window.alert(message);
        }
    }

    function closeRedeployModal() {
        if (!redeployModal) {
            return;
        }
        redeployModal.classList.add("hidden");
        setRedeployTargetLabel("Awaiting selection");
        updateRedeployStatus();
        closeRedeployStream();
    }

    redeployOverlay?.addEventListener("click", closeRedeployModal);
    redeployCloseButton?.addEventListener("click", closeRedeployModal);
    redeployConfirmButton?.addEventListener("click", handleRedeployConfirm);

    return {
        openRedeployModal
    };
}
