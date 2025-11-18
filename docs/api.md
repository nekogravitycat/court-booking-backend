# Court Booking Backend – API v1 Overview (Updated)

## 0. 總覽與核心原則

系統要解決：

* 多個 **organization**（品牌 / 場館營運方）
* 底下有實體 **locations**（分店 / 場館）
* 每個 organization 有不同類型的 **resources**（實際可預約的場地，例如羽球 A 場、B 場）
* 一般 **users** 自行註冊帳號
* 組織管理者把已註冊的 user 加入自己 organization，並設定其角色（owner/admin/member）
* **bookings**：user 對某個 resource 在一段時間內建立預約

**重要設計決策：**

> 1. **User 帳號一律由使用者「自行註冊」 (`POST /v1/auth/register`)。**
> 2. **Admin / Owner 不會「代創帳號」，只會將既有 user 加入 organization 並設定權限（例如變成場地管理者）。**



## 1. 角色與權限模型

### 1.1 角色定義

**System Admin（平台管理員）**

* DB: `users.is_system_admin = true`
* 權限：跨所有 organization / location / resource / booking 管理
* 典型用途：全平台後台、問題排查

**Organization-level roles（場館組織內角色）**

* DB: `organization_permissions` + enum `organization_role` = `'owner' | 'admin' | 'member'`

解讀：

* `owner`：該 organization 的擁有者，最高權限
* `admin`：該 organization 的管理者（**場地管理者 / Venue Manager**）
* `member`：該 organization 的一般成員（未來可用於內部員工）

> **Venue Manager** = 在某個 organization 的 `role = 'admin'` 的 user。

**Normal User（一般使用者）**

* 只存在於 `users`，不一定在任何 organization 有角色
* 權限：

  * 註冊 / 登入
  * 查詢公開場館與場地
  * 建立自己的 bookings
  * 查看 / 取消自己的 bookings



## 2. Data Model 概觀

* `users` – 所有使用者（含 System Admin）
* `organizations` – 場館營運單位（品牌）
* `organization_permissions` – 每個 user 在每個 organization 的角色
* `locations` – 實體場館（分店）
* `resource_types` – 場地類型（羽球場、籃球場…）
* `resources` – 實際可預約的場地（羽球 A 場、B 場）
* `bookings` – User 對某個 resource 在一段時間的預約
* `announcements` – 公告



## 3. API 共通約定

### 3.1 Base path & 版本

* Base: `/v1/...`

### 3.2 認證

* Header：`Authorization: Bearer <token>`
* Token 內容至少包含 `user_id`
* Middleware 會：

  * 解析 token → 找出對應 user
  * 查 `users.is_system_admin`
  * 查 `organization_permissions` 計算該 user 在各 org 的 roles
  * 放進 context 給 handler / service 使用

### 3.3 List API：分頁 / 排序 / 過濾

建議統一格式：

* Query：

  * `page`（起始 1）
  * `page_size`（預設 20，上限例如 100）
  * `sort`：逗號分隔欄位名，加 `-` 表示 desc
    例如：`sort=created_at,-email`
* Response：

```jsonc
{
  "items": [ /* ... */ ],
  "page": 1,
  "page_size": 20,
  "total": 123
}
```



## 4. Auth & Myself

### 4.1 註冊 / 登入

* `POST /v1/auth/register`

  * 說明：使用者自行註冊帳號
  * Body（示意）：

    * `email`
    * `password`
    * `display_name`

* `POST /v1/auth/login`

  * 說明：登入並取得 JWT

### 4.2 取得自己的資訊

* `GET /v1/me`

  * 回傳:

    * 基本資料：`id`, `email`, `display_name`, `is_active`, `is_system_admin`
    * 可選：該 user 在各 organization 的 roles 列表



## 5. Users（使用者）

> **重點：這裡不會有「Admin 建立 user」的 API。帳號只會透過 `/v1/auth/register` 產生。**

**Resource**: `User`
**DB**: `users`

### 5.1 List users（僅供 System Admin 檢視 / 管理）

* `GET /v1/users`
* 權限：

  * System Admin
* 用途：

  * 平台級後台，查看所有 user 狀態
* filters：

  * `email`
  * `display_name`
  * `is_active`

### 5.2 Get user by id

* `GET /v1/users/{user_id}`
* 權限：
  * System Admin

### 5.3 Update user（系統層級的修改）

* `PATCH /v1/users/{user_id}`
* 權限：

  * System Admin
* 可修改欄位：

  * `is_active`（停用 / 啟用帳號）
  * `is_system_admin`（升級 / 降級為平台管理員）
  * 等等

> 一般使用者修改自己 display name 的需求，可以另外設計
> `PATCH /v1/me`（只允許改 `display_name`），跟這個 admin API 分開。


## 6. Organizations & Members（場館組織與成員）

**Resource**: `Organization`, `OrganizationMember`
**DB**: `organizations`, `organization_permissions`

### 6.1 Organizations

* `GET /v1/organizations`

  * System Admin：全部
  * 之後可選擇讓 owner/admin 只看到自己有權限的 organizations
* `POST /v1/organizations`

  * 權限：

    * System Admin（v1 可以先這樣，未來再看要不要開放 user 創建）
* `GET /v1/organizations/{org_id}`
* `PATCH /v1/organizations/{org_id}`
* `DELETE /v1/organizations/{org_id}`（多半會做 soft-delete 或停用）

### 6.2 Organization members / permissions

> 這裡是「把已註冊的 user 加進 organization，並設定角色」的地方。
> **讓某人變成場地管理者 = 給他在該 org 的 `role = 'admin'`。**

* `GET /v1/organizations/{org_id}/members`

  * 權限：

    * 該 org 的 owner/admin
    * System Admin
  * filters:

    * `role=owner|admin|member`

* `POST /v1/organizations/{org_id}/members`

  * 權限：

    * owner/admin
  * Body：

    * **用 `user_id`**（前端先查 user 再選）：

       ```jsonc
       {
         "user_id": "uuid-of-existing-user",
         "role": "admin"
       }
       ```
  * 行為：

    * 在 `organization_permissions` 插入一筆 `(organization_id, user_id, role)`。

* `PATCH /v1/organizations/{org_id}/members/{user_id}`

  * 用途：調整角色，例如：

    * `member → admin`（升級為場地管理者）
    * `admin → member`（降權）

* `DELETE /v1/organizations/{org_id}/members/{user_id}`

  * 用途：把 user 從該 organization 移除

#### 6.2.1 Flow：讓某人變成場地管理者（Venue Manager）

1. 對方先自行註冊 → `POST /v1/auth/register`
2. 場館 owner/admin 到後台：

   1. 透過 `GET /v1/users?email=...` 或內部搜尋找到該 user
   2. 呼叫 `POST /v1/organizations/{org_id}/members`：

      * `user_id` = 該 user 的 id
      * `role` = `"admin"`
3. 之後該 user 就會以「場地管理者身份」看到：

   * 自己 organization 旗下的場館 / 場地 / bookings



## 7. Locations / Resource Types / Resources

### 7.1 Locations

**Resource**: `Location`
**DB**: `locations`

* `GET /v1/locations`

  * filters:

    * `organization_id`
    * 關鍵字 `q`
  * 一般 user 可以查公開場館列表
* `POST /v1/locations`

  * 權限：

    * 該 organization 的 owner/admin
* `GET /v1/locations/{location_id}`
* `PATCH /v1/locations/{location_id}`
* `DELETE /v1/locations/{location_id}`（可視為停用）

### 7.2 ResourceTypes

**Resource**: `ResourceType`
**DB**: `resource_types`

* `GET /v1/resource-types`

  * filters: `organization_id`
* `POST /v1/resource-types`
* `GET /v1/resource-types/{type_id}`
* `PATCH /v1/resource-types/{type_id}`
* `DELETE /v1/resource-types/{type_id}`

### 7.3 Resources

**Resource**: `Resource`（Domain 上就是 field/court）
**DB**: `resources`

* `GET /v1/resources`

  * filters:

    * `resource_type_id`
    * （可透過 join filter `organization_id`）
* `POST /v1/resources`

  * 權限：

    * 該 organization owner/admin
* `GET /v1/resources/{resource_id}`
* `PATCH /v1/resources/{resource_id}`
* `DELETE /v1/resources/{resource_id}`



## 8. Bookings（預約）

**Resource**: `Booking`
**DB**: `bookings`（`resource_id`, `user_id`, `start_time`, `end_time`, `status`）

### 8.1 List bookings

* `GET /v1/bookings`
* 權限 / 行為：

  * 一般 user：

    * 預設只看到自己預約
    * 例如：不帶 `user_id` 就代表 `me`
  * Organization admin / owner：

    * 可以看到自己 organization 底下所有 resources 的 bookings
  * System Admin：

    * 可以看到全部
* filters:

  * `user_id`
  * `resource_id`
  * `status`
  * `start_time_from`, `start_time_to`

### 8.2 Create booking（user 自己訂場）

* `POST /v1/bookings`
* 權限：

  * 已登入 user
* Body（簡化示意）：

  ```jsonc
  {
    "resource_id": 123,
    "start_time": "2025-11-20T18:00:00+08:00",
    "end_time": "2025-11-20T19:00:00+08:00"
  }
  ```
* 行為：

  * 檢查時間合法
  * 檢查是否有跟既有 booking 重疊
  * 建立一筆 `status = pending` 或 `confirmed`（依產品需求）

### 8.3 Get / Update / Delete booking

* `GET /v1/bookings/{booking_id}`

  * 看權限（本人 / org admin / system admin）
* `PATCH /v1/bookings/{booking_id}`

  * User：可以在規則範圍內修改自己的 booking
  * Org admin：可以改狀態（例如 `pending → confirmed` 或 `cancelled`）
* `DELETE /v1/bookings/{booking_id}`

  * User 取消自己的預約
  * Org admin / System Admin 也可以強制取消
  * 實作可以改成 `status = cancelled`，保留歷史紀錄



## 9. Announcements（公告）

**Resource**: `Announcement`
**DB**: `announcements`

* `GET /v1/announcements`

  * 可開放給所有 user
* `POST /v1/announcements`

  * System Admin（未來可考慮 org admin）
* `GET /v1/announcements/{id}`
* `PATCH /v1/announcements/{id}`
* `DELETE /v1/announcements/{id}`

