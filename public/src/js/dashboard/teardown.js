export function createTeardownController({ loadDashboard } = {}) {
    const teardownModal = document.getElementById("teardown-modal");
    const teardownStatus = teardownModal?.querySelector("[data-teardown-status]");
    const teardownTarget = teardownModal?.querySelector("[data-teardown-target]");
    const teardownLog = document.getElementById("teardown-log");
    const teardownOverlay = document.getElementById("teardown-overlay");
    const teardownCloseButton = document.getElementById("close-teardown");
    const teardownConfirmButton = teardownModal?.querySelector("[data-teardown-confirm]");
    const teardownConfirmDefaultText =
        teardownConfirmButton?.dataset.defaultLabel?.trim() || teardownConfirmButton?.textContent?.trim() || "Destroy competition";
    let teardownEventSource = null;
    let teardownInProgress = false;

    function updateTeardownStatus(text = "Awaiting confirmation", tone = "text-slate-300") {
        if (!teardownStatus) {
            return;
        }
        teardownStatus.textContent = text;
        teardownStatus.className = `text-sm font-semibold ${tone}`;
    }

    function setTeardownTargetLabel(text = "Awaiting selection") {
        if (!teardownTarget) {
            return;
        }
        teardownTarget.textContent = text;
    }

    function updateTeardownControls() {
        if (teardownConfirmButton) {
            teardownConfirmButton.disabled = teardownInProgress;
            teardownConfirmButton.textContent = teardownInProgress ? "Destroying…" : teardownConfirmDefaultText;
        }
    }

    function setTeardownBusy(isBusy) {
        teardownInProgress = Boolean(isBusy);
        updateTeardownControls();
    }

    function closeTeardownStream() {
        if (!teardownEventSource) {
            return;
        }
        teardownEventSource.close();
        teardownEventSource = null;
    }

    function resetTeardownModalState() {
        if (teardownLog) {
            teardownLog.textContent = "";
            teardownLog.classList.add("hidden");
        }
        updateTeardownStatus();
        closeTeardownStream();
        if (teardownModal) {
            delete teardownModal.dataset.compId;
            delete teardownModal.dataset.compLabel;
        }
        setTeardownBusy(false);
        setTeardownTargetLabel("Awaiting selection");
    }

    function appendTeardownLog(message) {
        if (!teardownLog) {
            return;
        }
        const timestamp = new Date().toLocaleTimeString();
        teardownLog.classList.remove("hidden");
        teardownLog.textContent += `[${timestamp}] ${message}\n`;
        teardownLog.scrollTop = teardownLog.scrollHeight;
    }

    function handleTeardownStatus(message = "") {
        if (!message) {
            return;
        }
        const lower = message.toLowerCase();
        if (lower.includes("torn down successfully") || lower.includes("teardown completed")) {
            updateTeardownStatus("Competition destroyed", "text-emerald-400");
            closeTeardownStream();
            setTeardownBusy(false);
            if (typeof loadDashboard === "function") {
                loadDashboard();
            }
            return;
        }
        if (lower.includes("destroying competition")) {
            updateTeardownStatus("Destroying competition...", "text-amber-400");
            return;
        }
        if (lower.includes("error") || lower.includes("failed")) {
            updateTeardownStatus("Teardown failed", "text-rose-500");
            setTeardownBusy(false);
            closeTeardownStream();
            return;
        }
    }

    function startTeardownLogStream(jobID) {
        if (!jobID) {
            return;
        }

        if (typeof EventSource === "undefined") {
            appendTeardownLog("Live log streaming is unavailable in this browser.");
            updateTeardownStatus("Teardown in progress...", "text-amber-400");
            setTeardownBusy(false);
            return;
        }

        closeTeardownStream();
        appendTeardownLog(`Connecting to teardown log (${jobID})...`);

        const source = new EventSource(`/api/competitions/teardown/${encodeURIComponent(jobID)}/stream`);
        teardownEventSource = source;

        source.onmessage = function(event) {
            const message = event.data || "";
            appendTeardownLog(`[teardown] ${message}`);
            handleTeardownStatus(message);
        };

        source.onerror = function() {
            appendTeardownLog("Log stream disconnected.");
            closeTeardownStream();
            setTeardownBusy(false);
        };
    }

    function openTeardownModal(compName, compID) {
        if (!teardownModal) {
            return;
        }
        resetTeardownModalState();
        if (compID) {
            teardownModal.dataset.compId = compID;
        }
        if (compName) {
            teardownModal.dataset.compLabel = compName;
            setTeardownTargetLabel(compName);
        } else {
            setTeardownTargetLabel("Awaiting selection");
        }
        teardownModal.classList.remove("hidden");
    }

    async function handleTeardownConfirm() {
        if (!teardownModal || !teardownConfirmButton || teardownInProgress || teardownConfirmButton.disabled) {
            return;
        }
        const compID = teardownModal.dataset.compId;
        if (!compID) {
            return;
        }
        const compLabel = teardownModal.dataset.compLabel || compID;

        setTeardownBusy(true);
        appendTeardownLog(`Requesting teardown for ${compLabel}.`);
        updateTeardownStatus("Requesting teardown...", "text-amber-400");

        try {
            const response = await fetch(`/api/competitions/${encodeURIComponent(compID)}/teardown`, {
                method: "POST",
                credentials: "include"
            });
            const payload = await response.json().catch(function() {
                return {};
            });
            if (!response.ok) {
                throw new Error(payload?.error || payload?.message || "Failed to destroy competition");
            }
            const successMessage = payload?.message || `Teardown queued (${payload?.jobID || "pending"})`;
            appendTeardownLog(successMessage);
            if (payload?.jobID) {
                startTeardownLogStream(payload.jobID);
            } else {
                updateTeardownStatus("Teardown in progress...", "text-amber-400");
                setTeardownBusy(false);
            }
        } catch (error) {
            const message = error.message || "Unable to tear down competition.";
            appendTeardownLog(message);
            updateTeardownStatus("Teardown failed", "text-rose-500");
            setTeardownBusy(false);
            window.alert(message);
        }
    }

    function closeTeardownModal() {
        if (!teardownModal || teardownInProgress) {
            return;
        }
        teardownModal.classList.add("hidden");
        resetTeardownModalState();
    }

    teardownOverlay?.addEventListener("click", closeTeardownModal);
    teardownCloseButton?.addEventListener("click", closeTeardownModal);
    teardownConfirmButton?.addEventListener("click", handleTeardownConfirm);

    return {
        openTeardownModal
    };
}
