## ADDED Requirements

### Requirement: ACP run 圖片輸出以檔案附件方式傳送至 Discord

當 Discord session 的 ACP run 完成並帶有圖片附件時，Perch 應將這些圖片以 Discord 檔案附件方式，與文字回覆同一則訊息一起傳送。

#### Scenario: 圖片與文字同一訊息傳送

- **WHEN** `RunCompleted` 為 Discord session 產生圖片附件記錄
- **THEN** Perch 讀取每張圖片檔案，加入 `MessageSend.Files`（`Name` 為原始檔名，`Reader` 為檔案位元組），並在同一個 `ChannelMessageSendComplex` 呼叫中與文字 chunk 一起送出

#### Scenario: 超過 Discord 8 MB 限制的圖片略過

- **WHEN** 圖片檔案大於 8 MB
- **THEN** 該檔案不加入 `MessageSend.Files`，並在最後一個文字 chunk 末尾附加 `(圖片過大，無法傳送至 Discord)`

#### Scenario: 圖片傳送失敗不影響文字回覆

- **WHEN** 某張圖片的 Discord 檔案上傳失敗
- **THEN** Perch 記錄錯誤、略過該圖片，仍正常傳送文字回覆與其他成功的圖片

#### Scenario: 無圖片時僅傳送文字回覆

- **WHEN** `RunCompleted` 未產生任何圖片附件記錄
- **THEN** Discord 回覆行為與原本相同（純文字訊息，套用現有 chunking 規則）
