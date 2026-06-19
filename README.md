# Court Booking Backend 球場預約系統後端

這是一個基於 Go 開發的場地預約系統後端 API。專案採用**分層架構**設計，具備完整的身份驗證、角色權限管理以及組織／場地管理功能。

## 🛠 技術棧

- **Language:** Go 1.21+
- **Web Framework:** [Gin Web Framework](https://github.com/gin-gonic/gin)
- **Database:** PostgreSQL 18
- **Database Driver:** [pgx v5](https://github.com/jackc/pgx) (High performance PostgreSQL driver)
- **Authentication:** JWT (JSON Web Tokens) + Bcrypt
- **Configuration:** godotenv
- **Documentation:** OpenAPI v3 / Swagger UI
- **Infrastructure:** Docker & Docker Compose

## 🏗 系統架構設計

本專案遵循**分層架構**，將關注點分離，確保程式碼的可測試性與維護性。依賴關係由外向內（Handler -> Service -> Repository），並透過 `internal/app/container.go` 進行**依賴注入**。

### 分層說明

1.  **HTTP Layer (`internal/*/http`)**
    - 負責處理 HTTP 請求與回應。
    - 定義 Request/Response DTO (Data Transfer Object)。
    - 驗證輸入資料，將請求轉發給 Service 層。
    - 不包含商業邏輯，僅處理路由與資料格式轉換。

2.  **Service Layer (`internal/*/service.go`)**
    - 核心商業邏輯層。
    - 處理業務規則（如：檢查密碼強度、驗證預約時間是否重疊）。
    - 呼叫 Repository 獲取資料。
    - **與 HTTP 無關**：此層不知道它是被 API 呼叫還是 CLI 呼叫。

3.  **Repository Layer (`internal/*/repository.go`)**
    - 資料存取層 (Data Access Layer)。
    - 使用 `pgx` 執行 Raw SQL 與資料庫互動。
    - 負責將資料庫 Row 映射為 Go Struct (Model)。

4.  **Model (`internal/*/model.go`)**
    - 定義核心領域物件 (Domain Entities)。
    - 定義 Enum 與過濾器結構 (Filter)。

### 目錄結構

```text
.
├── cmd/server/         # 應用程式入口 (Main entry point)
├── db/                 # 資料庫 schema 與初始化腳本
├── docs/               # API 文件 (OpenAPI/Swagger)
├── internal/           # 私有應用程式代碼
│   ├── api/            # 全局 API 設定 (Router, Middleware)
│   ├── app/            # 依賴注入容器 (Dependency Container)
│   ├── config/         # 設定檔讀取邏輯
│   ├── pkg/            # 共用工具 (如 Response wrapper)
│   └── [modules]/      # 業務模組 (user, auth, booking, organization, file...)
├── tests/              # 整合測試 (Integration Tests)
├── compose.yml         # Docker Compose (DB, Swagger & Cloudflare Tunnel)
└── .env                # 環境變數 (需自行建立)
```

## ✨ 功能特性

- **使用者系統 (User System)**
  - 註冊／登入 (JWT Token)。
  - 系統管理員 (System Admin) 與一般使用者區分。
- **多租戶組織架構 (Organizations)**
  - 支援建立多個組織 (Organization)。
  - 組織內角色權限：Owner (擁有者), Manager (管理員)。
- **場館與資源管理**
  - 場館 (Locations)：支援經緯度、營業時間。
  - 資源 (Resources)：球場、會議室等具體可預約單位。
- **預約系統 (Booking)**
  - 時段預約，防止時間重疊衝突。
  - 預約狀態流轉 (Pending -> Confirmed/Cancelled)。
- **安全性**
  - 密碼加密存儲 (Bcrypt)。

## 🚀 快速開始

### 前置需求

- Go (1.20+)
- Docker & Docker Compose

### 安裝與運行

1.  **Clone 專案**

    ```bash
    git clone https://github.com/nekogravitycat/court-booking-backend.git
    cd court-booking-backend
    ```

2.  **設定環境變數**

    複製範例設定檔並修改：

    ```bash
    cp .env.example .env
    ```

    確保 `.env` 內容包含正確的 DB 連線字串：

    ```dotenv
    DB_DSN=postgres://user:password@localhost:5432/court_booking?sslmode=disable
    JWT_SECRET=your-super-secret-key
    ```

    若要透過 Cloudflare Tunnel 對外發佈服務，另需設定 `CLOUDFLARE_TUNNEL_TOKEN`（見下方「對外發佈」一節）。

3.  **啟動資料庫與 Swagger**

    使用 Docker Compose 啟動 PostgreSQL 和 Swagger UI：

    ```bash
    docker compose up -d
    ```

    這會啟動 PostgreSQL（資料表結構由應用程式啟動時自動套用 migration 建立，見下）。

4.  **運行應用程式**

    ```bash
    go run cmd/server/main.go
    ```

    應用程式啟動時會先用 golang-migrate 套用 `db/migrations/` 下所有尚未執行的 migration（已內嵌於 binary），再開始服務。伺服器將啟動於 `http://localhost:8080`。

## 🌐 對外發佈 (Cloudflare Tunnel)

`compose.yml` 內建 `cloudflared` 服務，透過 Cloudflare Tunnel 對外發佈。所有服務共用同一個 Docker 網路，`backend` 與 `swagger-ui` **不再對 host 暴露任何 port**，外部流量一律經由 Cloudflare 邊緣節點 → `cloudflared`（僅向外建立連線）→ 內部 Docker 網路抵達各服務，全部封裝在同一個 Compose 內。

1.  **設定 Tunnel Token**

    在 `.env` 中填入 Cloudflare Zero Trust 提供的 Tunnel Token（此檔已被 `.gitignore` 忽略，密鑰不會進版控）：

    ```dotenv
    CLOUDFLARE_TUNNEL_TOKEN=your_cloudflare_tunnel_token
    ```

2.  **設定 Public Hostname 路由**

    於 Cloudflare Zero Trust Dashboard 對應的 Tunnel 設定中，將 Public Hostname 的 Service 指向**內部 Docker 服務名稱**（`cloudflared` 在同一網路內可直接解析）：

    | Public Hostname        | Service                  |
    | ---------------------- | ------------------------ |
    | `api.yourdomain.com`   | `http://backend:8080`    |
    | `swagger.yourdomain.com` | `http://swagger-ui:8080` |

3.  **啟動**

    ```bash
    docker compose up -d
    ```

> 注意：由於 host port 已關閉，本機開發若需直接存取 `:8080` / `:8081`，可在 `compose.yml` 暫時加回對應的 `ports` 設定，或直接以 `go run cmd/server/main.go` 在本機執行。

## 🗄 資料庫設計

- **Migration 檔案**：`db/migrations/`（`{version}_{name}.up.sql` / `.down.sql`，採 golang-migrate 慣例）。
- **初始化與變更**：schema 由 migration 管理，內嵌於 binary。應用程式與測試啟動時自動套用尚未執行的 migration，毋需手動執行。版本記錄於 `schema_migrations` 表。
- **核心表格**：
  - `users`：平台使用者。
  - `organizations`：頂層組織單位。
  - `organization_members` / `managers`：組織成員與權限管理。
  - `locations`：實體場館。
  - `resources`：可預約的單一資源。
  - `bookings`：預約紀錄。
  - `files`：檔案上傳紀錄 (Avatar, Cover)。
  - `announcements`：系統公告。

### 從 host 直接操作資料庫

`db` 服務基於安全考量**預設不對 host 暴露 port**，因此有兩種連線方式：

#### 方法 A：透過容器執行（推薦，毋需開 port）

直接在執行中的 DB 容器內開 `psql`，連線流量完全留在容器內：

```bash
# 進入互動式 psql
docker compose exec db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB"

# 或直接執行單一查詢
docker compose exec db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "SELECT * FROM users LIMIT 10;"
```

> `$POSTGRES_USER` / `$POSTGRES_DB` 為 `.env` 中設定的值；若 shell 未載入這些變數，請改填實際值。

#### 方法 B：對 host 暴露 port（供 GUI 工具如 DBeaver、TablePlus 連線）

若需用本機的 GUI 客戶端連線，可在 `compose.yml` 的 `db` 服務加上 port 對應：

```yaml
db:
  # ...
  ports:
    - "5432:5432"
```

重新啟動容器後即可從 host 以 `localhost:5432` 連線：

```text
Host:     localhost
Port:     5432
Database: <POSTGRES_DB>
User:     <POSTGRES_USER>
Password: <POSTGRES_PASSWORD>
```

> 注意：暴露 `5432` 等同把資料庫開放給本機網路，請僅在開發環境使用，正式環境建議維持不開 port、改用方法 A。

## 📖 API 文件

本專案使用 OpenAPI (Swagger) 3.0 規範。

1.  **瀏覽器預覽**：
    啟動 Docker Compose 後，訪問 http://localhost:8081

2.  **原始檔案**：
    位於 `docs/openapi.yml`。

3.  **權限系統說明**：
    開發者可參考 `docs/role_system.md`，內含詳細的角色權限設計說明。

## 🧪 測試

專案包含整合測試，位於 `tests/` 目錄下。測試會連接測試用資料庫以確保邏輯正確且不影響實際資料庫。

1.  **準備測試資料庫**：
    `compose.yml` 預設配置了 `POSTGRES_TEST_DB`，啟動容器時會自動建立測試庫。

2.  **執行測試**：

    ```bash
    # 確保設置了測試環境變數 (通常 main_test.go 會嘗試讀取 ../.env)
    go test ./tests/... -v
    ```

## 📋 開發規範

### 1. 程式碼風格

- **格式化**：嚴格遵守 Go 標準格式 (`go fmt`)。
- **註解**：所有註解必須使用**英文**撰寫。
- **禁止 Emojis**：程式碼與註解中不得出現 Emoji。

### 2. 錯誤處理架構

為確保系統穩定性並提供一致的 API 錯誤回應，本專案採用統一的錯誤處理策略。

#### A. Model 層 (`internal/*/model.go`)

- **定義錯誤**：使用 `apperror.New` 定義業務邏輯錯誤，並直接關聯 HTTP 狀態碼。
- **範例**:
  ```go
  var (
    ErrNotFound      = apperror.New(http.StatusNotFound, "resource not found")
    ErrNameRequired  = apperror.New(http.StatusBadRequest, "name is required")
    ErrOrgIDRequired = apperror.New(http.StatusBadRequest, "organization_id is required")
  )
  ```

#### B. Service 層 (`internal/*/service.go`)

- **回傳錯誤**：業務邏輯檢查失敗時，直接回傳 Model 層定義的錯誤變數。
- **系統錯誤**：底層系統錯誤（如 DB 連線失敗）應直接回傳，Handler 層會將其視為 500 Internal Server Error。
- **範例**:
  ```go
  if name == "" {
    return ErrNameRequired
  }
  ```

#### C. Handler 層 (`internal/*/http/handler.go`)

- **統一回應**：使用 `response.Error(c, err)` 輔助函式處理所有錯誤回應。
- **自動映射**：`response.Error` 會自動判斷錯誤類型：
  - 若是 `AppError`，則使用定義的狀態碼與訊息回傳。
  - 若是其他錯誤，則回傳 `500 Internal Server Error` 並隱藏內部細節。
- **範例**:
  ```go
  if err := h.service.Delete(ctx, id); err != nil {
    response.Error(c, err)
    return
  }
  ```

### 3. 架構分層職責

- **Handler Layer (`http`)**:
  - 負責解析 HTTP Request (Body, Query, Param)。
  - 負責權限檢查 (Middleware 或 Service 輔助)。
  - **不包含業務邏輯**。
  - 負責將 Service 回傳的 Go error 映射為 HTTP Status Code。
- **Service Layer**:
  - 核心業務邏輯中心。
  - 負責跨模組的邏輯串接 (e.g., Booking Service 呼叫 Location Service)。
  - **不包含 HTTP 相關依賴** (如 `gin.Context`)。
- **Repository Layer**:
  - 負責 Raw SQL 執行與資料庫互動。
  - 負責將 SQL Row Scan 轉為 Go Struct。
  - 使用 `pgx` driver。

### 4. 資料庫規範

- **Raw SQL**：本專案不使用 ORM，請撰寫乾淨的 SQL 語句。
- **Soft Delete**：對於主要實體（Organization, User 等），優先採用 `is_active` 機制，避免實體資料刪除。
- **Schema**：變更需新增一個 migration（`db/migrations/{下一個版號}_{描述}.up.sql` 及對應 `.down.sql`），勿直接修改既有 migration。

### 5. API 回應格式

- **成功**：回傳 JSON 物件。
- **列表**：必須包含分頁資訊。
  ```json
  {
    "items": [],
    "page": 1,
    "page_size": 20,
    "total": 100
  }
  ```
- **錯誤**：必須回傳 `{"error": "description"}` 格式。
