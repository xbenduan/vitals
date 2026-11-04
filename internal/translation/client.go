// Package translation translates clipboard text with Tencent TranSmart.
package translation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	TargetAuto    = "auto"
	TargetChinese = "zh"
	TargetEnglish = "en"

	endpoint         = "https://transmart.qq.com/api/imt"
	maxResponseBytes = 2 << 20
	maxInputRunes    = 5000
)

// Config holds TranSmart developer credentials and the desired output language.
// User is the developer account's User value, not a Tencent Cloud SecretId.
type Config struct {
	User           string `json:"user"`
	Token          string `json:"token,omitempty"`
	TargetLanguage string `json:"targetLanguage"`
}

// Default selects automatic Chinese/English translation without credentials.
func Default() Config {
	return Config{TargetLanguage: TargetAuto}
}

// WithDefaults normalizes preferences loaded from older configuration files.
func (c Config) WithDefaults() Config {
	c.User = strings.TrimSpace(c.User)
	c.Token = strings.TrimSpace(c.Token)
	switch c.TargetLanguage {
	case TargetAuto, TargetChinese, TargetEnglish:
	default:
		c.TargetLanguage = TargetAuto
	}
	return c
}

// Client performs bounded, cancellable requests without accessing UI or clipboard state.
type Client struct {
	config Config
	http   *http.Client
}

// New validates TranSmart credentials and configures the official HTTPS endpoint.
func New(config Config) (*Client, error) {
	switch config.TargetLanguage {
	case "", TargetAuto, TargetChinese, TargetEnglish:
	default:
		return nil, errors.New("请选择自动互译、简体中文或英文")
	}
	config = config.WithDefaults()
	if config.User == "" || config.Token == "" {
		return nil, errors.New("请在翻译设置中填写腾讯交互翻译的 User 和 Token")
	}
	return &Client{config: config, http: &http.Client{
		Timeout: 45 * time.Second,
		// Credentials live in the JSON body; never replay it to a redirected host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

type requestHeader struct {
	Fn    string `json:"fn"`
	User  string `json:"user"`
	Token string `json:"token"`
}

type response struct {
	Header struct {
		RetCode string `json:"ret_code"`
	} `json:"header"`
	Status          int             `json:"status"`
	Error           json.RawMessage `json:"error"`
	Language        string          `json:"language"`
	AutoTranslation []string        `json:"auto_translation"`
}

// Translate detects the source language and requests Markdown-aware translation.
// Automatic mode translates Chinese to English and other languages to Simplified Chinese.
// The app limits each request to 5,000 Unicode characters and 45 seconds in total.
func (c *Client) Translate(ctx context.Context, text string) (string, error) {
	if strings.TrimSpace(text) == "" {
		return "", errors.New("待翻译内容不能为空")
	}
	if !utf8.ValidString(text) {
		return "", errors.New("待翻译内容不是有效的 UTF-8 文字")
	}
	if utf8.RuneCountInString(text) > maxInputRunes {
		return "", errors.New("单次最多翻译 5,000 个字符，请选取较短内容")
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	analysis, err := c.request(ctx, map[string]any{
		"header":    requestHeader{Fn: "text_analysis", User: c.config.User, Token: c.config.Token},
		"type":      "plain",
		"text":      text,
		"normalize": map[string]bool{"merge_broken_line": false},
	})
	if err != nil {
		return "", err
	}
	source := strings.TrimSpace(analysis.Language)
	switch strings.ToLower(source) {
	case "", "auto", "unknown", "und", "un":
		return "", errors.New("腾讯交互翻译未能识别原文语言，请调整内容后重试")
	}
	target := c.config.TargetLanguage
	if target == TargetAuto {
		target = TargetChinese
		language := strings.ToLower(source)
		if language == "zh" || strings.HasPrefix(language, "zh-") || strings.HasPrefix(language, "zh_") || language == "zt" {
			target = TargetEnglish
		}
	}
	if source == target {
		return text, nil
	}
	translated, err := c.request(ctx, map[string]any{
		"header": requestHeader{Fn: "auto_translation", User: c.config.User, Token: c.config.Token},
		"type":   "markdown",
		"source": map[string]any{"lang": source, "text_list": []string{text}},
		"target": map[string]string{"lang": target},
	})
	if err != nil {
		return "", err
	}
	if len(translated.AutoTranslation) != 1 || strings.TrimSpace(translated.AutoTranslation[0]) == "" {
		return "", errors.New("腾讯交互翻译未返回完整译文，请重试")
	}
	return translated.AutoTranslation[0], nil
}

func (c *Client) request(ctx context.Context, body any) (response, error) {
	var result response
	data, err := json.Marshal(body)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New("无法连接腾讯交互翻译，请检查网络后重试")
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return result, errors.New("腾讯交互翻译认证失败，请检查 User、Token 和 API 权限")
	case http.StatusTooManyRequests:
		return result, errors.New("腾讯交互翻译请求过于频繁或额度不足，请稍后重试")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return result, fmt.Errorf("腾讯交互翻译返回 HTTP %d，请稍后重试", res.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		return result, errors.New("读取腾讯交互翻译响应失败，请重试")
	}
	if len(data) > maxResponseBytes {
		return result, errors.New("腾讯交互翻译响应超过 2 MB")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, errors.New("腾讯交互翻译响应格式无效，请稍后重试")
	}
	var serviceError string
	_ = json.Unmarshal(result.Error, &serviceError)
	if result.Status == 401 || result.Status == 403 || strings.EqualFold(result.Header.RetCode, "Auth-Failed") || serviceError == "Session-Out" {
		return result, errors.New("腾讯交互翻译认证失败，请检查 User、Token 和 API 权限")
	}
	if result.Status == 429 {
		return result, errors.New("腾讯交互翻译请求过于频繁或额度不足，请稍后重试")
	}
	if result.Header.RetCode != "succ" || (result.Status != 0 && result.Status != 200) ||
		(len(result.Error) > 0 && string(result.Error) != "null" && string(result.Error) != `""`) {
		// Do not expose raw server errors: they may echo credentials or clipboard text.
		return result, errors.New("腾讯交互翻译未能完成请求，请检查 API 权限、额度及语言支持")
	}
	return result, nil
}
