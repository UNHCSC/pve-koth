export class Team {
    id = 0; // Database auto-increment ID
    name = "";
    score = 0;
    /** @type {number[]} */
    containerIDs = [];
    lastUpdated = new Date();
    createdAt = new Date();
}

export class Container {
    id = 0; // Database auto-increment ID
    ipAddress = "";
    status = "";
    lastUpdated = new Date();
    createdAt = new Date();
}

export class ContainerRestrictions {
    hostnamePrefix = "";
    rootPassword = "";
    template = "";
    storagePool = "";
    gatewayIPv4 = "";
    nameserver = "";
    searchDomain = "";
    storageGB = 10;
    memoryMB = 512;
    cores = 1;
    individualCIDR = 0;
}

export class Competition {
    id = 0; // Database auto-increment ID
    name = "";
    /** @type {number[]} */
    teamIDs = [];
    /** @type {number[]} */
    containerIDs = [];
    createdAt = new Date();
    sshPubKeyPath = "";
    sshPrivKeyPath = "";
    containerRestrictions = new ContainerRestrictions();
    isPrivate = false;
    /** @type {string[]} */
    privateLDAPAllowedGroups = [];
}