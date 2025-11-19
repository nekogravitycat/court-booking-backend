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
│   ├── auth/           # 認證模組 (JWT, Password Hashing)
│   ├── config/         # 設定檔讀取邏輯
│   ├── db/             # 資料庫連線池封裝
│   ├── organization/   # [模組] 組織、場館、成員管理
│   ├── user/           # [模組] 使用者管理
│   └── pkg/            # 共用工具 (如 Response wrapper)
├── tests/              # 整合測試 (Integration Tests)
├── compose.yml         # Docker Compose (DB & Swagger)
└── .env                # 環境變數 (需自行建立)
```

## ✨ 功能特性

- **使用者系統 (User System)**
  - 註冊／登入 (JWT Token)。
  - 系統管理員 (System Admin) 與一般使用者區分。
- **多租戶組織架構 (Organizations)**
  - 支援建立多個組織 (Organization)。
  - 組織內角色權限：Owner (擁有者), Admin (管理員), Member (成員)。
- **場館與資源管理**
  - 場館 (Locations)：支援經緯度、營業時間。
  - 資源 (Resources)：球場、會議室等具體可預約單位。
- **預約系統 (Booking)**
  - 時段預約，防止時間重疊衝突。
  - 預約狀態流轉 (Pending -> Confirmed/Cancelled)。
- **安全性**
  - 密碼加密存儲 (Bcrypt)。
  - 軟刪除機制，保留歷史數據。

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

3.  **啟動資料庫與 Swagger**

    使用 Docker Compose 啟動 PostgreSQL 和 Swagger UI：

    ```bash
    docker compose up -d
    ```

    這會自動執行 `db/schema.sql` 初始化資料庫表格。

4.  **運行應用程式**

    ```bash
    go run cmd/server/main.go
    ```

    伺服器將啟動於 `http://localhost:8080`。

## 🗄 資料庫設計

- **Schema 檔案**：`db/schema.sql`
- **初始化**：當 Docker 容器首次啟動時，PostgreSQL 映像檔會自動執行 `/docker-entrypoint-initdb.d` 下的 SQL 檔案。
- **核心表格**：
  - `users`：平台使用者。
  - `organizations`：頂層組織單位。
  - `organization_permissions`：連結 User 與 Organization 的權限表 (Role)。
  - `locations`：實體場館。
  - `resources`：可預約的單一資源。
  - `bookings`：預約紀錄。

## 📖 API 文件

本專案使用 OpenAPI (Swagger) 3.0 規範。

1.  **瀏覽器預覽**：
    啟動 Docker Compose 後，訪問 http://localhost:8081

2.  **原始檔案**：
    位於 `docs/openapi.yml`。

3.  **API 整合指南**：
    前端開發者可參考 `docs/api.md`，內含詳細的認證流程與回傳格式說明。

## 🧪 測試

專案包含整合測試，位於 `tests/` 目錄下。測試會連接測試用資料庫以確保邏輯正確且不影響實際資料庫。

1.  **準備測試資料庫**：
    `compose.yml` 預設配置了 `POSTGRES_TEST_DB`，啟動容器時會自動建立測試庫。

2.  **執行測試**：

    ```bash
    # 確保設置了測試環境變數 (通常 main_test.go 會嘗試讀取 ../.env)
    go test ./tests/... -v
    ```

### 開發規範

- **Git Commit**：請使用英文撰寫 Commit Message。
- **程式碼風格**：符合 Go 標準 (`go fmt`)。註解使用英文撰寫，不得使用 Emoji。
- **錯誤處理**：盡量在 Service 層回傳具體的 `error` 變數 (如 `ErrNotFound`)，由 Handler 層決定 HTTP Status Code。
