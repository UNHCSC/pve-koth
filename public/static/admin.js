import * as types from "./lib/types.js";

// Building a Competition is as follows:
// 1. Fill out meta data (name, private, LDAP group restrictions, container restrictions, number of containers per team)
// 2. Add a team builder (name)
// 3. Submit to server, wait for response on dynamic log output of progress

// Managing a Competition is as follows:
// 1. Select a competition from the list of competitions
// 2. Modify scores, power off/on/stop containers, add/remove containers, add/remove teams, stop competition

// Containers can be individually listed and managed:
// 1. Select a container from the list of containers
// 2. Power off/on/stop container, view logs, view console, view stats