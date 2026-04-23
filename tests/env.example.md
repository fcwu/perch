# 環境設定範本

> 複製此檔並命名為 `tests/.env.<name>.md`，填入實際值後使用。
> 參考通用方法：`docs/debug.md`

## 連線資訊

| 項目                   | 值                            |
| ---------------------- | ----------------------------- |
| 本機 URL               | `<protocol>://<hostname>`     |
| 主機                   | `<host-ip>:<ssh-port>`        |
| SSH 主機               | 使用 SSH config `<ssh-alias>` |
| 容器名稱               | `<container-name>`            |
| Deploy 目錄            | `/path/to/deploy/dir/`        |
| docker binary          | `/path/to/docker`             |
| 暫存路徑（大檔案中轉） | `/path/to/staging/dir`        |

## Discord 設定

| 環境變數              | 值                         |
| --------------------- | -------------------------- |
| `DISCORD_BOT_TOKEN`   | `<your-discord-bot-token>` |
| `DISCORD_ACP_ENABLED` | `true`                     |
| `ACP_EXECUTABLE`      | `claude-agent-acp`         |
| `ACP_RUN_TIMEOUT`     | `120`                      |

## Workspace Git 測試環境

| 項目                        | 值                                                     |
| --------------------------- | ------------------------------------------------------ |
| Bare repo（模擬 remote）    | `/path/to/git-repos/workspace-test.git`（host）        |
| Working workspace           | `/path/to/workspaces/test-workspace`（host）           |
| Container 內 workspace 路徑 | `/workspace`                                           |
| Container 內 bare repo 路徑 | `/git-repos/workspace-test.git`（remote URL 即此路徑） |
| Default branch              | `master`                                               |

### docker-compose.local.yml

`/path/to/deploy/dir/docker-compose.local.yml` 已設定：

- image: `perch:test-deploy`
- `/workspace` → `test-workspace`（working clone）
- `/git-repos` → `git-repos/`（bare repo，供 git push/pull）

### 啟動 test 環境（含 git sync）

```bash
ssh <ssh-alias> "
  cd /path/to/deploy/dir &&
  /path/to/docker compose \
    -f docker-compose.yml -f docker-compose.local.yml up -d
"
```

### 驗證 git sync 是否正常

```bash
# 在 host 上直接修改 workspace 檔案，看 perch 是否自動 commit + push
ssh <ssh-alias> "
  DOCKER=/path/to/docker
  echo 'test change' >> /path/to/workspaces/test-workspace/test.txt

  # 等一個 sync interval（預設 300s，可在 .env 調整 WORKSPACE_GIT_SYNC_INTERVAL）
  # 之後確認 bare repo 是否有新 commit
  \$DOCKER run --rm --entrypoint sh \
    -v /path/to/git-repos:/git-repos \
    alpine/git -c 'git -C /git-repos/workspace-test.git log --oneline -5'
"
```

---

## Agent 操作規範

> 以下規則適用於所有在此環境執行的 agent，**必須遵守**。

部署務必以 docker compose 為主，**禁止直接在遠端執行 docker run**，以免造成環境不一致或資源浪費。

### 暫存檔案

- **本機 `/tmp` 不可存大檔案**（Docker image tar 等）——本機 /tmp 空間有限
- **本機大檔案暫存**：使用 `/tmp` 只限截圖（< 1 MB）等小檔案
- **遠端暫存路徑**：`/path/to/staging/dir`——大型檔案（Docker image tar.gz）必須傳到這裡

### Build & Deploy image 的正確流程

```bash
# 1. 本機 build（amd64）
docker buildx build --platform linux/amd64 -t perch:test-deploy /path/to/project

# 2. 存到本機並直接 pipe 傳到遠端（不落地到本機 /tmp）
docker save perch:test-deploy | gzip | ssh <ssh-alias> \
  "cat > /path/to/staging/dir/perch-test.tar.gz"

# 3. 遠端載入並重啟
ssh <ssh-alias> "
  /path/to/docker load \
    -i /path/to/staging/dir/perch-test.tar.gz &&
  cd /path/to/deploy/dir &&
  docker compose -f docker-compose.local.yml up -d
"
```

### Using Browser

務必使用環境變數 `chrome-cdp` 指定 DevToolsActivePort 路徑：

```
CDP_PORT_FILE=$(git rev-parse --show-toplevel)/tests/.chrome-agent/DevToolsActivePort node .../cdp.mjs
```

截圖（< 1 MB）可存本機 `/tmp`，例如：

```bash
node .../cdp.mjs shot <target> /tmp/test-screenshot.png
```
