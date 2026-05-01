## MODIFIED Requirements

### Requirement: Runtime prerequisites are installed in the runtime image

The runtime image SHALL ship binaries for every supported `AGENT_RUNTIME` so the runtime can be switched without rebuilding. The OpenCode binary SHALL be sourced from `sst/opencode` GitHub releases (NOT `anomalyco/opencode`), and the asset MUST match the host architecture (amd64 → `linux-x64.tar.gz`, arm64 → `linux-arm64.tar.gz`).

> Modification rationale: prior to this change the Dockerfile pulled an arm64-only tarball from `anomalyco/opencode`, leaving amd64 deployments with a broken binary. The official upstream is `sst/opencode`, which publishes both amd64 and arm64 assets and ships native ACP support.

#### Scenario: amd64 host installs opencode-linux-x64

- **WHEN** the image is built on an amd64 host
- **THEN** Dockerfile detects `dpkg --print-architecture == amd64` and downloads `opencode-linux-x64.tar.gz` from `sst/opencode/releases/latest`

#### Scenario: arm64 host installs opencode-linux-arm64

- **WHEN** the image is built on an arm64 host
- **THEN** Dockerfile detects `dpkg --print-architecture == arm64` and downloads `opencode-linux-arm64.tar.gz`

#### Scenario: claude-agent-acp coexists with opencode

- **WHEN** the image build completes
- **THEN** both `/usr/local/bin/opencode` and the npm-installed `claude-agent-acp` are present and executable
- **AND** `AGENT_RUNTIME` at startup picks which one is used by the ACP path

#### Scenario: Unsupported architecture fails the build

- **WHEN** the image is built on a host whose `dpkg --print-architecture` is neither amd64 nor arm64
- **THEN** the Dockerfile RUN step exits non-zero with a clear error message
