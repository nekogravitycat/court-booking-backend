# 球場預約系統 (Court Booking System) - API 整合指南

這份文件旨在協助前端開發者快速理解後端 API 的架構、認證方式以及資料互動格式。

## 1. 基礎資訊 (General Info)

  * **Base URL (Local):** `http://localhost:8080/v1`
  * **Content-Type:** `application/json`
  * **日期時間格式:** ISO 8601 (e.g., `2023-11-19T10:00:00Z`)

## 2. 認證機制 (Authentication)

本系統使用 **JWT (JSON Web Token)** 進行身份驗證。

### 流程

1.  呼叫 `POST /auth/login` 取得 `access_token`。
2.  在後續所有需要權限的 API 請求中，將 Token 放入 HTTP Header。

### Header 格式

```http
Authorization: Bearer <your_access_token>
```

> **注意**: 如果 Token 過期或無效，API 將返回 `401 Unauthorized`。

## 3. 通用回應格式 (Response Format)

### 成功 (Success)

通常直接回傳資料物件，或是包裝在特定結構中（如分頁）。

### 錯誤 (Error)

當發生錯誤（4xx, 5xx）時，回應 Body 會包含一個 `error` 欄位說明原因。

```json
{
  "error": "invalid email or password"
}
```

### 分頁格式 (Pagination)

列表類型的 API 統一採用以下結構：

```json
{
  "items": [ ... ],      // 資料陣列
  "page": 1,             // 目前頁碼
  "page_size": 20,       // 每頁筆數
  "total": 100           // 總資料筆數 (供前端計算總頁數)
}
```

**通用查詢參數 (Query Params):**

  * `page`: 頁碼 (預設 1)
  * `page_size`: 每頁數量 (預設 20)

## 4. API 資源詳解

### A. 認證與個人資訊 (Auth & Me)

一般使用者登入與註冊的入口。

| Method | Path | 描述 | 關鍵參數 /備註 |
| :--- | :--- | :--- | :--- |
| **POST** | `/auth/register` | 註冊新帳號 | Body: `{email, password, display_name}` |
| **POST** | `/auth/login` | 登入 | Body: `{email, password}` <br> Response: `{access_token, user}` |
| **GET** | `/me` | 取得目前使用者資料 | 需帶 Token |

### B. 使用者管理 (Users) - System Admin Only

系統管理員用來查詢與管理平台所有使用者。

| Method | Path | 描述 | 篩選參數 (Query) |
| :--- | :--- | :--- | :--- |
| **GET** | `/users` | 使用者列表 | `email`, `display_name`, `is_active`, `sort` |
| **GET** | `/users/{id}` | 取得特定使用者 | - |
| **PATCH** | `/users/{id}` | 更新使用者狀態 | Body: `{display_name, is_active, is_system_admin}` |

### C. 組織管理 (Organizations)

管理場館擁有者（品牌/組織）。

  * **讀取**: 登入使用者皆可。
  * **寫入**: 目前限制為 System Admin。

| Method | Path | 描述 | 備註 |
| :--- | :--- | :--- | :--- |
| **GET** | `/organizations` | 組織列表 | 僅列出 `is_active=true` 的組織 |
| **POST** | `/organizations` | 建立組織 | Body: `{name}` |
| **GET** | `/organizations/{id}` | 取得單一組織 | - |
| **PATCH** | `/organizations/{id}` | 更新組織資訊 | Body: `{name}` |
| **DELETE** | `/organizations/{id}` | 刪除組織 | 軟刪除 (Soft Delete) |

### D. 組織成員 (Members)

管理特定組織底下的成員及其權限（Owner, Admin, Member）。

| Method | Path | 描述 |
| :--- | :--- | :--- |
| **GET** | `/organizations/{org_id}/members` | 列出該組織成員 |
| **POST** | `/organizations/{org_id}/members` | 邀請/加入成員 |
| **PATCH** | `/organizations/{org_id}/members/{user_id}` | 修改成員權限 (Role) |
| **DELETE** | `/organizations/{org_id}/members/{user_id}` | 移除成員 |

### E. 場館據點 (Locations)

組織旗下的實體場館（例如：台北分館、台中分館）。

| Method | Path | 描述 | 關鍵欄位 (Body) |
| :--- | :--- | :--- | :--- |
| **GET** | `/locations` | 場館列表 | Query: `organization_id`, `q` (關鍵字搜尋) |
| **POST** | `/locations` | 新增場館 | `organization_id`, `opening_hours`, `lat/long` |
| **GET** | `/locations/{id}` | 場館詳情 | - |
| **PATCH** | `/locations/{id}` | 更新場館 | - |
| **DELETE** | `/locations/{id}` | 刪除場館 | - |

### F. 場地資源 (Resources & Types)

實際可預約的單位（例如：A 球場、B 會議室）。

  * **Resource Types**: 場地類型定義（如：羽球場、籃球場）。
  * **Resources**: 實際的場地實體。

| Method | Path | 描述 | 篩選參數 |
| :--- | :--- | :--- | :--- |
| **GET** | `/resource-types` | 類型列表 | Query: `organization_id` |
| **POST** | `/resource-types` | 建立類型 | - |
| **GET** | `/resources` | 場地列表 | Query: `location_id`, `resource_type_id` |
| **POST** | `/resources` | 建立場地 | Body: `{resource_type_id, location_id}` |

### G. 預約管理 (Bookings)

使用者預約場地，或管理員查詢預約狀況。

| Method | Path | 描述 | 關鍵參數 |
| :--- | :--- | :--- | :--- |
| **GET** | `/bookings` | 查詢預約 | Query: `user_id`, `resource_id`, `status`, `start/end_time` |
| **POST** | `/bookings` | 建立預約 | Body: `{resource_id, start_time, end_time}` <br> **注意**: 系統會檢查時間重疊，若重疊回傳 409 |
| **GET** | `/bookings/{id}` | 預約詳情 | - |
| **PATCH** | `/bookings/{id}` | 修改預約 | 可更新時間或狀態 (`pending`, `confirmed`, `cancelled`) |
| **DELETE** | `/bookings/{id}` | 取消/刪除 | - |

### H. 系統公告 (Announcements)

全域公告系統。

| Method | Path | 描述 | 權限 |
| :--- | :--- | :--- | :--- |
| **GET** | `/announcements` | 公告列表 | 登入使用者 |
| **POST** | `/announcements` | 發布公告 | System Admin |
| **PATCH** | `/announcements/{id}` | 更新公告 | System Admin |
| **DELETE** | `/announcements/{id}` | 刪除公告 | System Admin |

## 5. 常見 Status Code 對照表

  * `200 OK`: 請求成功。
  * `201 Created`: 資源建立成功 (如註冊、新增組織)。
  * `204 No Content`: 刪除成功，無回傳內容。
  * `400 Bad Request`: 參數錯誤 (如 JSON 格式不對、缺少必填欄位)。
  * `401 Unauthorized`: 未登入或 Token 無效。
  * `403 Forbidden`: 有登入但權限不足 (如一般使用者嘗試刪除組織)。
  * `404 Not Found`: 找不到資源 (ID 不存在)。
  * `409 Conflict`: 資源衝突 (如 Email 已被註冊、預約時間重疊)。
  * `500 Internal Server Error`: 伺服器內部錯誤。
