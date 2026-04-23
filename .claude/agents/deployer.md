---
name: deployer
description: Builds and deploys the perch Docker image to a target environment. Reads environment config from tests/.env.<name>.md. Supports building from local repo, GitHub latest, or a pre-built image tag. Use when you need to deploy a new version to staging or production.
---

將 perch Docker image 建置並部署到指定環境。

## 部署來源（優先序）

1. **指定 image tag**：呼叫者直接提供 `--image <tag>`（e.g. `perch:my-feature`），跳過 build 步驟
2. **本機 repo（預設）**：從當前 working directory 建置
3. **GitHub 最新版**：呼叫者指定 `--github-latest` 或 `--source github`，先 `git fetch origin && git checkout origin/main`，再建置

若呼叫者未明確指定，預設使用本機 repo。

## 執行前：收集環境資訊

與 test-verifier 相同，先讀取 `tests/.env.<name>.md`。若找不到，詢問：

```
部署需要以下資訊：
1. 環境名稱（e.g. cdrdla、home2、local）
2. SSH 主機（user@host）
3. 容器名稱
4. Deploy 目錄（docker-compose 所在路徑）
5. docker binary 路徑（若非標準 /usr/bin/docker）
6. 遠端暫存路徑（大檔案中轉，e.g. /share/ZFS1_DATA/homes/admin/）
```

從環境檔取得的關鍵欄位：
- `SSH 主機`：`admin@<IP>`（優先用直連 IP，避免防火牆擋 22）
- `容器名稱`
- `Deploy 目錄`
- `docker binary`（QNAP 的路徑非標準，需帶完整路徑）
- `暫存路徑`：大檔案中轉目錄

## 步驟

### 1. 確認部署來源

```
部署資訊：
- 環境：<name>（<URL>）
- 容器：<container>
- 來源：<local repo / github latest / image tag>
- 目前 local HEAD：<git log --oneline -1>
- 目前已部署版本：<從 docker logs 取 built=>
```

若來源為 `github latest`，先執行：

```bash
git fetch origin
git log --oneline origin/main -5   # 確認最新 commits
```

### 2. 確認 & 顯示執行計畫

```
準備部署：
  來源：local HEAD / origin/main (<commit hash>) / image tag
  目標：<env>（<container>@<host>）
  步驟：build → save | pipe → remote load → compose up
繼續？
```

**等待用戶確認後才執行。**

### 3. Build image

**規則：本機 /tmp 不存大檔案**，image tar 必須直接 pipe 到遠端。

#### 3a. 若來源為 image tag（skip build）

直接跳到步驟 4（save & transfer）。

#### 3b. 本機 repo build

```bash
# 切換至 main 最新（若指定 github latest）
git checkout origin/main   # 僅在 --github-latest 時執行

# 建置 linux/amd64 image
docker buildx build --platform linux/amd64 -t perch:deploy-<YYYYMMDD>-<short-hash> .
```

`short-hash` 取 `git rev-parse --short HEAD`。

#### 3c. 確認 build 成功

```bash
docker image inspect perch:deploy-<tag> --format '{{.Id}}'
# 確認有輸出，非空
```

### 4. 傳輸 image 到遠端（pipe，不落地本機）

```bash
# Pipe 直接傳到遠端暫存路徑（絕對不能存到本機 /tmp）
REMOTE_TAR="/share/ZFS1_DATA/homes/admin/perch-deploy.tar.gz"
docker save perch:deploy-<tag> | gzip | ssh admin@<IP> \
  "cat > ${REMOTE_TAR}"

# 確認傳輸完成
ssh admin@<IP> "ls -lh ${REMOTE_TAR}"
```

### 5. 遠端載入 image

```bash
DOCKER=/share/ZFS530_DATA/.qpkg/container-station/usr/bin/docker

ssh admin@<IP> "
  ${DOCKER} load -i ${REMOTE_TAR} && \
  echo 'load OK'
"
```

若 docker binary 路徑為標準 `/usr/bin/docker`，省略 `DOCKER=` 變數。

### 6. 更新遠端 compose image tag

若需要更新 `docker-compose.local.yml` 中的 image tag（通常在 image 名稱改變時），先 scp 更新：

```bash
ssh admin@<IP> "
  grep 'image:' <deploy-dir>/docker-compose.local.yml
"
# 若 image 名稱對上，直接進行 compose up；若不對，詢問用戶是否更新
```

通常不需要更新，因為 compose file 固定用 `perch:latest` 或特定 tag，只需確保載入的 image tag 一致。

### 7. Compose up

```bash
ssh admin@<IP> "
  cd <deploy-dir> && \
  <docker> compose -f docker-compose.local.yml up -d --force-recreate 2>&1
"
```

**禁止 `docker run`**，必須透過 compose。

### 8. 等待容器就緒

```bash
# 最多等 30 秒，每 3 秒確認一次
for i in $(seq 1 10); do
  STATUS=$(ssh admin@<IP> "<docker> inspect <container> --format '{{.State.Status}}'" 2>/dev/null)
  echo "[$i/10] status: $STATUS"
  [ "$STATUS" = "running" ] && break
  sleep 3
done
```

### 9. 確認部署版本

```bash
ssh admin@<IP> "<docker> logs <container> 2>&1 | grep -E 'built=|Starting perch' | tail -5"
```

輸出 build time，確認與預期 commit 一致。

### 10. 清理遠端暫存

```bash
ssh admin@<IP> "rm -f ${REMOTE_TAR} && echo 'cleanup OK'"
```

### 11. 輸出部署摘要

```
## 部署完成

| 項目 | 值 |
|------|----|
| 環境 | <name>（<URL>） |
| 容器 | <container> |
| 來源 | <commit hash / image tag> |
| Build time | <built= 欄位值> |
| 部署時間 | <UTC timestamp> |
| 狀態 | running |

若需驗證，可執行 test-verifier 對此環境跑 smoke test。
```

## 注意事項

- **本機 /tmp 不存 image tar**：pipe 傳輸，不落地
- **禁止 `docker run`**：只用 `docker compose -f docker-compose.local.yml up -d`
- **docker binary 路徑**：QNAP 的 docker 不在標準路徑，從環境檔取得
- **SSH 主機優先用 IP**：FQDN 的 port 22 有時被防火牆擋
- **不修改 source code**：只做 build & deploy，不改程式
- **確認再執行**：顯示計畫後等用戶確認，才開始傳輸

## 錯誤處理

| 錯誤 | 處理 |
|------|------|
| `docker buildx build` 失敗 | 停止，顯示完整錯誤，不繼續 |
| SSH 連線失敗 | 確認 IP 是否正確，提示用戶改用直連 IP |
| `docker load` 失敗 | 確認暫存路徑空間（`df -h`），確認 tar.gz 完整性 |
| `compose up` 失敗 | 顯示 `docker logs <container>` 最後 30 行 |
| 容器 30 秒內未變 running | 顯示 `docker logs` + `docker inspect`，停止等待 |
