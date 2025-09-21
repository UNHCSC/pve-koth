import * as types from "/static/lib/types.js";

function setupCreateCompetitionMenu() {
    const createModal = document.querySelector("div#create-modal");
    const createForm = document.querySelector("form#create-comp");
    const createModalOpen = document.querySelector("button#open-create");
    const createModalClose = document.querySelector("button#close-create");
    const createModalCancel = document.querySelector("button#cancel-create");

    function openCreateModal() {
        createModal.classList.remove("hidden");
    }

    function closeCreateModal() {
        createModal.classList.add("hidden");
    }

    createModalOpen.addEventListener("click", openCreateModal);
    createModalClose.addEventListener("click", closeCreateModal);
    createModalCancel.addEventListener("click", closeCreateModal);

    // Adaptive for the private
    const privateSelect = document.querySelector("select#comp-private");
    const groupsDiv = document.querySelector("div#ldap-groups-div");
    const groupsInput = document.querySelector("input#comp-groups");

    privateSelect.addEventListener("change", function handlePrivateChange(e) {
        groupsDiv.classList.toggle("hidden", e.target.value === "0");
        if (e.target.value === "0") {
            groupsInput.value = "";
        }

        groupsInput.required = e.target.value === "1";
    });

    createForm.addEventListener("submit", async function handleFormSubmit(e) {
        e.preventDefault();

        const formData = new FormData(createForm);
        window.formData = formData;

        console.log("Done");
    });
}

document.addEventListener("DOMContentLoaded", function() {
    setupCreateCompetitionMenu();
});