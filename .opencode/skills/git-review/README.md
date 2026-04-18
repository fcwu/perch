# git-review

比對 GitLab MR 或 commit 實作與 JIRA 規格，進行功能完整性審查。

## 功能

- 從 GitLab MR/commit 自動擷取 JIRA ticket ID
- 從 JIRA 取得規格說明（description、attachments、Confluence 連結）
- 比對程式碼變更與規格需求
- 產生 checklist 格式的審查報告

## 安裝

先依 [通用安裝](../../README.md#通用安裝) 安裝 `skills/git-review`，再完成以下依賴與設定。

如專案內已有 `.venv`，請先啟用後再執行以下指令。

### 1. 安裝 Python 套件

```bash
if [ -d .venv ]; then
  . .venv/bin/activate
fi

# 使用 uv (推薦，更快)
uv pip install -r skills/git-review/requirements.txt

# 或使用 pip
pip install -r skills/git-review/requirements.txt

# 或手動安裝
pip install jira atlassian-python-api beautifulsoup4
```

### 2. 設定檔

將設定檔放置於 `~/.config/git-review/` 目錄下：

```bash
mkdir -p ~/.config/git-review
cp skills/git-review/references/gitlab.example.json ~/.config/git-review/gitlab.json
cp skills/git-review/references/jira.example.json ~/.config/git-review/jira.json
```

編輯設定檔，填入您的 credentials：

**gitlab.json** - GitLab 設定（支援多個 instance）：
```json
{
  "instances": {
    "sauron.qnap.com": {
      "url": "https://sauron.qnap.com/",
      "token": "glpat-your-token-here",
      "skip_ssl_verify": true
    },
    "gitlab.example.com": {
      "url": "https://gitlab.example.com",
      "token": "glpat-another-token",
      "skip_ssl_verify": false
    }
  },
  "default_instance": "sauron.qnap.com"
}
```

**jira.json** - JIRA/Confluence 設定：
```json
{
  "jira": {
    "url": "https://jira.example.com",
    "token": "your-jira-personal-access-token",
    "skip_ssl_verify": true
  },
  "confluence": {
    "url": "https://confluence.example.com",
    "token": "your-confluence-personal-access-token",
    "skip_ssl_verify": true
  }
}
```

## 使用方式

### 基本使用

在 AI 助手中直接告訴它要 review 的 GitLab URL：

```
使用 git-review skill 來 review 這個 commit：
https://sauron.qnap.com/cec-sw1/qcloud/portal/-/commit/1c1c1ef574e492d41c71f2bc733cf01c31ec9522
```

```
請用 git-review 來審查這個 MR：
https://sauron.qnap.com/cec-sw1/qcloud/portal/-/merge_requests/456
```

### 審查流程

若需手動執行 scripts，請先啟用 `.venv`（若存在）。

1. AI 助手接收 GitLab URL（MR 或 commit）
2. 執行 `python3 scripts/gitlab_fetcher.py <url>` 取得程式碼變更
3. 從 MR 標題/描述或 commit message 中擷取 JIRA ticket ID
4. 執行 `python3 scripts/jira_fetcher.py <ticket_id>` 取得規格
5. 比對規格需求與程式碼實作
6. 產生 checklist 格式的審查報告

### 輸出格式

審查報告採用 checklist 格式，專注於**功能完整性**：

```markdown
## Spec Review: TEM61510-12345

### ✅ 已實作的需求
- [x] 需求 1 - 實作於 `file.go:123`
- [x] 需求 2 - 涵蓋於 `handler.go` 的變更

### ⚠️ 部分實作
- [ ] 需求 3 - 缺少 edge case X 的錯誤處理
- [ ] 需求 4 - API endpoint 已建立但驗證不完整

### ❌ 未實作
- [ ] 需求 5 - 沒有相關的程式碼變更
- [ ] 需求 6 - 規格中提到但未處理

### 📝 備註
- 關於實作品質的額外觀察
- 改進建議
```

### 環境變數（替代方案）

如果不想使用設定檔，也可以使用環境變數。

#### GitLab（支援多組 instance）

使用 `GITLAB_TOKEN_<HOST>` 格式來設定不同 GitLab instance 的 token：

```bash
# 針對特定 host 設定 (將 . 和 - 替換為 _，轉大寫)
# sauron.qnap.com → GITLAB_TOKEN_SAURON_QNAP_COM
export GITLAB_TOKEN_SAURON_QNAP_COM="glpat-token-for-sauron"
export GITLAB_SKIP_SSL_SAURON_QNAP_COM="true"

# qif-gitlab.dev-myqnapcloud.com → GITLAB_TOKEN_QIF_GITLAB_DEV_MYQNAPCLOUD_COM
export GITLAB_TOKEN_QIF_GITLAB_DEV_MYQNAPCLOUD_COM="glpat-token-for-qif"
export GITLAB_SKIP_SSL_QIF_GITLAB_DEV_MYQNAPCLOUD_COM="true"

# gitlab.com → GITLAB_TOKEN_GITLAB_COM
export GITLAB_TOKEN_GITLAB_COM="glpat-token-for-gitlab-com"

# 預設 token（當找不到特定 host 的 token 時使用）
export GITLAB_TOKEN="glpat-default-token"
export GITLAB_SKIP_SSL_VERIFY="true"  # 全域 SSL 設定
```

**環境變數命名規則：**
- 將 hostname 轉為大寫
- 將 `.` 和 `-` 替換為 `_`
- 例如：`my-gitlab.example.com` → `GITLAB_TOKEN_MY_GITLAB_EXAMPLE_COM`

#### JIRA / Confluence

```bash
# JIRA
export JIRA_URL="https://jira.example.com"
export JIRA_TOKEN="your-jira-personal-access-token"
export JIRA_SKIP_SSL_VERIFY="true"

# Confluence
export CONFLUENCE_URL="https://confluence.example.com"
export CONFLUENCE_TOKEN="your-confluence-personal-access-token"
export CONFLUENCE_SKIP_SSL_VERIFY="true"
```

### 支援的 JIRA Ticket ID 格式

- `TEM61510-12345`
- `PROJ-123`
- `ABC123-456`

Pattern: `[A-Z][A-Z0-9]+-\d+`

如果無法從 MR/commit 中找到 ticket ID，AI 助手會詢問您提供。
