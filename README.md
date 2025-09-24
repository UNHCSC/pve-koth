# Proxmox VE King of the Hill

This project is based off of [the old proxmox king of the hill](https://github.com/UNHCSC/proxmox-koth/). I learned a lot of lessons of what to expect when dealing with the Proxmox API, and also just storing and managing all of this data in general.

So here are some of my **goals**:
1. Streamline the API and web app with frameworks
2. Rely on web app for administration
3. Allow for multiple competitions at once
4. Allow for each team to own multiple containers
5. LDAP Logins for the web app, group control (admins, viewers)

Adding ports on `firewall-cmd`:
```bash
firewall-cmd --add-port=8006/tcp --permanent
firewall-cmd --add-port=5000/tcp --permanent
firewall-cmd --reload
```

## Competition ZIP File Structure

```
competition.zip
│
├── config.json
├── README.md
├── writeup.md
├── writeup.pdf
├── public/
│   ├── websiteFiles.tar.gz
│   └── staticFile.txt
├── scripts/
│   ├── setup_global.sh
│   └── setup_website.sh
|   └── score_global.sh
│   └── score_website.sh
```

### config.json [SCHEMA]

Here's a minimal example of a `config.json` file to create a competition with one container per team, which will be a web server. Scripts referenced will be defined below in the scripting section.

```json
{
    "competitionID": "exampleComp",
    "competitionName": "Example Competition",
    "competitionDescription": "This is an example competition.",
    "competitionHost": "Example Host",
    "privacy": {
        "public": true,
        "LDAPAllowedGroupsFilter": ""
    },
    "containerSpecs": {
        "templatePath": "local:vztmpl/ubuntu-25.04-standard.tar.zst",
        "storagePool": "Storage",
        "rootPassword": "password123",
        "storageSizeGB": 8,
        "memoryMB": 1024,
        "cores": 1,
        "gatewayIPv4": "10.0.0.1",
        "cidrBlock": "8",
        "nameServerIPv4": "10.0.0.2",
        "searchDomain": "cyber.lab"
    },
    "teamContainerConfigs": [
        {
            "name": "website",
            "lastOctetValue": 1,
            "setupScript": ["scripts/setup_global.sh", "scripts/setup_website.sh"],
            "scoringScript": ["scripts/score_global.sh", "scripts/score_website.sh"],
            "scoringSchema": [
                {
                    "id": "icmp",
                    "name": "Container Reachable (ICMP)",
                    "passPoints": 1,
                    "failPoints": 0
                },
                {
                    "id": "exporter",
                    "name": "Prometheus Exporter Running",
                    "passPoints": 1,
                    "failPoints": -1
                },
                {
                    "id": "nginx",
                    "name": "Webpage Running (nginx)",
                    "passPoints": 3,
                    "failPoints": -1
                },
                {
                    "id": "content",
                    "name": "Correct Webpage Content",
                    "passPoints": 2,
                    "failPoints": -2
                }
            ]
        }
    ],
    "setupPublicFolder": "public",
    "writeupFilePath": "writeup.pdf"
}
```

As an explanation of the fields:
- `competitionID`: A unique identifier for the competition, used in paths, URLs, and database entries.
- `competitionName`: The display name of the competition.
- `competitionDescription`: A brief description of the competition.
- `competitionHost`: The name of the host or organization running the competition.
- `privacy`: Settings for competition visibility and LDAP group restrictions.
    - `public`: If true, the competition is visible to all users. If false, only users in specified LDAP groups can access it.
    - `LDAPAllowedGroupsFilter`: An LDAP filter string to specify which groups can access the competition (e.g., `(memberOf=cn=cybersec,ou=Groups,dc=example,dc=com)`).
- `containerSpecs`: Default specifications for the containers to be created for each team.
    - `templatePath`: The Proxmox template to use for the containers.
    - `storagePool`: The Proxmox storage pool where the container's disk will be created.
    - `rootPassword`: The root password for the containers.
    - `storageSizeGB`: The size of the container's disk in gigabytes.
    - `memoryMB`: The amount of RAM allocated to each container in megabytes.
    - `cores`: The number of CPU cores allocated to each container.
    - `gatewayIPv4`: The default gateway for the containers.
    - `cidrBlock`: The CIDR block for the container's network.
    - `nameServerIPv4`: The DNS server for the containers.
    - `searchDomain`: The search domain for the containers.
- `teamContainerConfigs`: An array of container configurations for each team. Each object defines a container type.
    - `name`: The name of the container type (e.g., "website").
    - `lastOctetValue`: The last octet of the container's IP address, which will be unique per container.
    - `setupScript`: An array of script paths to run during container setup.
    - `scoringScript`: An array of script paths to run during scoring.
    - `scoringSchema`: An array of scoring criteria for the container.
        - Each scoring criterion includes:
            - `id`: A unique identifier for the scoring criterion.
            - `name`: A descriptive name for the scoring criterion.
            - `passPoints`: Points awarded if the criterion is met.
            - `failPoints`: Points deducted if the criterion is not met.
- `setupPublicFolder`: The path to the folder containing public files to be served to the containers when setup scripts are running.
- `writeupFilePath`: The path to the competition writeup file (PDF or Markdown) that will be publicly available for competition members to view.

## Scripting

### Setup Scripts

### Scoring Scripts

### Public Folder

## Networking