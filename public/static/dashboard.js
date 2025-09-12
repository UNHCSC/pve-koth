import * as types from "/static/lib/types.js";
const list = document.getElementById("comps");
const empty = document.getElementById("empty");
async function load() {
    const comps = await fetch("/api/competitions", {
        credentials:
            "include"
    }).then(r => r.json());
    if (!comps.length) {
        empty.classList.remove("hidden");
        list.innerHTML = "";
        return;
    }
    empty.classList.add("hidden");
    list.innerHTML = comps.map(c => `
<li class=\"p-3 border rounded flex items-center justify-between\">
<div>
<div class=\"font-medium\">${c.name}</div>
<div class=\"text-xs text-gray-600\">${c.teamIDs.length} teams · $
{c.containerIDs.length} containers</div>
</div>
<a class=\"text-blue-600 text-sm hover:underline\" href=\"/
admin\">Manage</a>
</li>
`).join("");
}
load();