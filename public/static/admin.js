import * as types from "/static/lib/types.js";
const compsEl = document.getElementById("competitions");
const teamsEl = document.getElementById("teams");
const contsEl = document.getElementById("containers");
async function reloadLists() {
    const [comps, teams, containers] = await Promise.all([
        fetch("/api/competitions", { credentials: "include" }).then(r => r.json()),
        fetch("/api/teams", { credentials: "include" }).then(r => r.json()),
        fetch("/api/containers", { credentials: "include" }).then(r => r.json()),
    ]);
    const li = (t) => `<li class=\"p-2 border rounded\">${t}</li>`;
    compsEl.innerHTML = comps.length ? comps.map(c => li(`${c.name} · $
{c.containerIDs.length} containers · ${c.teamIDs.length} teams`)).join("") :
        "<li class=\"text-gray-600\">No competitions</li>";
    teamsEl.innerHTML = teams.length ? teams.map(t => li(`${t.name} · score $
{t.score}`)).join("") : "<li class=\"text-gray-600\">No teams</li>";
    contsEl.innerHTML = containers.length ? containers.map(x => li(`${x.ipAddress
        || "(pending)"} · ${x.status}`)).join("") : "<li class=\"text-gray-600\">No containers</li > ";
}
reloadLists();
// Create competition + stream logs (SSE)
const form = document.getElementById("create-comp");
const logEl = document.getElementById("create-log");
const cancelBtn = document.getElementById("cancel-create");
let currentEventSrc = null;
form.addEventListener("submit", async (e) => {
    e.preventDefault();
    logEl.classList.remove("hidden");
    logEl.textContent = "";
    cancelBtn.classList.remove("hidden");
    const fd = new FormData(form);
    /** @type {types.Competition} */
    const comp = {
        id: 0,
        name: fd.get("name")?.toString() ?? "",
        teamIDs: [],
        containerIDs: [],
        createdAt: new Date(),
        sshPubKeyPath: "",
        sshPrivKeyPath: "",
        containerRestrictions: {
            hostnamePrefix: fd.get("hostnamePrefix")?.toString() ?? "",
            rootPassword: "", // never from UI plain text here
            template: fd.get("template")?.toString() ?? "",
            storagePool: fd.get("storagePool")?.toString() ?? "",
            gatewayIPv4: fd.get("gatewayIPv4")?.toString() ?? "",
            nameserver: fd.get("nameserver")?.toString() ?? "",
            searchDomain: fd.get("searchDomain")?.toString() ?? "",
            storageGB: Number(fd.get("storageGB") ?? 10),
            memoryMB: Number(fd.get("memoryMB") ?? 512),
            cores: Number(fd.get("cores") ?? 1),
            individualCIDR: Number(fd.get("individualCIDR") ?? 0),
        },
        isPrivate: (fd.get("isPrivate") ?? "false").toString() === "true",
        privateLDAPAllowedGroups: (fd.get("groups")?.toString() ||
            "").split(",").map(s => s.trim()).filter(Boolean),
    };
    // Start SSE stream by POSTing JSON and switching to EventSource via token
    const createRes = await fetch("/api/competitions", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        credentials: "include",
        body: JSON.stringify(comp),
    });
    if (!createRes.ok) {
        const err = await createRes.json().catch(() => ({
            error:
                createRes.statusText
        }));
        logEl.textContent = `Error: ${err.error || createRes.statusText}`;
        cancelBtn.classList.add("hidden");
        return;
    }
    const { streamToken } = await createRes.json();
    currentEventSrc = new EventSource(`/api/competitions/stream/${encodeURIComponent(streamToken)}`);
    currentEventSrc.addEventListener("message", (evt) => {
        logEl.textContent += evt.data + "\n";
        logEl.scrollTop = logEl.scrollHeight;
    });
    currentEventSrc.addEventListener("end", async () => {
        currentEventSrc?.close();
        cancelBtn.classList.add("hidden");
        await reloadLists();
    });
    currentEventSrc.onerror = () => {
        currentEventSrc?.close();
        cancelBtn.classList.add("hidden");
    };
});
cancelBtn.addEventListener("click", () => {
    currentEventSrc?.close();
    cancelBtn.classList.add("hidden");
});