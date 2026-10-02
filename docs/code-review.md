# 程式碼審查報告：場地預約系統後端

- 審查日期：2026-10-02
- 審查基準：`main` @ `8fd96d0`（working tree clean）
- 審查重點：**正確性**、**安全性**（惡意輸入、權限越界、資安）
- 語言：繁體中文（程式碼識別字維持原文）

---

## 0. 範圍與方法

**範圍**：`cmd/`、`internal/` 全部模組（auth、user、organization、location、resource、booking、pickup、skilllevel、sports、skillrating、notification、favorite、file、announcement、api、config、db、pkg）、`db/migrations/000001–000011`、`Dockerfile`、`compose*.yml`、`set_admin.sh`、`.env.example`，並對照 `docs/role_system.md` 與 OpenAPI 描述確認「刻意設計」與「疑似漏洞」的界線。

**方法**：逐檔人工閱讀（靜態審查），權限檢查逐條路由追蹤；另外做了三項實證：

| 實證 | 結果 |
| --- | --- |
| `go vet ./...` | 無警告 |
| 以獨立小程式驗證 validator 對 `*string` 空字串的行為 | 確認 `omitempty,uuid` / `omitempty,tw_phone` 會**拒絕**空字串；`required` 會拒絕數值 `0`（見 L-3） |
| 以獨立小程式驗證 pgx 對 NULL 的掃描 | 確認 NULL 掃進 `string` 會回傳 `cannot scan NULL into *string`，掃進 `*string` 則正常（見 H-1） |
| `govulncheck ./...` | 7 個可達漏洞，皆為依賴 / 工具鏈版本問題（見 §4） |

**限制（請讀者留意）**：
- 測試資料庫沒有在跑（`docker ps` 為空），所以**整合測試沒有執行**，也沒有對執行中的服務做動態重現。下列每個發現都標註「靜態推導」或「已實證」。
- 標註「靜態推導」的項目，我認為邏輯清楚，但建議修復時先補一個能重現的整合測試再動手。
- 未審查：前端、CI/CD、實際部署的 `.env` 值、Cloudflare Tunnel 設定。

---

## 1. 摘要

| 嚴重度 | 數量 | 項目 |
| --- | :---: | --- |
| **High** | 2 | H-1、H-2 |
| **Medium** | 10 | M-1 ~ M-10 |
| **Low** | 7 | L-1 ~ L-7 |
| 依賴漏洞 | — | §4 |

最值得優先處理的三件事：

1. **H-1**：任何人註冊時把 `display_name` 填成純空白，再建立一筆預約，就能讓該預約**無法被讀取、修改、刪除**，並使整個資源的 `GET /resources/:id/availability` 對所有人回 500。
2. **H-2**：被團主 `rejected` 的報名者可以自己把訂單改回佔位狀態，或繞一圈重新報名，團主的「拒絕」不具約束力。
3. **M-2 / M-3**：公開的註冊、登入端點沒有任何速率限制，加上 bcrypt cost 12 與無逾時的 `http.Server`，是最直接的資源耗盡面。

整體評價：架構一致、SQL 全程參數化、排序欄位皆有白名單、檔案上傳有 magic-bytes 與重新編碼、併發控制（排他約束、`FOR UPDATE`、advisory lock）用心。問題集中在**狀態機（誰能把狀態改成什麼）**、**NULL / 空值處理**、**帳號停用後的殘留資料**，以及**缺少濫用防護**。

### 1.1 與 `docs/role_system.md` §8 的關係

`role_system.md` §8 已經記錄了數個待確認問題。本報告**不重複計為新發現**，只在相關處交叉引用：

| role_system.md | 狀態（以本次審查為準） |
| --- | --- |
| §8.1 頭像路由被 admin 群組擋住 | **文件已過時**：`route.go` 目前把 avatar 路由放在獨立的 `avatarGroup`（只掛 `authMiddleware`），handler 內的 `isSelfOrSysAdmin` 不是死碼。建議更新或刪除該節。 |
| §8.2 Location Manager 權限受限 | 屬實，見 §6 待確認 Q3 |
| §8.3 Org Manager 無法列出組織預約 | 屬實，見 §6 待確認 Q4 |
| §8.4 組織對臨打團無管轄權 | 屬實（設計問題，非缺陷） |
| §8.5 任何登入者可讀任意檔案 | 屬實，目前只存頭像 / 封面，風險可接受 |
| §8.6 AddLocationManager 缺 FK 映射 → 500 | **仍未修復**，見 M-9 |

另外，`docs/changelogs/20261001.md` 已明確記載「所有已登入使用者都能建立臨打團，不需 `is_pickup_host`」，所以 `CreateGroup` 沒檢查團主身份**是刻意的**，不列為缺陷；但 `role_system.md` §2.1、§4.2 仍寫「臨打團主」為開團角色，文件與現況不一致。

---

## 2. 發現（High / Medium）

### H-1　`display_name` 為 NULL 時，預約讀取全面失敗（任何使用者可癱瘓單一資源的可用時段）

- **類型**：正確性 + 可被惡意觸發的 DoS
- **位置**：`internal/user/service.go:163-166`（寫入 NULL）、`internal/booking/model.go:77` + `internal/booking/repository.go:89,176`（`UserName string` 掃描）
- **驗證**：靜態推導 + 已實證 pgx 行為

**成因**

1. `RegisterRequest.DisplayName` 是 `binding:"required,max=50"`，`"   "`（純空白）能通過 `required`。
2. `Register` 把它 `TrimSpace` 後判定為空，存成 `NULL`（`displayNamePtr = nil`）。`users.display_name` 欄位本身允許 NULL。
3. `booking` 的兩個查詢 `JOIN users` 取 `u.display_name`，並掃進 **`string`**（`Booking.UserName`），不是 `*string`。NULL 掃進 `string` 會回傳錯誤。

**影響（逐一對照程式碼）**

| 操作 | 結果 |
| --- | --- |
| `POST /bookings`（該使用者） | 先 `INSERT` 成功，隨後 `repo.GetByID` 失敗 → 回 **500**，但預約**已寫入並佔住時段** |
| `GET /bookings`、`GET /bookings/:id` | 含該預約的列表 / 單筆 → 500 |
| `PATCH` / `DELETE /bookings/:id` | 兩者開頭都先 `repo.GetByID` → 500，**擁有者、管理員、System Admin 都無法取消或刪除** |
| `GET /resources/:id/availability` | `GetAvailability` 以 `repo.List` 撈該日全部預約，只要其中一筆屬於 NULL 名稱的使用者 → 整個端點 500，**對所有人** |

也就是：一個免費註冊的帳號，就能讓任一資源的可用時段查詢失效，並留下無法透過 API 清除的預約（需要直接改 DB）。

**重現步驟**

1. `POST /v1/auth/register`，`display_name` 傳 `"   "`（三個空白）。
2. 登入後對任一資源 `POST /v1/bookings` → 得到 500。
3. 任何人 `GET /v1/resources/{該資源}/availability?date=…` → 500。

**建議**

- 立即：booking repository 改用 `COALESCE(u.display_name, u.username)`（或掃進 `*string` 並在 DTO 層回退）。
- 治本：`display_name` 在 DTO 層加上「trim 後長度 ≥ 1」的驗證，或在 DB 加 `CHECK (display_name IS NULL OR length(btrim(display_name)) > 0)`；並盤點既有資料中是否已有 NULL。
- 補測試：註冊空白 display_name → 建預約 → 讀取 / 取消 / availability。
- 同類檢查：我閱讀了其他取 `display_name` 的掃描點（pickup 的 `HostDisplayName`、組織 / 場地管理員、收藏），皆為 `*string`，只有 booking 這兩處是 `string`；新增欄位時請留意。

---

### H-2　臨打訂單狀態機：報名者可推翻團主的「拒絕」

- **類型**：權限越界 / 狀態機繞過
- **位置**：`internal/pickup/service.go:592-601`（`updateOrder` 的 booker 分支）、`internal/pickup/repository.go:477-502`（`CreateOrder` 的既有訂單處理）
- **驗證**：靜態推導。現有測試（`tests/pickup_sports_test.go:203-225`）只覆蓋「rejected 後直接再報名會被擋」，沒有覆蓋下列路徑。

**成因**：一般報名者（`isOwner && !isReviewer`）只受兩條限制：

```go
if st == OrderStatusCancelled && (old == Confirmed || old == CancelRequest || payment == Done) { 需審核 }
if st != OrderStatusCancelled && st != OrderStatusCancelRequest { 禁止 }
```

兩條都**沒有檢查「目前狀態」是否允許離開**。於是：

- **路徑 A（rejected → cancelled → 重新報名）**：被拒絕者且未付款時，`PATCH status=cancelled` 通過（舊狀態是 rejected，不在第一條的攔截清單內）。訂單變成 `cancelled`，而 `CreateOrder` 對 `cancelled` 的既有訂單明確允許「就地重置」→ 再報名成功。`rejected` 的永久封鎖被兩步驟繞過。
- **路徑 B（rejected / cancelled → cancel_request）**：`PATCH status=cancel_request` 同樣通過；`cancel_request` 屬於佔位狀態，走 `UpdateOrderWithCapacityCheck`，只要還有名額就成功，報名者**重新佔住座位**，並向團主發出「申請取消」通知。

`docs/paths/pickup.yml` 對 rejected 的描述是「該使用者無法再次報名」，與實作不符。

**建議**：在 booker 分支加上來源狀態白名單，例如：

| 目前狀態 | booker 可移往 |
| --- | --- |
| pending | cancelled、cancel_request（未付款時 cancelled 才直接生效） |
| confirmed | cancel_request |
| cancel_request | （無，等待審核） |
| cancelled | （無，要重新報名請走 `POST`） |
| rejected | （無） |

同時補整合測試（A、B 兩條路徑 + 付款 done 的組合）。

---

### M-1　預約（booking）沒有「需審核才能取消」的機制，與臨打訂單不一致

- **位置**：`internal/booking/service.go:191-224`（時間變更）、`:226-241`（status）、`:262-293`（Delete）
- **驗證**：靜態推導。`role_system.md §6.5` 載明「預約本人可 DELETE」，所以刪除本身是已記載行為；以下是我認為值得重新確認的部分。

1. 預約本人可把**已確認、已付款**的預約直接設成 `cancelled`，`cancel_request`（申請取消）形同裝飾。臨打模組已有 `ErrCancellationRequiresReview`，booking 沒有同等規則。
2. 預約本人可直接 `DELETE` 已付款預約，等於抹除付款紀錄（硬刪除）。
3. 預約本人可改 `start_time` / `end_time`，**不受目前 status / payment_status 限制**：已確認且已付款的預約也可以自行改期，且可對已經開始或結束的預約延長 `end_time`（過去時間只檢查了「新的 start_time」）。
4. 預約本人可把 `cancelled` 改回 `cancel_request`（重新佔位；DB 排他約束會擋衝突，但無需管理員同意）。

**建議**：參照 `pickup.updateOrder` 的規則為 booking 補狀態機；已付款 / 已確認的預約改為只能 `cancel_request`，改期與刪除需管理階層。這是業務規則，請見 §6 Q1。

---

### M-2　公開端點無速率限制（暴力破解、帳號列舉、CPU 耗盡）

- **位置**：`internal/api/router.go`（無任何限流 middleware）、`internal/user/service.go`（登入 / 註冊）、`internal/config/config.go`（`BCRYPT_COST` 預設 12）
- **驗證**：靜態推導

- `POST /auth/login`、`POST /auth/register` 無限流、無鎖定機制。單次 bcrypt cost 12 約數百毫秒 CPU，未驗證者即可持續打滿 CPU（登入對不存在帳號也會做 dummy compare，所以無捷徑可省）。
- `POST /auth/register` 對重複 email / username 回 409，可列舉已註冊的 email。
- 登入端：`Login` 對「停用帳號」在 bcrypt 之前就回傳（`ErrInactiveUser`），雖然 handler 把錯誤訊息統一成 `invalid email or password`，但**回應時間**會暴露「帳號存在且已停用」。
- 註冊無任何成本，可大量建立帳號（搭配 M-6 的佔位濫用）。

**建議**：在 Cloudflare 層與應用層同時設限（至少針對 `/auth/*` 以 IP + email 做滑動視窗）；登入失敗加上漸進延遲或暫時鎖定；停用帳號的路徑也做一次 dummy compare。
**注意**：`gin.New()` 預設信任所有 proxy，`c.ClientIP()` 可被 `X-Forwarded-For` 偽造。要以 IP 限流前請先 `r.SetTrustedProxies(...)`（Cloudflare 後面建議改讀 `CF-Connecting-IP` 並驗證來源）。目前沒有程式使用 `ClientIP()`，但 `gin.Logger()` 會記錄偽造的 IP。

---

### M-3　`http.Server` 沒有任何逾時

- **位置**：`cmd/server/main.go:60-63`
- **驗證**：靜態推導

`&http.Server{Addr, Handler}` 未設定 `ReadHeaderTimeout` / `ReadTimeout` / `WriteTimeout` / `IdleTimeout`，Slowloris 類連線可長時間佔住連線。前面有 Cloudflare Tunnel 能緩解，但後端 `:8080` 在 `compose.dev.yml` 直接暴露；也不應把防護完全外包。

**建議**：至少設 `ReadHeaderTimeout: 10s`、`IdleTimeout: 60–120s`；`ReadTimeout` / `WriteTimeout` 需考慮 5MB 圖片上傳與串流下載後再定。

---

### M-4　帳號「刪除 / 停用」後，名下的預約、報名、團仍然有效

- **位置**：`internal/user/service.go:280-303`（`Delete`）、`:252-278`（`Update` 的 `IsActive`）
- **驗證**：靜態推導

`Delete` 只做 `is_active=false`（加清收藏與頭像）。被停用的使用者：

- 未來的**預約仍佔住時段**（排他約束只看 status，不看使用者是否啟用）。
- 臨打**訂單仍為 pending / confirmed，仍佔名額**，也仍計入 `current_enrolled` 與參與者統計。
- 若是團主，其**團仍 active 且出現在公開列表**，繼續接受報名，但團主本人已無法登入處理。
- 若是組織擁有者，組織沒有可用的 owner（只能由 System Admin 處理）。

另外 System Admin 可以把**自己**停用（只擋了「撤銷自己的 admin 旗標」）；`set_admin.sh` 只改 `is_system_admin`，不會把 `is_active` 改回來，最後一位管理員被停用時需手動下 SQL。

**建議**：停用 / 刪除流程在同一個交易內處理後續（取消未來的預約與訂單、停用其開的團、通知相關人），並禁止停用最後一位啟用中的 System Admin。

---

### M-5　`UpdateGroup` 的鎖與連線池可能互相卡死（理論上）

- **位置**：`internal/pickup/repository.go:70-90`（`WithGroupLock` 取得排他 advisory lock 並佔用一條連線）、`internal/pickup/service.go:239-250,317`（在鎖內呼叫 `validateSportAndSkillRange`）
- **驗證**：靜態推導，**未重現**

`WithGroupLock` 開交易、取得**全域**排他 advisory lock（`scheduleLockID`），之後在持鎖狀態下呼叫 `sportsService` / `skillLevelService`——它們使用的是**連線池的另一條連線**，不在該交易內。當同時有 ≥ 池上限（pgx 預設 `max(4, NumCPU)`）個請求都在修改 `sport_id` / `min_skill_level` / `max_skill_level` 時：持鎖者需要第二條連線，但連線都被「正在等鎖的交易」佔滿 → 互相等待，直到 request context 取消。專案沒有設定 statement / lock timeout，也沒有 server 逾時（見 M-3）。

此外，該全域排他鎖會在持鎖期間**阻擋全系統所有報名與訂單更新**（它們取 shared lock），任何一次團編輯的耗時都會直接變成全站報名延遲。

**建議**：把 sport / skill 的驗證移到取鎖之前（驗證完再進交易，鎖內只做寫入）；為 DB 連線設定 `lock_timeout` / `statement_timeout`；評估是否能把全域 advisory lock 縮小成「受影響使用者集合」的鎖。

---

### M-6　佔位濫用：無額度、無逾期釋放

- **位置**：`internal/booking/service.go:Create`、`internal/pickup/service.go:CreatePartyOrder`
- **驗證**：靜態推導（屬設計層風險）

- 任何已登入使用者可無限制建立 `pending` 預約；沒有未付款自動取消、沒有每人上限、沒有「最遠可預約日期」。惡意者可在營業時間內把所有時段排滿（精度可到毫秒，亦可切碎時段）。
- 臨打多人報名單筆最多 50 席，且在 `cancel_request` 狀態仍佔席位；注意「取消後可重新報名」會把原本的付款狀態重置為 pending。

**建議**：預約加上「每人同時有效預約數」「最遠預約天數」「時段需對齊（如 30 分鐘）」「pending 超時自動取消」；臨打視業務需求限制每人同期報名數。

---

### M-7　個資外洩面（email、電話）

1. `internal/pickup/http/handler.go:321`：單人報名時 `bookerName` 在沒有 `display_name` 時**回退為 email**。這個值會寫進訂單（團主可見）與給團主的通知內容（`「<email> 報名了…」`）。H-1 的修補若改成寫入空白名稱，這個回退仍會觸發。建議回退為 `username`。
2. `GET /pickup-groups/:id`（任何已登入者）回傳 `host.phone`。列表端點刻意不含電話，但詳情對所有登入者公開。請確認是否預期（Q5）。
3. 組織 / 場地的成員、管理員列表對管理階層回傳 email——屬設計，僅提醒。

---

### M-8　`GET /hosts/:host_id/pickup-groups` 會公開 `enable=false` 的團

- **位置**：`internal/pickup/http/handler.go:124-178`
- **驗證**：靜態推導

公開列表 `GET /pickup-groups` 設了 `PubliclyVisibleOnly`（active、enable、未結束），但 `ListGroupsByHost` 沒有，所以**未登入者**可以列出該團主所有的團，包含 `enable=false`（被停用 / 隱藏）與已取消 / 已結束的。OpenAPI 的描述允許 `status` 篩選，所以顯示已取消 / 已完成可能是預期；`enable=false` 是否應公開請確認（Q5）。建議至少對非本人 / 非 admin 的請求固定排除 `enable=false`。

---

### M-9　外鍵違規 / 找不到資料被當成 500

這些都不會洩漏內部資訊（`response.Error` 會遮蔽細節），但回傳錯誤的狀態碼，也讓客戶端無法分辨「我送錯了」與「伺服器壞了」。

| 位置 | 情境 | 現況 | 應為 |
| --- | --- | --- | --- |
| `internal/location/repository.go:284`（`AddLocationManager`） | 指派**非組織成員**為場地管理員 | `location_managers_member_fkey` 違規未映射 → **500**（即 role_system.md §8.6，尚未修） | 400 / 409，參照 `internal/user/repository.go` 的 FK 映射 |
| `internal/pickup/repository.go:162`（`CreateGroup`）、`:325`（`UpdateGroup`） | `location_id` 不存在 | `pickup_groups_location_id_fkey` 違規未映射；handler 只驗 `uuid` 格式 → **500** | 400 / 404 |
| `internal/file/repository.go:67` | 檔案 ID 不存在，或不是合法 UUID | `fmt.Errorf` 包住 `pgx.ErrNoRows`，`file.ErrNotFound`（已定義）**從未被使用** → **500** | 404；路由參數加 `uuid` 驗證 |
| `internal/location/service.go:Create`、`internal/resource/service.go:Create` | 任何 `GetByID` 錯誤 | 一律改寫成 404 / 400，連 DB 故障也被偽裝 | 只轉換 not-found，其餘照傳 |

---

### M-10　設定面的陷阱

- **`APP_ENV` 必須剛好等於 `"prod"`**（`internal/config/config.go:45`）才是正式模式。任何其他值（`production`、`prd`、打錯字）都會**靜默**進入開發模式：CORS 對所有來源放行、Gin 使用 debug 模式。`.env.example` 寫的是 `APP_ENV=local`。建議白名單驗證（`dev` / `prod`），其他值直接啟動失敗，或預設反過來（預設 prod）。
- **正式模式下 `PROD_ORIGINS` 為空**：`strings.Split("", ",")` 得到 `[""]`，`cors.New` 會因為不合法 origin 而 **panic**（fail-closed，但是以 panic 收場）。建議啟動時給出明確錯誤。
- **範例設定無法啟動**：`.env.example` 的 `JWT_SECRET=jwt_secret`（10 bytes）、README §106 的 `your-super-secret-key`（21 bytes）都短於程式強制的 32 bytes，照抄會在 `config.Load` 失敗。請更新範例並註明產生方式（如 `openssl rand -base64 48`）。
- `compose.yml` 把 `TEST_DB_DSN` / `TEST_JWT_SECRET` 傳給正式 backend 容器，用不到且擴大憑證暴露面，建議移除。
- `postgres:latest`、`cloudflare/cloudflared:latest`、`alpine:latest`、`golang:alpine` 未固定版本；`postgres:latest` 在大版本升級時可能導致資料目錄不相容。建議固定資料庫主版本（例如 `postgres:17`），避免重建容器時被動升級。
- `compose.dev.yml` 對外開 `5432` 並使用範例弱密碼——僅限開發機，請確認不會被用在有公網位址的主機上。

---

## 3. 發現（Low）

### L-1　JWT 與工作階段的強化建議

- `ParseAndValidate` 只檢查 `*jwt.SigningMethodHMAC`，會接受 HS256/384/512；建議 `jwt.WithValidMethods([]string{"HS256"})`、`jwt.WithExpirationRequired()`，並設 `iss` / `aud` 後驗證。現況無法被偽造（需要密鑰），僅屬縱深防禦。
- `AuthOptional` 不檢查帳號是否仍啟用（`AuthRequired` 有），影響只是公開列表的 `enrolled_status` 個人化，風險極低。
- **沒有「修改密碼」「忘記密碼」「登出 / 撤銷」功能**；JWT 無法撤銷，帳號被盜時只能等 token 過期（預設 15 分鐘、範例檔 1 小時）或由管理員停用帳號。屬功能缺口，不是 bug。

### L-2　上傳資源耗用

- `internal/file/service.go:Upload` 先把整個檔案讀進記憶體（≤ 5MB），才排隊等 `imageSlots`（容量 2）解碼。大量並行上傳時，排隊中的請求各自持有緩衝區；且同一張圖會各自解碼兩次（1000px 與 200px 縮圖）。上傳只能透過頭像 / 封面路由（有權限且會取代舊檔），沒有通用上傳端點，所以不會無限累積孤兒檔案，但仍建議對上傳路由加上每人速率限制。
- 檔案類型白名單（僅 jpeg / png）與 magic-bytes 比對有效擋住了 TIFF 等其他格式，因此 §4 的 `x/image/tiff` 漏洞在目前流程下**不可達**；仍建議升級依賴。
- 場地 / 資源以 `ON DELETE CASCADE` 硬刪除時，資源的封面檔案不會一併清理（`files` 資料列與磁碟檔案成為孤兒）。

### L-3　驗證規則造成的行為異常（已實證）

- `UpdateRequest.SportID` 使用 `omitempty,uuid`，空字串**一定被 400 拒絕**；但 `resource/service.go:123-128` 與註解明確寫著「空字串代表清除 sport」——這條路從 HTTP 進不來，無法清除資源的 sport。同理，`phone` 的 `omitempty,tw_phone` 讓使用者無法把電話清空。
- `Longitude` / `Latitude` 用了 `required`，數值 `0`（赤道 / 本初子午線，合法座標）會被當成缺失而回 400。建議改成指標型別 + `required`。
- 場地 `PATCH` 的 `name` 只驗 `min=1`，`" "` 可通過且不會被 trim（`Create` 有 trim，`Update` 沒有）；資源 `Update` 檢查了空白但存的是未 trim 的值。

### L-4　讀取—修改—寫入沒有鎖（丟失更新）

- `booking.Update`、`location.UpdateCover/RemoveCover`、`resource.UpdateCover/RemoveCover` 都是先 `GetByID` 再整列 `UPDATE`。例如場地管理員 `PATCH` 場地資料的同時另一人上傳封面，後寫入者會以舊值覆蓋前者的欄位；預約的「擁有者取消」與「管理員改付款狀態」同時發生時亦同。
- 建議：封面更新只更新 `cover` 欄位；預約更新用 `SELECT … FOR UPDATE` 或樂觀鎖（`updated_at` 條件）。

### L-5　錯誤處理與可觀測性

- `internal/pkg/response/error.go:27` 的註解自己承認「應該在這裡記錄內部錯誤」：所有 500 的真實原因目前**完全不會出現在日誌**，線上除錯幾乎不可能。建議在 `response.Error` 對非 `AppError` 記錄 `err`（含 request id）。
- 多數 handler 在 400 時回傳 `"details": err.Error()`，內容是 validator 的原文（含 Go struct 名與欄位名，如 `Key: 'RegisterRequest.Email' …`）。不嚴重，但建議轉成對使用者友善、不洩漏內部命名的訊息。
- 幾處錯誤被吞掉：`user.Login` 的 `UpdateLastLogin`、各處 `_ = fileService.Delete(...)`（檔案清理失敗無日誌，孤兒檔案無法察覺）。

### L-6　效能與分頁

- 每個已驗證請求都會呼叫 `UserService.GetByID`，該查詢含兩層關聯子查詢與 `json_agg`（組織成員資料）；`RequireSystemAdmin`、handler、`IsOwnerOrAbove` 又各自再查一次，同一個請求可能查 3–5 次。建議在 middleware 把使用者（至少 `is_active` / `is_system_admin`）放進 `gin.Context` 重用，並讓「只需旗標」的查詢走輕量版本。
- `GetOrdersByUserID`、通知、收藏等清單沒有分頁 / 保留期限，長期會無限成長；`notifications` 也沒有清理機制。
- 多個清單（組織、場地、資源、預約）排序沒有穩定的次要鍵，分頁時相同排序值的列可能重複或遺漏（pickup 與 notifications 已用 `id` 當 tiebreaker，可比照）。
- `ILIKE '%' || 使用者輸入 || '%'` 沒有跳脫 `%` / `_`，使用者可注入萬用字元（非 SQL 注入，僅改變匹配語意，且多為管理端點）。深分頁（`page` 最大 100000 × `page_size` 100）會造成大 OFFSET 掃描。
- `GetAvailability` 未傳 `date` 時使用伺服器時區的「今天」，而非場地時區；`startOfDay.Add(24h)` 在有夏令時間的時區當天不是 24 小時（台灣無夏令，僅提醒）。

### L-7　其他小項

- 組織建立 / 轉移擁有者時，沒有檢查新擁有者帳號是否仍啟用。
- `organization.repository.ListOrganizationManagers` 的排序邏輯有一段被後面 `switch` 完全覆蓋的死碼（`"u." + filter.SortBy`），建議清理，避免日後有人誤以為它是白名單之外的路徑。
- 場地、資源、臨打團使用硬刪除（`DELETE`），與 AGENTS.md「主要實體偏好軟刪除」不一致；目前由 FK `RESTRICT` 擋住有關聯資料的情況，是可接受的，但請確認是否為刻意。
- 場地營業時間不支援跨午夜（`end` 必須大於 `start`），夜間場館無法表示；若有此需求需擴充模型。
- `DELETE /favorites/host` 使用 request body，部分 proxy / client 會丟棄 DELETE 的 body，建議改成路徑參數。

---

## 4. 依賴與工具鏈（`govulncheck`）

| ID | 位置 | 說明 | 目前 | 修復版 | 是否可達 |
| --- | --- | --- | --- | --- | --- |
| GO-2026-5970 | `golang.org/x/text` | 非法輸入造成無窮迴圈 | v0.37.0 | v0.39.0 | 經 `pgxpool` 可達 |
| GO-2026-5066 / 5062 | `golang.org/x/image/tiff` | 越界 panic / 無限制 tile | v0.41.0 | v0.43.0 | 工具回報可達，但上傳白名單只允許 jpeg / png，**實際不可達** |
| GO-2026-6089 / 6090 / 6088 / 5972 | Go 標準庫 `net/http`、`crypto/tls`、`encoding/xml`、`encoding/asn1` | — | go1.26.5（本機工具鏈） | go1.26.6 | 取決於建置映像：`Dockerfile` 使用浮動的 `golang:alpine`，重新建置即可取得修補版；本機請升級 Go |

建議：`go get golang.org/x/text@v0.39.0 golang.org/x/image@v0.43.0 && go mod tidy`，並把 `govulncheck` 加入 CI。`go.mod` 的 `go 1.25.1` 與 Dockerfile 的浮動標籤也請固定（例如 `golang:1.26-alpine`）。

---

## 5. 做得好的地方（請保留）

- **SQL 全程參數化**（Squirrel + pgx），排序欄位都以白名單 `oneof` + repository 內部映射，沒有發現 SQL 注入面。
- **登入防護**：對不存在帳號做 dummy bcrypt compare；`AuthRequired` 每個請求都檢查 `is_active`，停權即時生效；JWT secret 強制 ≥ 32 bytes。
- **併發控制**：預約有 `EXCLUDE USING gist` 排他約束作最後防線（並映射為 409）；臨打報名有 `FOR UPDATE` + 固定鎖序（shared advisory → user → group → order）；組織角色互斥用 `FOR UPDATE` 序列化。
- **上傳安全**：大小以實際內容為準、magic-bytes 與宣告型別比對、白名單、強制重新編碼為 JPEG（剝除 EXIF 與嵌入內容）、像素與尺寸上限、解碼並行上限、`nosniff`、Content-Disposition 檔名過濾。
- **請求本體限制**（10 MiB）、手動通知有每分鐘速率限制與收件人驗證、DTO 與領域模型分離（`UpdateMeRequest` 不含特權欄位，沒有 mass-assignment 問題）。
- **權限檢查**：每個管理類路由都有明確的 Owner / Manager / Location Manager 判斷，且大多先 `CheckOperation` 擋住停用組織；組織角色以 DB 複合外鍵保證租戶隔離（`location_managers` 不可能掛到別的組織的場地）。
- Dockerfile 使用 non-root 使用者，多階段建置；`.env` 已被 gitignore 與 dockerignore。

---

## 6. 待確認問題（需要你的決定）

| # | 問題 | 影響的發現 |
| --- | --- | --- |
| Q1 | 預約的取消 / 改期規則：已確認或已付款的預約，使用者本人是否應只能「申請取消」並由管理階層核准（比照臨打訂單）？預約本人是否仍可 `DELETE` 已付款預約？ | M-1 |
| Q2 | 被團主 `rejected` 的人，是否**永遠**不得再報名同一個團？（目前文件說是，實作不是。） | H-2 |
| Q3 | Location Manager 是否應該能管理自己場地的資源與預約？目前只有封面與場地資料。 | role_system.md §8.2 |
| Q4 | Organization Manager 是否需要能以 `organization_id` 列出自己組織的預約？ | role_system.md §8.3 |
| Q5 | 隱私邊界：(a) `GET /pickup-groups/:id` 回傳團主電話給所有登入者是否預期？(b) `enable=false` 的團是否該出現在 `/hosts/:id/pickup-groups`？(c) 報名者沒有顯示名稱時，是否可以改用 `username` 而非 email？ | M-7、M-8 |
| Q6 | 帳號停用 / 刪除時，名下未來的預約、臨打報名、開的團希望怎麼處理（自動取消並通知 / 保留）？是否要禁止停用最後一位管理員？ | M-4 |
| Q7 | 是否接受為預約新增「每人同時有效預約數」「最遠預約天數」「未付款逾時自動取消」？數值由你決定。 | M-6 |
| Q8 | 報告存放位置：AGENTS.md 寫的是 `./ref-only/code-review-{idx}.md`（git 忽略），你這次指定 `docs/code-review.md`（會被提交）。後續輪次要沿用哪一個？ | — |

---

## 7. 建議修復順序

1. **H-1**（NULL display_name）：修查詢 + 加驗證 + 補測試，並檢查正式資料庫是否已有 NULL 的 `display_name` 與卡住的預約。
2. **H-2**（訂單狀態機）：加來源狀態白名單 + 兩條繞過路徑的測試。
3. **M-3**（server 逾時）、**M-2**（登入 / 註冊限流）、**M-10**（`APP_ENV` 驗證與範例設定）：改動小、風險高、可以一起做。
4. **M-9**（FK / not-found 映射）與 **L-5**（記錄 500 的內部錯誤）：能直接提升可維運性，後者建議越早越好，其餘修復時才能看到線上錯誤。
5. **M-4、M-1、M-6、M-7、M-8**：待 Q1–Q7 決定後實作。
6. **M-5**、§4 依賴升級、Low 項目依序處理。

---

## 附錄 A：審查時追蹤過的權限矩陣（摘要）

| 端點群 | 存取條件 | 審查結論 |
| --- | --- | --- |
| `/users`、`/pickup-hosts`、`/organizations`（POST/PATCH/DELETE）、`/announcements`（寫）、`/sports`、`/skill-levels`（寫）、`/notifications`（POST） | `authMiddleware` + `RequireSystemAdmin` | 路由層即擋下，OK |
| `/users/:id/avatar` | 本人或 System Admin（handler 內檢查） | OK |
| `/organizations/:id/managers|members` 寫入 | Owner 以上（先 `CheckOperation`） | OK |
| `/locations` 建立 / 刪除 / 管理員指派 | Org Manager 以上 | OK；錯誤訊息寫「only owners」但實際允許 Manager，訊息過時 |
| `/locations/:id` 更新 / 封面 | Location Manager 以上 | OK |
| `/resources` 建立 / 更新 / 刪除 | Org Manager 以上；封面為 Location Manager 以上 | 兩者門檻不同，請確認是否預期（Q3） |
| `/bookings` | 列表僅限自己（admin 例外）；單筆：本人 / Org Manager / admin | OK，缺口見 Q4、M-1 |
| `/pickup-groups` 讀取 | 公開（簡化欄位）；詳情需登入；訂單需團主或 admin | OK，見 M-7、M-8 |
| `/pickup-groups/:id` 更新 | 團主或 admin | OK |
| `/pickup-orders/:id` 更新 | 本人（受限）或團主 / admin | 本人分支有缺口（H-2） |
| `/pickup-groups/:id/ratings` | 團主或 admin；且團已結束、對象為 confirmed 參與者、不可自評 | OK |
| `/files/:id` | 任何已登入者 | 已記載於 role_system.md §8.5 |

## 附錄 B：未涵蓋 / 後續建議

- 沒有執行整合測試，也沒有在實際資料庫上重現 H-1、H-2；建議修復前先以測試固定行為。
- 沒有模糊測試與負載測試（M-2、M-3、M-5 建議以壓測驗證）。
- 前端如何使用 `Authorization`（儲存位置、XSS 風險）不在範圍內；由於後端只用 Bearer header，沒有 CSRF 面。
