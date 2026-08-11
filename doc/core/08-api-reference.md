# 领星 OpenAPI 接入参考（Go 实现专用）

> **主要参考来源：领星官方 OpenAPI 文档**（每接口页直接给出令牌桶容量、限流间隔、参数类型、唯一键）。  
> SDK（`QQiot/lingxing`、`PerfectWorld233/lingxingapi-httpx`）仅作签名算法交叉验证，不作为接口规格的权威来源。  
> 生产踩坑记录来自 `doc/LINGXING_API_INTEGRATION.md`（polabel2 生产验证）。
>
> LingxingSync **只接 OpenAPI**，ERP 报表接口（auth-token 那套）和页面 token 一律不碰。
>
> 此处引用 polabel2 仅用于学习已验证的 method/path/body、候选上下文、字段处理和错误处理；两个项目完全独立，不连接数据库、不投影数据、不修改 polabel2 页面或事实表。

---

## 1. 三套认证系统 — 必须区分

| 类型 | 域名 | 认证方式 | LingxingSync 要用吗 |
|---|---|---|---|
| **OpenAPI** | `openapi.lingxing.com` | `app_key + app_secret + access_token + sign` | ✅ 全部用这套 |
| ERP 报表 | `gw.lingxingerp.com` | `auth-token + x-ak-company-id` | ❌ 不用 |
| 页面登录态 | `erp.lingxing.com` | 浏览器 Cookie | ❌ 不用 |

**OpenAPI 的 `access_token` 不能拿去打 ERP 报表接口，反之亦然。** polabel2 已踩过这个坑。

---

## 2. OpenAPI 认证流程

### 2.1 取 Token

```
POST https://openapi.lingxing.com/api/auth-server/oauth/access-token
Content-Type: multipart/form-data

appId=<app_key>
appSecret=<app_secret>
```

返回：
```json
{
  "code": 0,
  "data": {
    "access_token": "xxx",
    "refresh_token": "xxx",
    "expires_in": 7200
  }
}
```

- Token 有效期 7200 秒（2小时）
- 过期后用 `refresh_token` 刷新，或重新取
- **同一 app_key 并发取 token 会触发限流** → 必须 singleflight（见架构文档）

### 2.2 签名算法

每次业务请求必须带签名。算法（已在 polabel2 生产验证）：

```
1. 收集参数：业务 body 参数 + app_key + access_token + timestamp（Unix 秒）
2. 按 key 字母升序排列
3. 拼接：key1=val1&key2=val2&...（不 URL encode）
4. MD5 → 转大写 → 得到 sign_str
5. AES-ECB 加密 sign_str，密钥用 app_secret 补齐 16/32 字节
6. Base64 → 得到最终 sign
```

公共参数（放在 query，不在 body）：
```
app_key=xxx&access_token=xxx&timestamp=1234567890&sign=xxx
```

业务参数放 JSON body。

### 2.3 Go 实现（完整可用，直接 copy 进 `internal/api/sign.go`）

```go
package api

import (
    "crypto/aes"
    "crypto/md5"
    "encoding/base64"
    "fmt"
    "sort"
    "strings"
)

// Sign 构造领星 OpenAPI 签名。
// params 必须包含：所有业务 body 参数 + app_key + access_token + timestamp（Unix 秒字符串）。
// 返回值放在 query 参数 sign= 里。
func Sign(params map[string]string, appSecret string) string {
    // Step 1: key 字母升序
    keys := make([]string, 0, len(params))
    for k := range params {
        keys = append(keys, k)
    }
    sort.Strings(keys)

    // Step 2: 拼接 k=v&k=v，不做 URL encode
    parts := make([]string, 0, len(keys))
    for _, k := range keys {
        parts = append(parts, k+"="+params[k])
    }
    raw := strings.Join(parts, "&")

    // Step 3: MD5 → 全大写
    h := md5.Sum([]byte(raw))
    signStr := strings.ToUpper(fmt.Sprintf("%x", h))

    // Step 4: AES-ECB 加密（取 app_secret 前 16 字节，不足补零）
    key := padOrTrimKey([]byte(appSecret), 16)
    encrypted, err := aesECBEncrypt([]byte(signStr), key)
    if err != nil {
        // 签名失败 = 代码 bug，不是运行时错误，直接 panic
        panic(fmt.Sprintf("lingxing sign: AES encrypt failed: %v", err))
    }

    // Step 5: Base64 → 最终 sign
    return base64.StdEncoding.EncodeToString(encrypted)
}

// padOrTrimKey 将 key 补零或截断到 size 字节（AES-128 传 16）。
func padOrTrimKey(key []byte, size int) []byte {
    if len(key) >= size {
        return key[:size]
    }
    padded := make([]byte, size)
    copy(padded, key)
    return padded
}

// aesECBEncrypt 用 AES-ECB + PKCS7 填充加密。
// Go 标准库不暴露 ECB 模式，这里手动逐块加密（ECB = 每块独立，无 IV）。
func aesECBEncrypt(data, key []byte) ([]byte, error) {
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    bs := block.BlockSize() // AES 固定 16 字节

    // PKCS7 填充
    padding := bs - len(data)%bs
    padded := make([]byte, len(data)+padding)
    copy(padded, data)
    for i := len(data); i < len(padded); i++ {
        padded[i] = byte(padding)
    }

    // 逐块加密
    encrypted := make([]byte, len(padded))
    for i := 0; i < len(padded); i += bs {
        block.Encrypt(encrypted[i:i+bs], padded[i:i+bs])
    }
    return encrypted, nil
}
```

**使用方法**：

```go
// 在 client.go 里组装 query 参数时调用
queryParams := map[string]string{
    "app_key":      cfg.AppKey,
    "access_token": token,
    "timestamp":    strconv.FormatInt(time.Now().Unix(), 10),
}
// 把业务 body 参数（string 化）也加入 queryParams 一起签名
for k, v := range bodyParamsAsString {
    queryParams[k] = v
}
queryParams["sign"] = Sign(queryParams, cfg.AppSecret)
// sign 本身不参与签名，最后加
```

---

## 3. 请求结构模板

```go
// 公共 query 参数
type CommonQuery struct {
    AppKey      string `url:"app_key"`
    AccessToken string `url:"access_token"`
    Timestamp   int64  `url:"timestamp"`
    Sign        string `url:"sign"`
}

// 分页参数（大多数列表接口通用）
type PageParams struct {
    Offset int `json:"offset"`   // 从 0 开始
    Length int `json:"length"`   // 单页最大通常 200
}
```

---

## 4. 分页规则

- 分页参数必须按接口合同逐项确认；常见形态是 `offset + length`，也有接口使用 `page + pageSize`，不能跨接口猜测。
- 当前同步机 Worker 的通用分页实现使用 `offset + length`：`offset` 从 0 开始，下一页按实际取到的行数前进。
- 大多数接口返回 `has_more: true/false` 或 `total` 字段
- 判断终止：`has_more == false` 或 `offset + length >= total`
- 安全边界：单页 `length` 不超过 200（部分接口 50）
- 建议每页取完后 sleep 100-300ms，防止触发限流

分页是否可并行不是由 `bucket > 1` 单独决定。只有在接口已确认支持多个在途请求、首屏 `total` 可靠且同步期间结果稳定时，才允许预计算多个 offset；否则保持串行，避免数据变化造成漏行或重复。

---

## 5. 限流处理

| 触发条件 | 响应特征 | 处理方式 |
|---|---|---|
| 请求过快 | HTTP 429、`code=3001008` 或限流文案 | 优先使用 `Retry-After`；没有时按 `5s/15s/30s/60s/120s` 冷却，最多 5 次 |
| 上游临时异常 | `code=103` | 按 `30s/60s/120s` 延迟后有限重试；不把普通 103 当令牌桶限流 |
| Performance 频繁请求 | 路径 `/bd/productPerformance/openApi/asinList` 且 `code=103` 消息明确含“频繁请求” | 按限流冷却处理，使用 `Retry-After` 或 `5s/15s/30s/60s/120s`，最多 5 次 |
| Token 过期/不正确 | `code=2001003` / `code=2001005` | 当前 credential 单飞刷新 token，再重试原请求 1 次 |
| 未授权 | `code=2001004` | 直接报错，人工检查接口授权，不重试 |
| IP 白名单 | `code=3001002` 或 `ip not permit` | 直接报错，检查固定出口 IP 和领星白名单，不重试 |
| 参数/签名错误 | 参数缺失、`code=2001006`、`code=2001007` 等 | 直接报错，修正合同、签名或时钟，不指数重试 |
| 并发取 token | 领星直接拒绝 | singleflight 保证只有一个 goroutine 取 |

### 5.1 请求级退避合同

- `Retry-After` 是可选响应头，支持“秒数”和 HTTP-date 两种格式；存在且可解析时优先使用。
- 当前实现不能假设领星每个业务错误都会返回该响应头；业务码 `3001008`/`103` 必须保留本地退避表作为兜底。
- 网络超时、连接中断等错误如果请求可能已经到达领星，不能按 `500ms` 立即补发；当前按 2 分钟远端令牌保护窗口后再尝试，避免制造新的 `3001008`。
- 所有重试都必须重新经过同一个 `(quota_group, path)` 限流器；重试不能绕过限流器。
- 达到对应最大次数后必须以 error 结束，不能把空数据或部分数据标记为 success。

**限流的正确模型**（见 [09-endpoint-contract.md §格4](09-endpoint-contract.md)）：

- 领星按 `(账号, path)` 共享配额；同账号下所有 appId 共用一个桶
- 运行时限流器 key = `(quota_group, path)`，**不是** endpoint 配置名
- `bucket = 1` → 强制串行：前一个请求返回后 + `interval_ms` 等待，才能发下一个
- `bucket > 1` → 可并发翻页，`rate.Limiter` 自动控速

⚠️ **多店铺并行踩坑**：不要以为每个店铺有独立预算就随意并发 — 它们都消耗同一个 `(quota_group, path)` 桶，叠加超额会被限流。`multi_interval_ms` 就是要在多店铺间错开间隔。

每个接口的限流档案从官方文档 Rate Limit 区块直接抄写，填入 config.yaml 的 `rate:` 块。

---

## 6. 接口清单

### 6.1 基础数据（优先实现）

| 接口 | 方法 | 路径 | 说明 | DB 表 |
|---|---|---|---|---|
| 取 Token | POST | `/api/auth-server/oauth/access-token` | 认证，不落库 | — |
| SC 店铺列表 | GET | `/erp/sc/data/seller/lists` | 带 `has_ads_setting` | `ls_stores` |
| VC 店铺列表 | POST | `/basicOpen/platformAuth/vcSeller/pageList` | `offset/length` | `ls_stores` |
| 广告账号列表 | POST | `/basicOpen/baseData/account/list` | 拿 `profile_id` | `ls_ad_accounts` |

### 6.2 Listing

| 接口 | 方法 | 路径 | 主键字段 | DB 表 |
|---|---|---|---|---|
| VC Listing | POST | `/basicOpen/listingManage/vcListing/pageList` | `asin + vc_store_id` | `ls_listings` |

### 6.3 订单

| 接口 | 方法 | 路径 | 主键字段 | DB 表 |
|---|---|---|---|---|
| VC 订单列表 | POST | `/basicOpen/platformOrder/vcOrder/pageList` | `local_po_number` | `ls_orders_vc` |
| VC PO 详情 | POST | `/basicOpen/platformOrder/vcOrderPo/detail` | `local_po_number` | `ls_orders_vc_po` |

### 6.4 库存

| 接口 | 方法 | 路径 | 说明 | DB 表 |
|---|---|---|---|---|
| FBA 库存 | POST | `/basicOpen/inventory/fba/list`（待验证） | 参考 httpx 仓 `api.warehouse` | `ls_fba_inventory` |

### 6.5 广告报表（按优先级）

| 接口 | 方法 | 路径 | 归因窗口 | DB 表 |
|---|---|---|---|---|
| SP 商品报表 | POST | `/pb/openapi/newad/spProductAdReports` | 7 天 | `ls_ads_sp` |
| SB 商品报表 | POST | `/pb/openapi/newad/listHsaProductAdReport` | 14 天 | `ls_ads_sb` |
| SD 商品报表 | POST | `/pb/openapi/newad/sdProductAdReports` | 14 天 | `ls_ads_sd` |
| 广告授权过滤 | GET | `/erp/sc/data/seller/lists` | 看 `has_ads_setting` | 内存判断 |

⚠️ 广告接口注意（polabel2 已踩坑）：
- SC 店铺走 `sid`；VC 广告账号走 `profile_id`，两者不能互换
- `ENTITY ID` 只是页面展示字段，不当报表参数用
- SB 创意如果覆盖多个 ASIN，不拆，只记能唯一映射到单个 ASIN 的条目

### 6.6 财务（二期）

| 接口 | 方法 | 路径 | DB 表 |
|---|---|---|---|
| 结算报告 | POST | `/basicOpen/finance/settlement/pageList`（待验证） | `ls_finance_settlement` |

---

## 7. Go SDK 使用（QQiot/lingxing）

```bash
go get github.com/hiscaler/lingxing
```

SDK 处理了：token 获取、签名、基础 HTTP 封装。  
返回格式统一：`(items, nextOffset, isLastPage, err)`。

**使用建议：**
- SDK 是 thin wrapper，业务参数自己构造
- token 刷新需配合 TokenHolder（singleflight）管理，不要让 SDK 自己管
- 如果 SDK 某个接口不全，直接绕过 SDK 手写 HTTP 请求（签名逻辑独立封装）

---

## 8. 踩坑汇总（来自 polabel2 生产）

| 坑 | 怎么避免 |
|---|---|
| 用 `access_token` 打 ERP 报表接口 | LingxingSync 根本不接 ERP，彻底绕过 |
| 并发多个 goroutine 取同一 token | singleflight，一个 goroutine 取完，其他等结果 |
| `sid` 和 `profile_id` 混用 | 建表时分开存：`sid` 归 SC 店铺，`profile_id` 归广告账号 |
| 分页没判断 `has_more` 就退出 | Worker 循环必须检查终止条件，不提前 break |
| token 过期没重试就报失败 | 遇 40001，刷新一次，重试；仍失败才写 error |
| 请求频率不控制，被限流 | rate.Limiter，每个 endpoint 独立令牌桶 |
| 把签名失败当作业务错误处理 | 签名失败一律 panic 或 fatal，是代码 bug，不是运行时错误 |
