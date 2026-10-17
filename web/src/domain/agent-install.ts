// Generates installation artefacts locally; host access only happens in the installer.
import type { FileOwner, HostConnection } from "./model";
import { SYSTEM_LABEL } from "./system";
import { absolutePath, installDirProblem, isInside } from "./validation";

export const AGENT_PORT = 3552;
/** Reference image for the design lab; live plans receive their image from the hub. */
export const AGENT_IMAGE = "registry.example.com/docklet/agent:0.33.0";

export interface AgentInstallPlan {
  hostName: string;
  connection: HostConnection;
  address: string;
  port: number;
  hubUrl: string;
  projectDir: string;
  installDir: string;
  fileOwner?: FileOwner;
  /** Only a manual group is emitted into Compose; runtime detection is authoritative. */
  socketGid?: number;
  image?: string;
  token: string;
  timeZone: string;
}

export interface InstallArtifacts {
  compose: string;
  env: string;
  script: string;
  readme: string;
  composeCommand: string;
  scriptCommand: string;
}

const q = (v: string) => `'${v.replace(/'/g, "'\\''")}'`;
const dotenv = (v: string) => `'${v.replace(/\\/g, "\\\\").replace(/'/g, "\\'")}'`;

function validatePlan(plan: AgentInstallPlan): void {
  if (absolutePath(plan.projectDir) || absolutePath(plan.installDir) || installDirProblem(plan.projectDir, plan.installDir)) throw new Error("agent-install.invalid-directories");
  if (![plan.hostName, plan.hubUrl, plan.timeZone, plan.token].every((v) => v && !/[\r\n\0]/.test(v))) throw new Error("agent-install.invalid-value");
  if (!/^[A-Za-z0-9._-]+$/.test(plan.token)) throw new Error("agent-install.invalid-token");
  if (!/^https?:\/\//.test(plan.hubUrl) || new URL(plan.hubUrl).username || new URL(plan.hubUrl).password) throw new Error("agent-install.invalid-hub-url");
  if (!Number.isInteger(plan.port) || plan.port < 1 || plan.port > 65535) throw new Error("agent-install.invalid-port");
  for (const id of [plan.fileOwner?.uid, plan.fileOwner?.gid, plan.socketGid]) {
    if (id !== undefined && (!Number.isInteger(id) || id < 0 || id > 65534)) throw new Error("agent-install.invalid-id");
  }
  const image = plan.image ?? AGENT_IMAGE;
  if (!/^[a-zA-Z0-9./:_-]+:\d+\.\d+\.\d+(?:-[a-zA-Z0-9.-]+)?(?:@sha256:[a-f0-9]{64})?$/.test(image)) throw new Error("agent-install.unpinned-image");
}

export function agentCompose(plan: AgentInstallPlan): string {
  return [
    "# Docklet agent; runtime detects the Docker socket group before dropping privileges.",
    "name: docklet-agent",
    "services:",
    "  agent:",
    `    image: \${DOCKLET_AGENT_IMAGE:-${plan.image ?? AGENT_IMAGE}}`,
    "    restart: unless-stopped",
    "    env_file: .env",
    "    labels:",
    `      ${SYSTEM_LABEL}: "agent"`,
    '      com.getarcaneapp.arcane.hidden: "true"',
    ...(plan.socketGid === undefined ? [] : ["    group_add:", `      - "${plan.socketGid}"`]),
    ...(plan.connection === "direct" ? ["    # Restrict this published port to the hub with the host firewall.", "    ports:", `      - "${plan.port}:${AGENT_PORT}"`] : []),
    "    volumes:",
    "      - /var/run/docker.sock:/var/run/docker.sock",
    `      - ${JSON.stringify(`${plan.projectDir}:${plan.projectDir}`)}`,
    "      - ./data:/app/data",
    "",
  ].join("\n");
}

export function agentEnv(plan: AgentInstallPlan): string {
  return [
    "AGENT_MODE=true",
    `EDGE_AGENT=${plan.connection === "edge"}`,
    `AGENT_TOKEN=${dotenv(plan.token)}`,
    `MANAGER_API_URL=${dotenv(plan.hubUrl)}`,
    `PROJECTS_DIRECTORY=${dotenv(plan.projectDir)}`,
    `PORT=${AGENT_PORT}`,
    `PUID=${plan.fileOwner?.uid ?? ""}`,
    `PGID=${plan.fileOwner?.gid ?? ""}`,
    `TZ=${dotenv(plan.timeZone)}`,
    "",
  ].join("\n");
}

/** Check every recognised Compose filename, including overrides, before changing the target. */
function preflight(plan: AgentInstallPlan): string[] {
  return [
    `INSTALL_DIR=${q(plan.installDir)}`,
    `PROJECT_DIR=${q(plan.projectDir)}`,
    '[ -d "$PROJECT_DIR" ] || { echo "Create the project directory with the intended owner first." >&2; exit 1; }',
    'PROJECT_DIR=$(cd "$PROJECT_DIR" && pwd -P)',
    'INSTALL_DIR=$(realpath -m "$INSTALL_DIR")',
    'case "$PROJECT_DIR/" in "$INSTALL_DIR/"*) echo "Install directory must not equal or contain the project directory." >&2; exit 1;; esac',
    'case "$INSTALL_DIR" in "$PROJECT_DIR"/*) RELATIVE=${INSTALL_DIR#"$PROJECT_DIR"/}; case "$RELATIVE" in */*) echo "Use a direct subfolder, outside any stack." >&2; exit 1;; esac;; esac',
    '[ -w "$PROJECT_DIR" ] || { echo "Project directory is not writable for the installer identity." >&2; exit 1; }',
    'for NAME in compose.yaml compose.yml docker-compose.yaml docker-compose.yml compose.override.yaml compose.override.yml docker-compose.override.yaml docker-compose.override.yml; do',
    '  EXISTING="$INSTALL_DIR/$NAME"',
    '  if [ -e "$EXISTING" ] || [ -L "$EXISTING" ]; then',
    '    [ -f "$EXISTING" ] && [ ! -L "$EXISTING" ] || { echo "Refusing non-regular Compose file." >&2; exit 1; }',
    "    if ! docker compose -f \"$EXISTING\" config --no-interpolate 2>/dev/null | awk '",
    '      /^services:$/ { services=1; next }',
    '      services && /^[^ ]/ { services=0 }',
    '      services && /^  [^ ].*:$/ { if (count && !agent) bad=1; count++; agent=0 }',
    '      services && /^      io[.]docklet[.]system: [\\042\\047]?agent[\\042\\047]?$/ { agent=1 }',
    "      END { exit !(count && agent && !bad) }'; then",
    '      echo "Refusing to overwrite a Compose project without io.docklet.system=agent on every service." >&2; exit 1',
    "    fi",
    "  fi",
    "done",
    'echo "Project directory owner: $(stat -c %u:%g "$PROJECT_DIR")"',
    'echo "Docker socket group (runtime detection is authoritative): $(stat -c %g /var/run/docker.sock 2>/dev/null || echo unavailable)"',
    `PUID=${plan.fileOwner ? q(String(plan.fileOwner.uid)) : '$(stat -c %u "$PROJECT_DIR")'}`,
    `PGID=${plan.fileOwner ? q(String(plan.fileOwner.gid)) : '$(stat -c %g "$PROJECT_DIR")'}`,
  ];
}

function installLines(plan: AgentInstallPlan): string[] {
  return [
    'mkdir -p "$INSTALL_DIR/data"',
    'WORK_DIR=$(mktemp -d "$INSTALL_DIR/.docklet-install.XXXXXXXX")',
    'trap \'rm -rf "$WORK_DIR"\' EXIT',
    "trap 'exit 1' HUP INT TERM",
    'awk -v uid="$PUID" -v gid="$PGID" \'/^PUID=/ {$0="PUID=" uid} /^PGID=/ {$0="PGID=" gid} {print}\' "$PACKAGE_DIR/.env" > "$WORK_DIR/.env"',
    'chmod 600 "$WORK_DIR/.env"',
    'cat > "$WORK_DIR/compose.yaml" <<\'EOF_COMPOSE\'',
    agentCompose(plan).trimEnd(),
    "EOF_COMPOSE",
    'mv -f "$WORK_DIR/.env" "$INSTALL_DIR/.env"',
    'mv -f "$WORK_DIR/compose.yaml" "$INSTALL_DIR/compose.yaml"',
    'cd "$INSTALL_DIR"',
    'docker compose -f compose.yaml --env-file .env up -d',
    'echo "Agent started. The next hub health check reports compatibility and directory checks (default interval: 2 minutes)."',
  ];
}

export function agentScript(plan: AgentInstallPlan): string {
  return [
    "#!/bin/sh",
    "set -eu",
    "umask 077",
    'PACKAGE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd -P)',
    '[ -f "$PACKAGE_DIR/.env" ] || { echo "Keep the package .env beside install.sh." >&2; exit 1; }',
    ...preflight(plan),
    ...installLines(plan),
    "",
  ].join("\n");
}

export function agentReadme(plan: AgentInstallPlan): string {
  return [
    "Docklet agent installation package",
    `Install directory: ${plan.installDir}`,
    `Project directory: ${plan.projectDir}`,
    ...(isInside(plan.installDir, plan.projectDir) ? [`The agent is hidden from discovery by ${SYSTEM_LABEL}=agent and the Arcane hidden label.`] : []),
    "Extract the private archive, then run: sh install.sh",
    "Requires Docker Compose v2, realpath, stat, awk and a writable project directory.",
    "The installer refuses existing Compose projects unless every service is labelled io.docklet.system=agent.",
    "It proposes the directory owner as PUID/PGID; the agent detects the socket group at every start.",
    "No project ownership or permissions are changed. Startup may chown the agent data directory (/app/data).",
    "The token is only in .env (0600). Keep the extracted directory private and DELETE THE ARCHIVE after installation.",
    "Copyable commands contain the .env heredoc: the token may remain in shell history. Prefer the downloaded package.",
    "The image can be overridden with DOCKLET_AGENT_IMAGE (use a pinned version or a locally built image).",
    "HTTP requires a trusted network. Restrict direct published ports to the hub. No VPN is configured.",
    "Agents must reach /api/environments/pair and /api/tunnel/* without browser sign-in; use an internal hub URL if needed.",
    "After an agent update, update-required clears on the next successful hub health run (default: 2 minutes).",
    "",
  ].join("\n");
}

function command(plan: AgentInstallPlan, withScript: boolean): string {
  return [
    "(",
    "set -eu",
    "umask 077",
    ...preflight(plan),
    'PACKAGE_DIR=$(mktemp -d)',
    'trap \'rm -rf "$PACKAGE_DIR"\' EXIT',
    "trap 'exit 1' HUP INT TERM",
    'cat > "$PACKAGE_DIR/.env" <<\'EOF_ENV\'',
    agentEnv(plan).trimEnd(),
    "EOF_ENV",
    ...(withScript ? [
      'cat > "$PACKAGE_DIR/install.sh" <<\'EOF_INSTALLER\'',
      agentScript(plan).trimEnd(),
      "EOF_INSTALLER",
      'chmod 700 "$PACKAGE_DIR/install.sh"',
      'sh "$PACKAGE_DIR/install.sh"',
    ] : [
      // A child shell owns target staging cleanup; the parent owns the private package.
      "(", ...installLines(plan), ")",
    ]),
    ")",
    "",
  ].join("\n");
}

export function buildInstallArtifacts(plan: AgentInstallPlan): InstallArtifacts {
  validatePlan(plan);
  return { compose: agentCompose(plan), env: agentEnv(plan), script: agentScript(plan), readme: agentReadme(plan), composeCommand: command(plan, false), scriptCommand: command(plan, true) };
}

/** Archive permissions are part of the installer contract. */
export function installPackageFiles(a: InstallArtifacts) {
  return [
    { name: "compose.yaml", content: a.compose, mode: 0o644 },
    { name: ".env", content: a.env, mode: 0o600 },
    { name: "install.sh", content: a.script, mode: 0o700 },
    { name: "README.txt", content: a.readme, mode: 0o644 },
  ];
}
