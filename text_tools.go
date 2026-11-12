package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"clipbox/internal/clipboard"
	"clipbox/internal/ocr"
	"clipbox/internal/translation"
)

const textToolsSettingsKey = "text_tools_settings"
const translationSecretKey = "translation_api_key_protected"

type TextToolsSettings struct {
	Endpoint       string `json:"endpoint"`
	Model          string `json:"model"`
	TargetLanguage string `json:"targetLanguage"`
	OCRLanguage    string `json:"ocrLanguage"`
	HasAPIKey      bool   `json:"hasAPIKey"`
	APIKey         string `json:"apiKey,omitempty"`
	ClearAPIKey    bool   `json:"clearAPIKey,omitempty"`
}

func (a *App) GetTextToolsSettings() (TextToolsSettings, error) {
	a.textToolsMu.Lock()
	defer a.textToolsMu.Unlock()
	return a.textToolsSettingsLocked()
}

func (a *App) textToolsSettingsLocked() (TextToolsSettings, error) {
	settings := TextToolsSettings{TargetLanguage: "简体中文"}
	if a.store == nil {
		return settings, fmt.Errorf("存储尚未就绪")
	}
	raw, err := a.store.GetSetting(textToolsSettingsKey)
	if err != nil {
		return settings, err
	}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &settings); err != nil {
			return settings, fmt.Errorf("识别与翻译设置无法读取，请重新保存")
		}
	}
	secret, err := a.store.GetSetting(translationSecretKey)
	if err != nil {
		return settings, err
	}
	settings.HasAPIKey = secret != ""
	settings.APIKey = ""
	settings.ClearAPIKey = false
	return settings, nil
}

func (a *App) SaveTextToolsSettings(settings TextToolsSettings) (TextToolsSettings, error) {
	a.textToolsMu.Lock()
	defer a.textToolsMu.Unlock()
	if a.store == nil {
		return TextToolsSettings{}, fmt.Errorf("存储尚未就绪")
	}
	settings.Endpoint = strings.TrimSpace(settings.Endpoint)
	settings.Model = strings.TrimSpace(settings.Model)
	settings.TargetLanguage = strings.TrimSpace(settings.TargetLanguage)
	settings.OCRLanguage = strings.TrimSpace(settings.OCRLanguage)
	key := strings.TrimSpace(settings.APIKey)
	if len(settings.Endpoint) > 1000 || len(settings.Model) > 200 || len(key) > 4096 || strings.ContainsAny(key, "\r\n") {
		return TextToolsSettings{}, fmt.Errorf("接口地址、模型或 API Key 无效或过长")
	}
	if settings.Endpoint != "" {
		if _, err := translation.Endpoint(settings.Endpoint); err != nil {
			return TextToolsSettings{}, err
		}
	}
	if settings.TargetLanguage == "" {
		settings.TargetLanguage = "简体中文"
	}
	if utf8.RuneCountInString(settings.TargetLanguage) > 60 || len(settings.OCRLanguage) > 80 {
		return TextToolsSettings{}, fmt.Errorf("语言设置过长")
	}
	values := make(map[string]string)
	if key != "" {
		protected, err := protectTranslationKey(key)
		if err != nil {
			return TextToolsSettings{}, err
		}
		values[translationSecretKey] = protected
	} else if settings.ClearAPIKey {
		values[translationSecretKey] = ""
	}
	settings.APIKey, settings.ClearAPIKey, settings.HasAPIKey = "", false, false
	data, err := json.Marshal(settings)
	if err != nil {
		return TextToolsSettings{}, err
	}
	values[textToolsSettingsKey] = string(data)
	if err := a.store.SetSettings(values); err != nil {
		return TextToolsSettings{}, err
	}
	return a.textToolsSettingsLocked()
}

func (a *App) beginTextTask(taskID string) (context.Context, func(), error) {
	if taskID == "" || len(taskID) > 100 {
		return nil, nil, fmt.Errorf("无效的处理任务")
	}
	a.textToolsMu.Lock()
	defer a.textToolsMu.Unlock()
	if a.textTasks == nil {
		a.textTasks = make(map[string]context.CancelFunc)
	}
	if _, exists := a.textTasks[taskID]; exists || len(a.textTasks) >= 4 {
		return nil, nil, fmt.Errorf("已有处理任务，请稍后重试")
	}
	parent := a.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 65*time.Second)
	a.textTasks[taskID] = cancel
	return ctx, func() {
		cancel()
		a.textToolsMu.Lock()
		delete(a.textTasks, taskID)
		a.textToolsMu.Unlock()
	}, nil
}

func (a *App) CancelTextTask(taskID string) {
	a.textToolsMu.Lock()
	cancel := a.textTasks[taskID]
	a.textToolsMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (a *App) GetOCRLanguages() ([]ocr.Language, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return ocr.Languages(ctx)
}

func (a *App) RecognizeClipText(id int64, language, taskID string) (ocr.Result, error) {
	ctx, done, err := a.beginTextTask(taskID)
	if err != nil {
		return ocr.Result{}, err
	}
	defer done()
	if a.store == nil {
		return ocr.Result{}, fmt.Errorf("存储尚未就绪")
	}
	if len(language) > 80 {
		return ocr.Result{}, fmt.Errorf("无效的 OCR 语言")
	}
	entry, err := a.store.GetByID(id)
	if err != nil {
		return ocr.Result{}, err
	}
	if entry == nil || entry.Type != "image" {
		return ocr.Result{}, fmt.Errorf("请选择一条图片记录")
	}
	dib, err := loadImageDib(entry.Content)
	if err != nil {
		return ocr.Result{}, fmt.Errorf("图片文件不可用，请重新截图")
	}
	if len(dib) > 50*1024*1024 {
		return ocr.Result{}, fmt.Errorf("图片过大，请缩小截图范围")
	}
	bmp := clipboard.DibToBmp(dib)
	if len(bmp) == 0 {
		return ocr.Result{}, fmt.Errorf("图片格式无效，请重新截图")
	}
	return ocr.Recognize(ctx, bmp, language)
}

func (a *App) TranslateText(text, target, taskID string) (string, error) {
	ctx, done, err := a.beginTextTask(taskID)
	if err != nil {
		return "", err
	}
	defer done()
	a.textToolsMu.Lock()
	settings, err := a.textToolsSettingsLocked()
	var protected string
	if err == nil {
		protected, err = a.store.GetSetting(translationSecretKey)
	}
	a.textToolsMu.Unlock()
	if err != nil {
		return "", err
	}
	if settings.Endpoint == "" || settings.Model == "" {
		return "", fmt.Errorf("请先到设置 → 识别与翻译填写接口地址、模型和 API Key")
	}
	key, err := unprotectTranslationKey(protected)
	if err != nil {
		return "", err
	}
	return translation.Translate(ctx, settings.Endpoint, settings.Model, key, text, target)
}

// Copy only on explicit user action. Normal capture rules decide whether the
// copied result becomes a new history entry; the source clip is never changed.
func (a *App) CopyTextResult(text string) error {
	if strings.TrimSpace(text) == "" || len(text) > 2*1024*1024 {
		return fmt.Errorf("没有可复制的文字，或文字过长")
	}
	a.clipboardActionMu.Lock()
	defer a.clipboardActionMu.Unlock()
	return clipboard.WriteTextToClipboard(text)
}
