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
    "numTeams": 4,
    "privacy": {
        "public": true,
        "ldapAllowedGroupsFilter": []
    },
    "containerSpecs": {
        "templatePath": "local:vztmpl/ubuntu-25.04-standard.tar.zst",
        "storagePool": "Storage",
        "rootPassword": "password123",
        "storageSizeGB": 8,
        "memoryMB": 1024,
        "cores": 1,
        "gatewayIPv4": "10.0.0.1",
        "cidrBlock": 8,
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
    - `ldapAllowedGroupsFilter`: An array of LDAP group names that are allowed to view the competition when it is private (e.g., `["cn=cybersec,ou=Groups,dc=example,dc=com"]`).
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

### Network Allocation

The server hands out networks to competitions automatically. Configure the `[network]` section in `config.toml` to describe the IPv4 pool that Proxmox containers should live in. The pool must be at least a `/16`, and each competition currently receives a dedicated `/16` which is then subdivided into `/24`s for the teams inside that competition. Containers themselves are provisioned with the `/8` mask defined by `container_cidr` to keep routing simple across nodes. If the configured pool can't provide at least a `/16`, the server will refuse to boot.

### Public URL

Containers need a stable way to reach the KOTH panel in order to download scripts and public files. Set `public_url` inside the `[web_server]` block when the application is exposed through a hostname (for example `https://koth.cyber.lab`). If it is omitted, the server falls back to its local IP address combined with the configured listen port.

## Scoring

Scoring live containers that are constantly being changed by attackers and defenders is a difficult task. The goal of the scoring system is to be as fair and accurate as possible, while also being easy to understand and implement for competition organizers.

The scoring system is based on the concept of "checks". Each container can have multiple checks defined in the `scoringSchema` array in the `config.json` file. Each check has a unique ID, a name, and points for passing or failing the check.

> Note that scoring scripts are executed as the `root` user in the container.

> Note that both pass and fail points are *added* to the team's score. Usually, fail points should be negative values to deduct points for failing a check.

### Scoring Script Output

Scoring scripts' exit status and output DO matter. The exit status should be `0` for success and non-zero for failure. The output of the script on a success should be **only** a JSON object where each property corresponds to the `id` of a check defined in the `scoringSchema` array inside `config.json`:

```json
{
    "ping": true,
    "nginx": false,
    "content": true
}
```

- Each property name must exactly match a check ID from the `scoringSchema`.
- Each property value must be a boolean (`true` = pass, `false` = fail). The scoring engine automatically applies the configured `passPoints` or `failPoints` when it sees those results.

Yes, it can be annoying to construct JSON output in bash scripts, but it makes the scoring system much more robust and easier to understand, as well as allowing for efficient collection and aggregation of multiple check scripts on a container in a scoring loop.

If a scoring script fails (non-zero exit status) or does not output valid JSON, the scoring engine will assume that all checks failed and award the fail points for each check.

> Note that a script "failing" means that the execution of your commands in the script has just failed. If a check fails as "incorrect" or "false", that is not a script failure, but rather a check failure, and the script should still exit with a status of `0` and output the JSON object with the check result as `false`.

### Background Scoring Loop

The server automatically scores every active competition once per minute. Each run:

- Loads the competition's configuration from the uploaded package and rebuilds the expected network layout for each team.
- Connects to every provisioned container in parallel per competition using the private SSH key that was generated during provisioning.
- Executes the configured scoring scripts in order, supplying the same `KOTH_*` environment variables that setup scripts receive (including `KOTH_ACCESS_TOKEN` and public folder URLs).
- Applies pass/fail points for every check reported by scoring scripts. If any script fails to run, omits a check, or returns invalid JSON, the missing checks in that container's `scoringSchema` automatically receive their configured fail points.
- Updates each team's score and `lastUpdated` timestamp in the database so the public scoreboard can refresh immediately.

Because each competition is processed concurrently, large events continue to score without blocking one another. If a container is offline or SSH authentication fails, its checks are treated as failed for that round, which matches the behavior described above.

## Scripting

### Script Execution

Scripts are executed in the order they are listed in the `setupScript` and `scoringScript` arrays in the `config.json` file. Each script will be served to the container via a secure HTTP endpoint, and executed in the form defined by these functions:

```go
func SetEnvs(envs map[string]any) (result string) {
	for k, v := range envs {
		result += fmt.Sprintf("%s=\"%v\" ", k, v)
	}

	if result != "" {
		result = result[:len(result)-1]
	}

	return
}

func LoadAndRunScript(scriptURL, accessToken string, envs map[string]any) (fullCommandlet string) {
	fullCommandlet = fmt.Sprintf("wget --header='Cookie: Authorization=%s' -qO- '%s' | %s bash -s --", accessToken, scriptURL, SetEnvs(envs))
	return
}
```

To summarize, the script will be downloaded using `wget` with an authorization header containing an access token. The script will then be executed with `bash`, and any environment variables defined in the `envs` map will be set before execution. The script is never stored on the container's filesystem.

### Environment Variables

The following environment variables will be set for each script execution:

- `KOTH_COMP_ID`: The competition ID from `config.json`.
- `KOTH_ACCESS_TOKEN`: The unique access token that is valid through the execution of the script which allows you to make requests to the web server for files in the public folder.
- `KOTH_PUBLIC_FOLDER`: The full HTTP path for the public folder for this competition (points at `/api/competitions/<id>/public/<setupPublicFolder>`).
- `KOTH_TEAM_ID`: The team ID of the current team
- `KOTH_HOSTNAME`: The hostname of the container.
- `KOTH_IP`: The IPv4 address of the container.
- `KOTH_CONTAINER_IPS_*`: A set of variables for every container in the team, where `*` is replaced with the configured name for that container. The value is the IPv4 address of that container.
- `KOTH_CONTAINER_IPS`: A comma-separated list of all container IPv4 addresses for the team.

### Serving Public Files

Every file extracted from the competition package lives in the stored package directory (by default `<storage_base>/packages/<competitionID>-<timestamp>/...`). Only the folder referenced by `setupPublicFolder` is served to competitors at `GET /api/competitions/<competitionID>/public/*`. Setup scripts should download their payloads by referencing `$KOTH_PUBLIC_FOLDER`, for example `wget "$KOTH_PUBLIC_FOLDER/website.tar.gz" -O /tmp/website.tar.gz`. All requests **must** include the `Authorization` cookie set to the value of `KOTH_ACCESS_TOKEN`, which is injected by the provisioning system and expires after the script completes.

### Setup Scripts

In our example above, we have two setup scripts: `scripts/setup_global.sh` and `scripts/setup_website.sh`.

#### scripts/setup_global.sh

```bash
#!/bin/bash

# Update and Upgrade
apt-get update && apt-get upgrade -y

# Install Packages
apt-get install -y curl wget nano

# Install Prometheus Node Exporter
wget https://github.com/prometheus/node_exporter/releases/download/v1.9.1/node_exporter-1.9.1.linux-amd64.tar.gz -O /tmp/node_exporter.tar.gz
tar -xzf /tmp/node_exporter.tar.gz -C /opt
mv /opt/node_exporter-1.9.1.linux-amd64 /opt/node_exporter

# Create systemd service for Node Exporter
cat <<EOF > /etc/systemd/system/node_exporter.service
[Unit]
Description=Prometheus Node Exporter
After=network.target

[Service]
User=root
Type=simple
WorkingDirectory=/opt/node_exporter
ExecStart=bash -c "/opt/node_exporter/node_exporter"
Restart=always

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now node_exporter

# Team's User
useradd -m -s /bin/bash koth
echo "koth:password" | chpasswd
usermod -aG sudo koth
echo "koth ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

# Other Users
newUsers=("Sylvia Schneider" "Zack Chan" "Katy Rivas" "Amie Freeman" "Nikita Willis" "Demi-Leigh Rocha" "Nate Alexander" "Amelie Bright" "Angus Larson" "Laila Lyons" "Paul Fitzgerald" "Violet Nolan" "Jada Terry" "Armaan Huffman" "Moshe Thornton" "Lorcan Pham" "Tomas O'Quinn" "Kajus Miranda" "Millie Bridges" "Ben Allen" "Vivian Cantu" "Lyndon Massey")

for user in "${newUsers[@]}"; do
    username=$(echo "$user" | tr '[:upper:]' '[:lower:]' | tr ' ' '.')
    if [[ ! " ${USERS[*]} " =~ " ${username} " ]]; then
        USERS+=("$username")
        useradd -m -s /bin/bash "$username"
        chfn -f "$user" "$username"
        echo "$username:password" | chpasswd
        echo "$username ALL=(ALL) NOPASSWD: ALL" >>/etc/sudoers
    fi
done
```

#### scripts/setup_website.sh

```bash
#!/bin/bash

# Install specific packages
apt-get install -y nginx

# Get the content of the website
wget $KOTH_PUBLIC_FOLDER/website.tar.gz -O /tmp/website.tar.gz
mkdir -p /tmp/website
tar -xzf /tmp/website.tar.gz -C /tmp/website

mv /tmp/website/* /var/www/html/

# Set up NGINX
cat <<EOF > /etc/nginx/sites-available/placebo-banking
server {
    listen 80;
    server_name _;

    root /var/www/html;
    index index.html;

    location / {
        try_files \$uri \$uri/ =404;
    }

    location /api/ {
        proxy_pass http://$KOTH_CONTAINER_IPS_api:5000/;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }
}
EOF

ln -sf /etc/nginx/sites-available/placebo-banking /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default
nginx -t && systemctl reload nginx
```

In the website setup script, we reference another container in the team using the `KOTH_CONTAINER_IPS_api` environment variable, which is automatically set to the IP address of the container named `api`. We didn't define an `api` container in our example `config.json`, but it is included as an example to show that we can reference other containers in the team.

### Scoring Scripts

In our example above, we have two scoring scripts: `scripts/score_global.sh` and `scripts/score_website.sh`.

#### scripts/score_global.sh

```bash
#!/bin/bash

CHECK_icmp=false
CHECK_exporter=false

# Container Can be Pinged? +1, -0
if ping -c 1 -W 1 "$KOTH_IP" &> /dev/null; then
    CHECK_icmp=true
fi

# Prometheus Exporter Running? +1, -1
if curl -s --max-time 2 "http://$KOTH_IP:9100/metrics" | grep -q "Processor"; then
    CHECK_exporter=true
fi

# Now give the data out as JSON
echo "{
    \"icmp\": $CHECK_icmp,
    \"exporter\": $CHECK_exporter
}"
```

#### scripts/score_website.sh

```bash
#!/bin/bash

CHECK_nginx=false
CHECK_content=false

# Nginx service running? +3, -1
if systemctl is-active --quiet nginx; then
    CHECK_nginx=true
fi

# Correct webpage content? +2, -2
if curl -s --max-time 2 "http://$KOTH_IP" | grep -q "Placebo Banking"; then
    CHECK_content=true
fi

# Now give the data out as JSON
echo "{
    \"nginx\": $CHECK_nginx,
    \"content\": $CHECK_content
}"
```

> Note that you can still use environment variables in scoring scripts, just like setup scripts!

### Public Folder

The public folder is a directory in the competition ZIP file that contains files that will be made available to the containers during setup script execution. The files in this folder can be accessed via HTTP using the `KOTH_PUBLIC_FOLDER` environment variable. Like scripts, files in the public folder can only be accessed with a valid access token.

The public folder in the ZIP file must be less than 100MB in size by default. This is configurable in the King of the Hill environment variables file.

## Networking

TODO

## Frontend Assets

The public site now uses Tailwind CSS and Webpack for its styles and scripts. Source files live under
`public/static/src`, and compiled assets are emitted to `public/static/build`.

```bash
npm install          # first-time setup
npm run build        # compile CSS/JS once
npm run devel        # watch Tailwind + Webpack during development
```

Built assets are written to `public/static/build` and served automatically by Fiber.

## Competition Teardown

Administrators can tear down an entire competition environment (containers, SSH keys, and database records) via:

```
POST /api/competitions/:competitionID/teardown
```

The request now queues a background teardown job and returns JSON containing a `jobID`. Clients can stream live logs from `/api/competitions/teardown/:jobID/stream` while the teardown runs, and the log still includes the final success or failure outcome.

This endpoint requires an authenticated admin session. The server stops and deletes every container recorded for the competition, removes the team/container rows from the database, and deletes the data directory under `koth_live_data/competitions/<competitionID>`. Uploaded packages remain untouched so the event can be reprovisioned later if needed.
