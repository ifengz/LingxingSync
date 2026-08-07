// Package api 是领星 OpenAPI 客户端层。
//
// 这个文件 sign.go 只负责一件事：领星 OpenAPI 签名算法。
//
// 算法权威来源：领星官方 Go SDK（openapi-go-sdk/sign.go + aes.go）+
// 官方文档 apidoc.lingxing.com 接入指南 §4.1。与宪法 doc/08、doc/LINGXING_API_INTEGRATION
// 两份文档均有出入（doc 说 key=app_secret 或 key=app_key+hex，实测均 sign not correct），
// 以官方 SDK 为准：key=appId 原始字节，输出 base64。
//
// 宪法对应：doc/08-api-reference.md §8（签名失败 = 代码 bug，panic 不当业务错误）。
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
//
// 入参 params 必须包含：所有参与签名的业务参数（已 string 化）+
// app_key + access_token + timestamp（Unix 秒字符串）。sign 本身不参与签名。
//
// 算法（官方 SDK 权威版，apidoc §4.1）：
//  1. 所有参数按 key ASCII 升序
//  2. 拼接 key1=val1&key2=val2（不 URL encode；value 为空串不参与，但这里调用方保证非空）
//  3. MD5(32) → 转大写 → signStr
//  4. AES/ECB/PKCS5(=PKCS7) 加密 signStr，密钥 = appId（app_key）的原始字节
//  5. base64 编码 → sign（传输时由 url.Values 自动 URL encode）
//
// 签名失败（aes.NewCipher 等，如 app_key 长度非 16/24/32 字节）= 代码 bug，
// 直接 panic（宪法 §8：签名失败不当业务错误）。
func Sign(params map[string]string, appKey, appSecret string) string {
	// Step 1: key ASCII 升序
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Step 2: 拼接 k=v&k=v，不 URL encode
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	raw := strings.Join(parts, "&")

	// Step 3: MD5(32) → 全大写 signStr
	h := md5.Sum([]byte(raw))
	signStr := strings.ToUpper(fmt.Sprintf("%x", h))

	// Step 4: AES-ECB-PKCS5 加密，密钥 = appId 原始字节（官方 SDK：NewAesTool([]byte(appId), len(appId))）
	encrypted, err := aesECBEncrypt([]byte(signStr), []byte(appKey))
	if err != nil {
		// 签名失败 = 代码 bug（app_key 长度非 16/24/32 等），panic，不当业务错误兜底。
		panic(fmt.Sprintf("lingxing sign: AES encrypt failed (appKey len=%d): %v", len(appKey), err))
	}

	// Step 5: base64 输出
	return base64.StdEncoding.EncodeToString(encrypted)
}

// aesECBEncrypt 用 AES-ECB + PKCS5/PKCS7 填充加密（对 AES 块大小 16，两者等价）。
// Go 标准库不暴露 ECB 模式，手动逐块加密（ECB = 每块独立，无 IV）。
func aesECBEncrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize() // AES 固定 16 字节

	// PKCS5/PKCS7 填充：即使 data 正好是 bs 整数倍，也补一整块。
	padding := bs - len(data)%bs
	padded := make([]byte, len(data)+padding)
	copy(padded, data)
	for i := len(data); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	// 逐块加密（ECB：每块独立，无 IV）
	encrypted := make([]byte, len(padded))
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(encrypted[i:i+bs], padded[i:i+bs])
	}
	return encrypted, nil
}
