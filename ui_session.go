package main

import (
	"encoding/json"
	"fmt"
)

// Activity leases protect unsaved drafts and user-initiated operations while
// the hidden interface is eligible for memory reclamation.
func (a *App) SetUIActivity(token string, busy bool) error {
	if a.resident == nil {
		return nil
	}
	if token == "" || len(token) > 100 {
		return fmt.Errorf("无效界面状态")
	}
	a.resident.mu.Lock()
	defer a.resident.mu.Unlock()
	if a.resident.activity == nil {
		a.resident.activity = make(map[string]bool)
	}
	if busy {
		a.resident.activity[token] = true
	} else {
		delete(a.resident.activity, token)
	}
	return nil
}

func (a *App) SaveViewState(state string) error {
	if len(state) > 8192 || !json.Valid([]byte(state)) {
		return fmt.Errorf("界面状态无效")
	}
	if a.resident != nil {
		a.resident.mu.Lock()
		a.resident.viewState = state
		a.resident.mu.Unlock()
	}
	return nil
}

func (a *App) GetViewState() string {
	if a.resident == nil {
		return ""
	}
	a.resident.mu.Lock()
	defer a.resident.mu.Unlock()
	return a.resident.viewState
}

type RuntimeInfo struct {
	UIRunning     bool  `json:"uiRunning"`
	UIReady       bool  `json:"uiReady"`
	Visible       bool  `json:"visible"`
	LastStartupMS int64 `json:"lastStartupMS"`
}

func (a *App) GetRuntimeInfo() RuntimeInfo {
	if a.resident == nil {
		return RuntimeInfo{}
	}
	a.resident.mu.Lock()
	info := RuntimeInfo{UIRunning: a.resident.child != nil, UIReady: a.resident.ready, LastStartupMS: a.resident.lastStartupMS}
	a.resident.mu.Unlock()
	a.mu.Lock()
	info.Visible = a.windowVisible
	a.mu.Unlock()
	return info
}
