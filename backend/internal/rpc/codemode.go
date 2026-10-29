package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/metrics"
	"strings"
	"time"

	"github.com/dop251/goja"
)

const (
	scriptTimeout       = 50 * time.Second
	maxScriptOutput     = 60000
	maxScriptCallDepth  = 2048
	scriptMemoryPollGap = 20 * time.Millisecond
)

var scriptMemoryLimit uint64 = 512 << 20

var (
	errScriptPending = errors.New("the script awaited a promise that never settled")
	errScriptMemory  = errors.New("the script used too much memory")
)

type scriptRun struct {
	vm        *goja.Runtime
	stringify goja.Callable
	parse     goja.Callable
	logs      []string
}

func (s *Server) runScript(ctx context.Context, c caller, code string) (output string, err error) {
	ctx, cancel := context.WithTimeout(ctx, scriptTimeout)
	defer cancel()

	r, err := newScriptRun()
	if err != nil {
		return "", err
	}
	defer func() {
		if p := recover(); p != nil {
			output, err = r.output(""), fmt.Errorf("script crashed: %v", p)
		}
	}()

	api := r.vm.NewObject()
	for _, m := range c.methods() {
		name := m.name
		if err := api.Set(name, func(call goja.FunctionCall) goja.Value {
			return r.call(func(args json.RawMessage) (any, error) {
				return s.beeCallTool(c.context(ctx), name, args)
			}, call.Argument(0))
		}); err != nil {
			return "", err
		}
	}
	if err := r.vm.Set("openbee", api); err != nil {
		return "", err
	}

	stop := context.AfterFunc(ctx, func() { r.vm.Interrupt(ctx.Err()) })
	defer stop()
	go r.watchMemory(ctx)

	v, err := r.vm.RunString("(async () => {" + code + "\n})()")
	if err != nil {
		var interrupted *goja.InterruptedError
		if errors.As(err, &interrupted) {
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return r.output(""), fmt.Errorf("the script timed out after %s", scriptTimeout)
			}
			if cause, ok := interrupted.Value().(error); ok {
				return r.output(""), cause
			}
		}
		return r.output(""), err
	}
	p, ok := v.Export().(*goja.Promise)
	if !ok {
		return r.output(r.format(v)), nil
	}
	switch p.State() {
	case goja.PromiseStateFulfilled:
		return r.output(r.format(p.Result())), nil
	case goja.PromiseStateRejected:
		return r.output(""), errors.New(r.describeError(p.Result()))
	default:
		return r.output(""), errScriptPending
	}
}

func newScriptRun() (*scriptRun, error) {
	vm := goja.New()
	vm.SetMaxCallStackSize(maxScriptCallDepth)
	jsonObj := vm.Get("JSON").ToObject(vm)
	stringify, ok := goja.AssertFunction(jsonObj.Get("stringify"))
	if !ok {
		return nil, errors.New("JSON.stringify is not available")
	}
	parse, ok := goja.AssertFunction(jsonObj.Get("parse"))
	if !ok {
		return nil, errors.New("JSON.parse is not available")
	}
	r := &scriptRun{vm: vm, stringify: stringify, parse: parse}

	console := vm.NewObject()
	logFn := func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, a := range call.Arguments {
			parts[i] = r.format(a)
		}
		r.logs = append(r.logs, strings.Join(parts, " "))
		return goja.Undefined()
	}
	for _, name := range []string{"log", "info", "warn", "error", "debug"} {
		if err := console.Set(name, logFn); err != nil {
			return nil, err
		}
	}
	if err := vm.Set("console", console); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *scriptRun) watchMemory(ctx context.Context) {
	base := heapBytes()
	ticker := time.NewTicker(scriptMemoryPollGap)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if used := heapBytes(); used > base && used-base > scriptMemoryLimit {
				r.vm.Interrupt(errScriptMemory)
				return
			}
		}
	}
}

func heapBytes() uint64 {
	sample := []metrics.Sample{{Name: "/memory/classes/heap/objects:bytes"}}
	metrics.Read(sample)
	if sample[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return sample[0].Value.Uint64()
}

func (r *scriptRun) call(fn func(json.RawMessage) (any, error), arg goja.Value) goja.Value {
	promise, resolve, reject := r.vm.NewPromise()
	result, err := r.invoke(fn, arg)
	if err != nil {
		_ = reject(r.newError(err.Error()))
	} else {
		_ = resolve(result)
	}
	return r.vm.ToValue(promise)
}

func (r *scriptRun) invoke(fn func(json.RawMessage) (any, error), arg goja.Value) (goja.Value, error) {
	args := json.RawMessage("{}")
	if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
		encoded, err := r.stringify(goja.Undefined(), arg)
		if err != nil {
			return nil, fmt.Errorf("arguments are not JSON: %w", err)
		}
		args = json.RawMessage(encoded.String())
	}
	result, err := fn(args)
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("encode result: %w", err)
	}
	return r.parse(goja.Undefined(), r.vm.ToValue(string(b)))
}

func (r *scriptRun) newError(msg string) goja.Value {
	obj, err := r.vm.New(r.vm.Get("Error"), r.vm.ToValue(msg))
	if err != nil {
		return r.vm.ToValue(msg)
	}
	return obj
}

func (r *scriptRun) format(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return ""
	}
	if s, ok := v.Export().(string); ok {
		return s
	}
	encoded, err := r.stringify(goja.Undefined(), v, goja.Null(), r.vm.ToValue(2))
	if err != nil || goja.IsUndefined(encoded) {
		return v.String()
	}
	return encoded.String()
}

func (r *scriptRun) describeError(v goja.Value) string {
	if obj, ok := v.(*goja.Object); ok {
		if stack := obj.Get("stack"); stack != nil && !goja.IsUndefined(stack) {
			return stack.String()
		}
	}
	return v.String()
}

func (r *scriptRun) output(result string) string {
	var b strings.Builder
	b.WriteString(result)
	if len(r.logs) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("console:\n")
		b.WriteString(strings.Join(r.logs, "\n"))
	}
	out := b.String()
	if len(out) > maxScriptOutput {
		out = strings.ToValidUTF8(out[:maxScriptOutput], "") + "\n[output truncated: return less data]"
	}
	return out
}
