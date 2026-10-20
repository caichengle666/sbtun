package mobile

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/caichengle666/sbtun/app"
)

// instance 是唯一的 app.App，由 Init 创建。
var instance *app.App

type callResult struct {
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// Call 按名字调用 app.App 的导出方法，args 是 JSON 数组，返回 {"result":...} 或 {"error":"..."}。
// 规则和 Wails 一样：返回值最后一个是 error 时，非空就算失败。
func Call(method, args string) (out string) {
	defer func() {
		if r := recover(); r != nil {
			out = encodeResult(callResult{Error: fmt.Sprint(r)})
		}
	}()
	if instance == nil {
		return encodeResult(callResult{Error: "sbtun 未初始化"})
	}
	m := reflect.ValueOf(instance).MethodByName(method)
	if !m.IsValid() || blockedMethods[method] {
		return encodeResult(callResult{Error: "不支持的方法: " + method})
	}
	var raw []json.RawMessage
	if args != "" {
		if err := json.Unmarshal([]byte(args), &raw); err != nil {
			return encodeResult(callResult{Error: "参数格式错误: " + err.Error()})
		}
	}
	t := m.Type()
	if t.NumIn() != len(raw) {
		return encodeResult(callResult{Error: fmt.Sprintf("%s 需要 %d 个参数，收到 %d 个", method, t.NumIn(), len(raw))})
	}
	in := make([]reflect.Value, len(raw))
	for i := range raw {
		v := reflect.New(t.In(i))
		if err := json.Unmarshal(raw[i], v.Interface()); err != nil {
			return encodeResult(callResult{Error: fmt.Sprintf("第 %d 个参数错误: %v", i+1, err)})
		}
		in[i] = v.Elem()
	}
	var result callResult
	for _, v := range m.Call(in) {
		if v.Type().Implements(errorType) {
			if !v.IsNil() {
				result.Error = v.Interface().(error).Error()
			}
			continue
		}
		result.Result = v.Interface()
	}
	return encodeResult(result)
}

// 生命周期方法只能由 Go 自己调用。
var blockedMethods = map[string]bool{"Startup": true, "StartupCLI": true, "StartupMobile": true, "Shutdown": true, "StartMonitoring": true}

func encodeResult(r callResult) string {
	data, err := json.Marshal(r)
	if err != nil {
		data, _ = json.Marshal(callResult{Error: err.Error()})
	}
	return string(data)
}
