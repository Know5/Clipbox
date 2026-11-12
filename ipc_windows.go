package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

const maxIPCBytes = 24 * 1024 * 1024

type ipcRequest struct {
	Token  string            `json:"token"`
	Method string            `json:"method"`
	Args   []json.RawMessage `json:"args,omitempty"`
}

type ipcResponse struct {
	Value json.RawMessage `json:"value,omitempty"`
	Error string          `json:"error,omitempty"`
}

type uiEvent struct {
	Name string          `json:"name"`
	Data json.RawMessage `json:"data,omitempty"`
}

type ipcClient struct {
	path  string
	token string
}

// The pipe's ACL is restricted to this Windows user. No TCP listener, port
// discovery file or clipboard/API credential is exposed to a browser.
func userPipe(dataDir string) (path, descriptor string, err error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", "", err
	}
	sid := user.User.Sid.String()
	hash := sha256.Sum256([]byte(sid + "|" + strings.ToLower(dataDir)))
	return `\\.\pipe\clipbox-` + hex.EncodeToString(hash[:12]), "D:P(A;;GA;;;" + sid + ")", nil
}

func (c *ipcClient) request(ctx context.Context, method string, args ...any) (json.RawMessage, error) {
	request := ipcRequest{Token: c.token, Method: method}
	for _, arg := range args {
		data, err := json.Marshal(arg)
		if err != nil {
			return nil, err
		}
		request.Args = append(request.Args, data)
	}
	conn, err := winio.DialPipeContext(ctx, c.path)
	if err != nil {
		return nil, fmt.Errorf("无法连接 ClipBox 后台，请重新启动")
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return nil, err
	}
	var result ipcResponse
	if err := json.NewDecoder(io.LimitReader(conn, maxIPCBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("ClipBox 后台响应中断，请重试")
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}
	return result.Value, nil
}

func callUIBackend[T any](b *UIBridge, name string, args ...any) (T, error) {
	var value T
	if b.direct != nil {
		if name == "QuitApp" {
			b.quitting.Store(true)
		}
		request := ipcRequest{Method: name}
		for _, arg := range args {
			data, err := json.Marshal(arg)
			if err != nil {
				return value, err
			}
			request.Args = append(request.Args, data)
		}
		result := dispatchBackend(b.direct, request)
		if result.Error != "" {
			return value, fmt.Errorf("%s", result.Error)
		}
		if len(result.Value) > 0 {
			err := json.Unmarshal(result.Value, &value)
			return value, err
		}
		return value, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	data, err := b.client.request(ctx, name, args...)
	if err == nil && len(data) > 0 {
		err = json.Unmarshal(data, &value)
	}
	return value, err
}

// The API facade is the allowlist: internal lifecycle, raw database and file
// helpers cannot be invoked via IPC. Typed JSON arguments are decoded before
// invoking the existing service, and all failures become ordinary errors.
func dispatchBackend(a *App, request ipcRequest) (response ipcResponse) {
	defer func() {
		if recover() != nil {
			response = ipcResponse{Error: "后台操作失败，请重试"}
		}
	}()
	if !backendMethods[request.Method] {
		return ipcResponse{Error: "未知的后台操作"}
	}
	method := reflect.ValueOf(a).MethodByName(request.Method)
	if !method.IsValid() || method.Type().NumIn() != len(request.Args) {
		return ipcResponse{Error: "操作参数不匹配"}
	}
	args := make([]reflect.Value, len(request.Args))
	for index, raw := range request.Args {
		value := reflect.New(method.Type().In(index))
		if err := json.Unmarshal(raw, value.Interface()); err != nil {
			return ipcResponse{Error: "操作参数类型无效"}
		}
		args[index] = value.Elem()
	}
	values := method.Call(args)
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	if len(values) > 0 && values[len(values)-1].Type().Implements(errorType) {
		last := values[len(values)-1]
		if !last.IsNil() {
			return ipcResponse{Error: last.Interface().(error).Error()}
		}
		values = values[:len(values)-1]
	}
	if len(values) == 1 {
		data, err := json.Marshal(values[0].Interface())
		if err != nil || len(data) > maxIPCBytes {
			return ipcResponse{Error: "结果过大，请缩小操作范围"}
		}
		response.Value = data
	}
	return response
}

func sameToken(a, b string) bool {
	return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
