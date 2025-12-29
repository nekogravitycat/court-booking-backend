# 角色與權限系統 (Role & Permission System)

這份文件描述了一個**階層式**的權限管理系統（RBAC），主要將權限分為「全域系統」、「組織（總部）」與「特定場館」三個層次：

1. **系統管理員 (System Admin)**：擁有神的視角，可管理所有組織與設定。
2. **組織層級 (Organization Level)**：包含「擁有者」與「組織管理員」。他們管理整個組織的營運，例如新增場域、管理員工。權限上兩者幾乎相同，但擁有者每個組織只能有一位。
3. **場域層級 (Location Level)**：即「場地管理員」。權限被限制在**特定場域**內，只能管理該場域的場地資源與預約狀況，無法觸及組織設定或刪除場域。

**關鍵邏輯**：系統在設計資料庫時，將「組織權限」與「場域權限」分開儲存（解耦）。這意味著一個人可以只當某個場域的管理員，而不需要是該組織的高層成員。

## 角色 (Roles)

本系統建立在 **組織 (Organizations)** 的概念之上。使用者隸屬於具有特定角色的組織。

### 1. 系統管理員 (System Admin) - 全域

**識別方式：** 在 `users` 表中 `is_system_admin = true`。

- **範圍：** 整個系統。
- **權限：** 對所有組織、使用者、場域和系統設定擁有完全存取權。
- **管理：** 可以建立組織並指派初始擁有者。

### 2. 組織擁有者 (Organization Owner)

**識別方式：** 在 `organization_permissions` 表中 `role = "owner"`。

- **數量：** 每個組織限制 **一位**。
- **範圍：** 他們所擁有的特定組織。
- **權限：**
  - 對組織內的所有資源擁有 **完全存取權 (Full access)**。
  - 可以管理組織設定（名稱、啟用狀態）。
  - 可以管理組織成員（管理員）。
  - 可以建立、更新和刪除場域 (Locations)。
  - 可以指派/取消指派場地管理員。

### 3. 組織管理員 (Organization Manager)

**識別方式：** 在 `organization_permissions` 表中 `role = "manager"`。

- **數量：** 允許由多位擔任。
- **範圍：** 特定的組織。
- **權限：**
  - 在能力上 **等同於擁有者**。
  - 可以管理組織設定。
  - 可以管理場域（建立/更新/刪除）。
  - 可以管理場地管理員。

### 4. 場地管理員 (Location Manager)

**識別方式：** 存在於 `location_admins` 資料表中。

- **數量：** 允許由多位擔任。
- **範圍：** 僅限 **特定場域**。
- **權限：**
  - **無權存取** 組織設定。
  - **無權** 操作場域本身（不能建立/刪除場域）。
  - **可以管理** 指派場域內的資源（場地/房間）。
  - **可以管理** 指派場域內的預約。
- **獨立性：** 使用者 **不需要** 是組織成員/組織管理員即可成為場地管理員。這兩者是分開的權限集。

## 權限邏輯 (Permission Logic)

存取控制是在服務層 (Service layer) 和 HTTP 處理層 (Handler layer) 強制執行的。

### 系統層級檢查

- **`IsSystemAdmin`：** 透過使用者標記 (flag) 進行檢查。此權限凌駕於所有其他檢查之上。

### 組織層級檢查

- **`CheckPermission(orgID, userID)` (完全存取權)：**
- 若使用者是系統管理員，回傳 `true`。
- 若使用者在 `organization_permissions` 中是 **Owner** 或 **Manager**，回傳 `true`。
- 授予權限：更新組織、建立/刪除場域、管理員工。

### 場域層級檢查

- **`CheckLocationPermission(orgID, locationID, userID)`：**
- 若使用者通過 `CheckPermission`（系統管理員 / 擁有者 / 組織管理員），回傳 `true`。
- 若使用者在特定 `locationID` 的 `location_admins` 清單中，回傳 `true`。
- 授予權限：更新場域詳細資訊、管理資源、管理預約。

## 用於權限管理的 API 端點

### 組織員工 (擁有者 & 組織管理員)

- `POST /v1/organizations/:id/members`：新增一位使用者為管理員。
  - 請求資料 (Payload)：`{"user_id": "uuid", "role": "manager"}`
- `DELETE /v1/organizations/:id/members/:user_id`：移除一位管理員。

### 場地員工 (場地管理員)

- `POST /v1/locations/:id/admins`：指派一位使用者為特定場域的管理員。
  - 請求資料 (Payload)：`{"user_id": "uuid"}`
  - 存取控制：僅限組織擁有者/組織管理員。
- `DELETE /v1/locations/:id/admins/:user_id`：從場域中移除一位管理員。
  - 存取控制：僅限組織擁有者/組織管理員。
- `GET /v1/locations/:id/admins`：列出某個場域的所有管理員。

## 資料庫架構模型 (Database Schema Model)

- **`organization_permissions`**：
  - 將 `user_id` 連結至 `organization_id` 並附帶 `role`（'owner' 或 'manager'）。
  - **限制 (Constraint)：** 應用程式邏輯應強制每個組織只有一位 'owner'。
  - **目的：** 授予高層級的組織控制權。
- **`location_admins`**：
  - 將 `user_id` 連結至 `location_id`。
  - **限制 (Constraint)：** 指向 `users` 和 `locations` 的外鍵 (Foreign keys)。
  - **目的：** 授予特定場域的存取權。
  - **解耦 (Decoupling)：** 此資料表獨立於 `organization_permissions`。
