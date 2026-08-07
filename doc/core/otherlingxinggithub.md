# 其他领星 OpenAPI 开源仓参考

> 本文档只收录「对本 Go 同步机有实际借鉴价值」的外部领星仓,以及**与语言无关、可直接落地的事实**(签名算法、限流实测参数、接口路径清单)。
> 借鉴原则:Python 仓的代码组织 / async 框架 / 类设计对 Go 项目**无价值**;只有算法步骤、实测数值、接口契约这类语言无关内容才值得抄。Go 能直接对照代码的是 `QQiot/lingxing`。
> 维护方式:外部仓随时可能改名/删库,抄进来的事实要做成本仓内部的定稿,不要长期外链依赖。

---

## 1. 仓库清单

| 仓库 | 地址 | 语言 | 对本项目的价值 |
|---|---|---|---|
| **QQiot/lingxing** | https://github.com/QQiot/lingxing | **Go** | ⭐ 最高:同语言,AES-ECB 签名、token 获取/刷新可直接对照 `internal/api/` |
| **SongKehao/lingxing-sdk** | https://github.com/SongKehao/lingxing-sdk | Python | ⭐ 高:680 接口路径清单、签名算法、**限流实测参数**(见 §3) |
| **zach22-1999/lingxing-mcp** | https://github.com/zach22-1999/lingxing-mcp | Python | 中:签名实现与 SongKehao 互相印证;只读 MCP,无落库 |
| AresJef/LingXingApi | https://github.com/AresJef/LingXingApi | Python | 低:async client |
| codeYi/lingxing | https://github.com/codeYi/lingxing | PHP | 低 |

**关键结论**:这些仓**全部停在 API client / SDK 层**(调领星接口、返回数据),没有一个做了"定时拉 → 限流 → 落库 → 给消费方读"的完整同步后端。本项目的同步链路架构在开源领星生态里是独一份,没有现成的同步后端可抄。

---

## 2. 签名算法(三方一致,Go 直接对照 QQiot)

领星 OpenAPI 业务请求签名(三个独立实现互相印证,结论一致):

```
1. 规范化参数:
   - 参数按 key 字典序排序
   - 跳过空值
   - bool 转小写(true/false)
   - dict/list 转 JSON(排序键)
   - 拼成  k=v&k=v
2. md5 = MD5(规范化串).hexdigest().upper()   ← 关键:MD5 后转大写
3. sign = AES-ECB-Encrypt(plaintext=md5, key=app_key)
   - 模式:ECB
   - padding:PKCS7
   - 密钥:app_key 本身(不是 app_secret),长度须为 16/24/32 字节
   - 输出:base64
```

- **Go 直接对照**:`QQiot/lingxing` 的 `aes.go`(标准 PKCS7 + ECB)、`authorization.go`(token 获取/刷新)。本项目 `internal/api/sign.go` 可逐行对照,确认 padding 与密钥用法无偏差。
- **Python 印证**:`SongKehao/lingxing-sdk` 的 `src/lingxing/core/sign.py` + `aes.py`;`zach22-1999/lingxing-mcp` 的 `lib/lingxing_openapi/client.py::generate_sign`。
- **注意**:token 获取接口(`/api/auth-server/oauth/access-token`)用 query 传 `appId` / `appSecret`,**不走上面的签名**;只有业务接口才需要 `app_key AES 签名`。

---

## 3. 限流实测参数(可直接进 `config.yaml` 的 `rate` 块)

> 来源:`SongKehao/lingxing-sdk` 的 `src/lingxing/core/rate_limiter.py`(实测注释)。这是本项目 `findings.md` 里一直没坐实的部分,现以下面数值为准。

| 场景 | 实测限流 | 建议间隔 | 领星错误码 |
|---|---|---|---|
| **Token 请求**(`/access-token`、`/refresh`) | 约每小时重置 | **≥ 60 秒** | `3001008` `"new requests too frequently. please request later."` |
| **业务 API** | 100 次/分钟 | **≥ 0.6 秒** | — |

落地建议:
- `rate.bucket=1` 的接口:`interval_ms` 取 `≥ 1000`(对应业务 API 留足余量,0.6s 是下限)。
- Token 由 TokenHolder 按 `app_key` 单飞刷新(本项目已实现),天然满足 60 秒间隔,不会多 Worker 并发踩 `3001008`。
- 撞 `3001008` 应**指数退避重试**,而不是直接把任务标 `error`——这是本项目 Worker 主循环可以补的一个改进点。
- 检测当前出口 IP 的官方接口:`https://toolbox.lingxing.com/api/getIp`(本项目部署到云服务器后出口固定,白名单一次性配置即可,不需要任何漂移处理)。

---

## 4. 接口路径清单(SongKehao 已覆盖 680 路径)

`SongKehao/lingxing-sdk` 的 `API-COVERAGE-REPORT.md` 标注了 **680 个 API path / 269 个 endpoint 方法**,且每个都带真实测试状态(OK / ERR:400 / ERR:500)。覆盖 VC 报表、海外仓、多平台广告、库存对账单等。

价值:本项目「加接口」时(`doc/core/07-add-endpoint.md` 五步),可用它快速查到目标接口的 `path` / `method` / 必填参数,避免从领星文档逐个翻。地址:
- 仓库:https://github.com/SongKehao/lingxing-sdk
- 覆盖报告:仓库根 `API-COVERAGE-REPORT.md`

> 注意:它的 OK/ERR 状态受其测试账号权限影响(如 VC 报表需 `vc_store_id`),ERR 不代表接口本身不可用,只代表该仓的测试条件没满足。以领星官方文档 `https://apidoc.lingxing.com/` 为最终准绳。

---

## 5. Go 项目能借鉴什么、不能借鉴什么

**能借(语言无关):**
- §2 签名算法 —— 已是定稿,对照 QQiot Go 版校验本项目实现即可。
- §3 限流实测数值 —— 直接进 `config.yaml` 的 `rate` 块。
- §4 接口路径清单 —— 加接口时查 path/参数。
- QQiot 的 Go 代码结构(token 刷新、http client 封装)—— 同语言,可对照写法。

**不能借(语言/范式不匹配,或违反本项目宪法):**
- Python async / 类继承体系 —— Go 是 goroutine + interface,套不上。
- MCP 项目的 token 缓存文件、多人令牌模式 —— 本项目用进程内 TokenHolder 单飞,不需要持久化。
- 任何仓的「完整同步链路」—— 它们没做;同步后端的参考在 Singer-SDK / Airbyte(通用 ETL 生态,见下),但那些是重量级框架,**引入即违反 CLAUDE.md「轻量、不锁」红线**,只学思想不引代码。

---

## 6. 同步后端(非领星)的设计参考,仅作思想借鉴

领星生态没有同步后端可抄。如果要看「增量游标 / 断点续传 / 父子流隔离」这些同步后端通式,**不要在领星仓里找**,看通用 ETL:

- **Airbyte 的 Amazon SP-API connector**:https://github.com/airbytehq/airbyte/tree/master/airbyte-integrations/connectors/source-amazon-seller-partner —— 声明式 YAML 把游标字段、回看窗、分页、429 退避、父子流全表达出来,是本项目「加接口零代码」理念的高级版。
- **meltano/singer-sdk**:https://github.com/meltano/sdk —— Stream 基类的 state/bookmark 断点续传、partition 级失败隔离。

⚠️ 这两个都是**重量级运行时框架**,只借鉴设计思想(`cursor_field` / `lookback_window` / 429 指数退避 / partition state),**不引入代码**——会违反本项目的轻量红线。
