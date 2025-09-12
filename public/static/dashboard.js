import * as types from "./lib/types.js";

// Fetch a list of competitions, include credentials if logged in, only competitions the user can see will be loaded
// Display a list of competitions, allow user to select one
// If no competitions, display message
// If competitions, display list of competitions with name, status (running, stopped, etc), number of teams, number of containers
// Allow user to select a competition to view
// On selection, redirect to /dashboard/competition/:id